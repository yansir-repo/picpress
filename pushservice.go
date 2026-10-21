package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"picpress/internal/imagepress"
	"picpress/internal/push"
)

// appVersion is set at build time (-ldflags "-X main.appVersion=…").
var appVersion = "dev"

// PushService exposes the optional push pipeline (docs/push-upload.md). Go
// owns all state; the frontend renders snapshots and events.
type PushService struct {
	app      *application.App
	store    *push.Store
	deviceID string
	ffmpeg   string
	ffprobe  string

	mu       sync.Mutex
	dirty    map[string]bool // files changed since the last "push:files" event
	pending  *push.Config    // imported but not yet confirmed
	plan     *push.Plan
	paths    []string
	preset   PushPreset
	run      *activeRun
	files    map[string]push.FileUpdate
	order    []string
	phase    push.Phase
	message  string
	summary  *push.Summary
	failures []push.Entry
	batch    *push.BatchState
}

type activeRun struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
}

// PushPreset picks image settings for a batch (F5).
type PushPreset struct {
	Kind    string             `json:"kind"` // "current" | "text"
	Options imagepress.Options `json:"options"`
}

func (p PushPreset) imageOptions() imagepress.Options {
	if p.Kind == "text" {
		return imagepress.TextPreset()
	}
	return p.Options.Normalized()
}

// ConfigPreview is the confirmation card shown before a config takes effect.
type ConfigPreview struct {
	Label  string `json:"label"`
	Host   string `json:"host"`
	Notice string `json:"notice"`
}

// PushSnapshot is the whole push UI state.
type PushSnapshot struct {
	Connection     *push.Connection  `json:"connection"`
	PendingConfig  *ConfigPreview    `json:"pendingConfig"`
	Plan           *push.Plan        `json:"plan"`
	Files          []push.FileUpdate `json:"files"`
	Phase          string            `json:"phase"` // "" when idle
	Message        string            `json:"message"`
	Summary        *push.Summary     `json:"summary"`
	CanResume      bool              `json:"canResume"`
	DeviceID       string            `json:"deviceId"`
	AppVersion     string            `json:"appVersion"`
	VideoSupported bool              `json:"videoSupported"`
}

// NewPushService opens the application data directory.
func NewPushService() *PushService {
	s := &PushService{files: map[string]push.FileUpdate{}, dirty: map[string]bool{}}
	store, err := push.DefaultStore()
	if err != nil {
		log.Printf("push: no data directory: %v", err)
		return s
	}
	s.store = store
	if id, err := store.DeviceID(); err == nil {
		s.deviceID = id
	}
	s.ffmpeg, s.ffprobe = push.FindFFmpeg()
	return s
}

func (s *PushService) attach(app *application.App) { s.app = app }

// ServiceStartup resumes a batch that was running when the app last quit.
func (s *PushService) ServiceStartup(context.Context, application.ServiceOptions) error {
	go s.resumeOpenBatch()
	return nil
}

func (s *PushService) emit(name string, data any) {
	if s.app != nil {
		s.app.Event.Emit(name, data)
	}
}

func (s *PushService) changed() { s.emit("push:state", s.State()) }

func (s *PushService) supported() map[push.Kind]bool {
	return map[push.Kind]bool{push.KindImage: true, push.KindDocument: true, push.KindVideo: s.ffmpeg != ""}
}

// State returns the current snapshot.
func (s *PushService) State() PushSnapshot {
	conn, _ := s.connection()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := PushSnapshot{
		Connection: conn, Plan: s.plan, Phase: string(s.phase), Message: s.message, Summary: s.summary,
		DeviceID: s.deviceID, AppVersion: appVersion, VideoSupported: s.ffmpeg != "",
		Files: make([]push.FileUpdate, 0, len(s.order)),
	}
	if s.pending != nil {
		snap.PendingConfig = preview(*s.pending)
	}
	for _, k := range s.order {
		snap.Files = append(snap.Files, s.files[k])
	}
	snap.CanResume = s.run == nil && s.batch != nil && s.batch.Status.Open()
	return snap
}

func (s *PushService) connection() (*push.Connection, error) {
	if s.store == nil {
		return nil, errors.New("无法打开应用数据目录")
	}
	return s.store.Connection()
}

func preview(cfg push.Config) *ConfigPreview {
	u, _ := url.Parse(cfg.CredentialsURL)
	return &ConfigPreview{Label: cfg.Label, Host: u.Host, Notice: cfg.Notice}
}

// ImportConfig reads a .picpress-config file and holds it for confirmation.
func (s *PushService) ImportConfig(path string) (*ConfigPreview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("读不到这个配置文件")
	}
	cfg, err := push.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending = &cfg
	s.mu.Unlock()
	s.changed()
	return preview(cfg), nil
}

// PickConfig asks for a config file (menu「导入配置…」).
func (s *PushService) PickConfig() (*ConfigPreview, error) {
	dialog := s.app.Dialog.OpenFile().
		SetTitle("导入配置").
		SetButtonText("导入").
		AddFilter("PicPress 配置", "*"+push.ConfigExt)
	if win := s.app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	return s.ImportConfig(path)
}

// ConfirmConfig makes the pending config the active connection.
func (s *PushService) ConfirmConfig() (*push.Connection, error) {
	s.mu.Lock()
	cfg, busy := s.pending, s.run != nil
	s.mu.Unlock()
	if cfg == nil {
		return nil, errors.New("没有待确认的配置")
	}
	if busy {
		return nil, errors.New("正在上传，请先暂停或取消")
	}
	if s.store == nil {
		return nil, errors.New("无法打开应用数据目录")
	}
	conn, err := s.store.SaveConfig(*cfg)
	if err != nil {
		return nil, err
	}
	s.resetLocked(func() { s.pending = nil })
	s.changed()
	return conn, nil
}

// DismissConfig drops the pending config.
func (s *PushService) DismissConfig() {
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
	s.changed()
}

// SaveCustomS3 connects to a hand-entered S3-compatible endpoint (F3).
func (s *PushService) SaveCustomS3(in push.CustomS3) (*push.Connection, error) {
	if s.isRunning() {
		return nil, errors.New("正在上传，请先暂停或取消")
	}
	if s.store == nil {
		return nil, errors.New("无法打开应用数据目录")
	}
	conn, err := s.store.SaveCustom(in)
	if err != nil {
		return nil, err
	}
	s.resetLocked(nil)
	s.changed()
	return conn, nil
}

// RemoveConnection forgets the active connection.
func (s *PushService) RemoveConnection() error {
	if s.isRunning() {
		return errors.New("正在上传，请先暂停或取消")
	}
	if s.store == nil {
		return nil
	}
	if err := s.store.Remove(); err != nil {
		return err
	}
	s.resetLocked(nil)
	s.changed()
	return nil
}

func (s *PushService) resetLocked(extra func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plan, s.paths, s.batch, s.summary, s.failures = nil, nil, nil, nil, nil
	s.files, s.order, s.phase, s.message = map[string]push.FileUpdate{}, nil, "", ""
	if extra != nil {
		extra()
	}
}

func (s *PushService) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run != nil
}

// HasConnection tells the frontend where dropped files should go.
func (s *PushService) HasConnection() bool {
	c, _ := s.connection()
	return c != nil
}

// Prepare scans dropped paths into a plan. No network request is made.
func (s *PushService) Prepare(paths []string) (*push.Plan, error) {
	conn, err := s.connection()
	if err != nil || conn == nil {
		return nil, errors.New("还没有导入配置")
	}
	if s.isRunning() {
		return nil, errors.New("正在上传，请等这一批结束或暂停后再拖入")
	}
	plan := push.Scan(paths, push.PlanOptions{Limits: conn.Limits, Supported: s.supported()})
	s.mu.Lock()
	s.plan, s.paths = &plan, append([]string{}, paths...)
	s.files, s.order, s.summary, s.phase, s.message = map[string]push.FileUpdate{}, nil, nil, "", ""
	s.mu.Unlock()
	s.changed()
	return &plan, nil
}

// ClosePlan discards a plan that was not started. Nothing was sent.
func (s *PushService) ClosePlan() {
	s.mu.Lock()
	if s.run == nil {
		s.plan, s.paths = nil, nil
		s.files, s.order, s.summary, s.phase, s.message = map[string]push.FileUpdate{}, nil, nil, "", ""
	}
	s.mu.Unlock()
	s.changed()
}

// Start begins uploading the prepared plan.
func (s *PushService) Start(preset PushPreset) error {
	s.mu.Lock()
	plan, paths := s.plan, s.paths
	busy := s.run != nil
	s.mu.Unlock()
	switch {
	case busy:
		return errors.New("已经在上传")
	case plan == nil:
		return errors.New("请先拖入文件")
	case len(plan.Blocked) > 0:
		return errors.New(strings.Join(plan.Blocked, "；"))
	case len(plan.Entries) == 0:
		return errors.New("没有可以上传的文件")
	}
	opts, _ := json.Marshal(preset)
	batch := push.BatchState{
		ID: push.NewBatchID(time.Now(), uint16(rand.UintN(1<<16))), Paths: paths, Options: opts, Started: time.Now().UTC(),
	}
	return s.launch(batch, *plan, plan.Entries, preset, nil)
}

// Pause stops after in-flight requests; Resume continues from the journal.
func (s *PushService) Pause() { s.stop(push.ErrPaused) }

// Cancel ends the batch: in-flight multipart uploads are aborted and a final
// manifest is written. Finished files stay (the credentials cannot delete).
// A paused batch is canceled through the same engine path, with a run whose
// context is canceled before it starts.
func (s *PushService) Cancel() error {
	s.mu.Lock()
	running, b, plan := s.run != nil, s.batch, s.plan
	s.mu.Unlock()
	if running {
		s.stop(push.ErrCanceled)
		return nil
	}
	if b == nil || !b.Status.Open() {
		return nil
	}
	var p push.Plan
	if plan != nil {
		p = *plan
	}
	var preset PushPreset
	_ = json.Unmarshal(b.Options, &preset)
	return s.launch(*b, p, nil, preset, push.ErrCanceled)
}

func (s *PushService) stop(cause error) {
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	if r != nil {
		r.cancel(cause)
		<-r.done
	}
}

// Resume continues the open batch (paused, failed fatally, or interrupted).
func (s *PushService) Resume() error {
	s.mu.Lock()
	b, plan, busy := s.batch, s.plan, s.run != nil
	s.mu.Unlock()
	if busy {
		return nil
	}
	if b == nil {
		return errors.New("没有可以继续的上传")
	}
	var preset PushPreset
	_ = json.Unmarshal(b.Options, &preset)
	if plan == nil {
		conn, err := s.connection()
		if err != nil || conn == nil {
			return errors.New("还没有导入配置")
		}
		p := push.Scan(b.Paths, push.PlanOptions{Limits: conn.Limits, Supported: s.supported()})
		plan = &p
		s.mu.Lock()
		s.plan, s.paths = plan, b.Paths
		s.mu.Unlock()
	}
	return s.launch(*b, *plan, plan.Entries, preset, nil)
}

// RetryFailed re-runs only the files that failed in the last run.
func (s *PushService) RetryFailed() error {
	s.mu.Lock()
	b, plan, failed, busy := s.batch, s.plan, s.failures, s.run != nil
	s.mu.Unlock()
	if busy || b == nil || plan == nil || len(failed) == 0 {
		return nil
	}
	var preset PushPreset
	_ = json.Unmarshal(b.Options, &preset)
	return s.launch(*b, *plan, failed, preset, nil)
}

// launch runs the engine in the background. A non-nil stopCause cancels the
// run before it starts (used to cancel a paused batch).
func (s *PushService) launch(batch push.BatchState, plan push.Plan, entries []push.Entry, preset PushPreset, stopCause error) error {
	if s.store == nil {
		return errors.New("无法打开应用数据目录")
	}
	conn, err := s.connection()
	if err != nil || conn == nil {
		return errors.New("还没有导入配置")
	}
	src, err := s.store.Source(conn, push.DeviceInfo{DeviceID: s.deviceID, AppVersion: appVersion, OS: runtime.GOOS + "-" + runtime.GOARCH})
	if err != nil {
		return err
	}
	journal, err := push.OpenJournal(s.store.JournalDir(conn))
	if err != nil {
		return fmt.Errorf("无法打开本机上传日志：%w", err)
	}
	session := &push.Session{Source: src}
	producers := map[push.Kind]push.Producer{
		push.KindImage:    push.ImageProducer{Options: preset.imageOptions()},
		push.KindDocument: push.DocumentProducer{},
	}
	if s.ffmpeg != "" {
		producers[push.KindVideo] = push.VideoProducer{FFmpeg: s.ffmpeg, FFprobe: s.ffprobe, Frames: conn.Video.Frames}
	}
	engine := &push.Engine{
		S3: push.NewS3(), Session: session, Journal: journal,
		StageDir:  s.store.JournalDir(conn) + string(os.PathSeparator) + "stage",
		Producers: producers, DeviceID: s.deviceID, AppVersion: appVersion,
		ProbeAddrs: []string{push.HostPort(conn.CredentialsURL)},
		OnFile:     s.onFile,
		OnPhase:    s.onPhase,
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	if stopCause != nil {
		cancel(stopCause)
	}
	run := &activeRun{cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	s.run, s.batch, s.summary = run, &batch, nil
	for _, e := range entries {
		u := push.FileUpdate{Source: e.Source, Key: e.Key, Kind: e.Kind, Status: push.StatusWaiting, Total: e.Size}
		if _, ok := s.files[e.Source]; !ok {
			s.order = append(s.order, e.Source)
		}
		s.files[e.Source] = u
	}
	s.mu.Unlock()
	go s.flushFiles(run.done)

	go func() {
		defer close(run.done)
		defer journal.Close()
		res, err := engine.Run(ctx, batch, plan, entries)
		final, _ := journal.Batch()

		failedSet := map[string]bool{}
		for _, f := range res.Failed {
			failedSet[f.Path] = true
		}
		var failures []push.Entry
		for _, e := range entries {
			if failedSet[e.Source] {
				failures = append(failures, e)
			}
		}
		s.mu.Lock()
		files := make([]push.FileUpdate, 0, len(s.order))
		for _, k := range s.order {
			files = append(files, s.files[k])
		}
		sum := push.Summarize(plan, files)
		s.run, s.batch, s.failures = nil, &final, failures
		if err == nil || errors.Is(err, push.ErrCanceled) {
			s.summary = &sum
		}
		s.mu.Unlock()
		s.changed()
		if err == nil {
			s.emit("push:done", sum)
		}
	}()
	s.changed()
	return nil
}

func (s *PushService) onFile(u push.FileUpdate) {
	s.mu.Lock()
	if _, ok := s.files[u.Source]; !ok {
		s.order = append(s.order, u.Source)
	}
	s.files[u.Source] = u
	s.dirty[u.Source] = true
	s.mu.Unlock()
}

// fileEventInterval batches per-file updates: one IPC event per interval
// carrying every file that changed, instead of one event per change.
const fileEventInterval = 250 * time.Millisecond

func (s *PushService) flushFiles(done <-chan struct{}) {
	t := time.NewTicker(fileEventInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-done:
			s.mu.Lock()
			s.dirty = map[string]bool{} // the final "push:state" carries everything
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		batch := make([]push.FileUpdate, 0, len(s.dirty))
		for k := range s.dirty {
			batch = append(batch, s.files[k])
		}
		s.dirty = map[string]bool{}
		s.mu.Unlock()
		if len(batch) > 0 {
			s.emit("push:files", batch)
		}
	}
}

func (s *PushService) onPhase(p push.Phase, msg string) {
	s.mu.Lock()
	s.phase, s.message = p, msg
	s.mu.Unlock()
	s.emit("push:phase", map[string]string{"phase": string(p), "message": msg})
}

// resumeOpenBatch continues a batch that was running when the app quit
// (F8: power loss, force quit). A batch the user paused stays paused.
func (s *PushService) resumeOpenBatch() {
	conn, err := s.connection()
	if err != nil || conn == nil {
		return
	}
	j, err := push.OpenJournal(s.store.JournalDir(conn))
	if err != nil {
		return
	}
	b, ok := j.Batch()
	j.Close()
	if !ok || !b.Status.Open() {
		return
	}
	s.mu.Lock()
	s.batch = &b
	s.mu.Unlock()
	if b.Status == push.BatchRunning {
		if err := s.Resume(); err != nil {
			log.Printf("push: resume: %v", err)
		}
		return
	}
	s.mu.Lock()
	s.phase = push.PhasePaused
	s.mu.Unlock()
	s.changed()
}

// shutdown stops a running batch as interrupted, so the next launch resumes
// it automatically (quitting is not the user asking to pause).
func (s *PushService) shutdown() { s.stop(push.ErrInterrupted) }

// CopySummary puts the end-of-batch text on the clipboard.
func (s *PushService) CopySummary() bool {
	s.mu.Lock()
	sum := s.summary
	s.mu.Unlock()
	if sum == nil || s.app == nil {
		return false
	}
	return s.app.Clipboard.SetText(sum.Text)
}
