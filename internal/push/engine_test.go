package push

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"picpress/internal/imagepress"
)

func noisyJPEG(t *testing.T, seed uint64, quality int) []byte {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 1))
	img := image.NewRGBA(image.Rect(0, 0, 96, 64))
	for y := range 64 {
		for x := range 96 {
			img.Set(x, y, color.RGBA{uint8(rng.IntN(256)), uint8(rng.IntN(256)), uint8(rng.IntN(256)), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func randomBytes(n int, seed uint64) []byte {
	rng := rand.New(rand.NewPCG(seed, 2))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.IntN(256))
	}
	return b
}

type harness struct {
	t      *testing.T
	f      *fakeS3
	dir    string // journal / stage
	src    CredentialSource
	phases []Phase
	latest map[string]FileUpdate
	mu     sync.Mutex
	probe  func(context.Context) bool
}

func newHarness(t *testing.T) *harness {
	f := newFakeS3(t)
	return &harness{t: t, f: f, dir: t.TempDir(), src: StaticSource{Creds: f.creds("", 0)}}
}

func (h *harness) engine() *Engine {
	j, err := OpenJournal(h.dir)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { j.Close() })
	probe := h.probe
	if probe == nil {
		probe = func(context.Context) bool { return true }
	}
	return &Engine{
		S3: NewS3(), Session: &Session{Source: h.src}, Journal: j,
		StageDir: filepath.Join(h.dir, "stage"),
		Producers: map[Kind]Producer{
			KindImage:    ImageProducer{Options: imagepress.Options{Format: imagepress.FormatWebP, Quality: 80}},
			KindDocument: DocumentProducer{},
		},
		DeviceID: "mac-test", AppVersion: "test",
		Probe: probe,
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		OnFile: func(u FileUpdate) {
			h.mu.Lock()
			if h.latest == nil {
				h.latest = map[string]FileUpdate{}
			}
			h.latest[u.Source] = u
			h.mu.Unlock()
		},
		OnPhase: func(p Phase, _ string) {
			h.mu.Lock()
			h.phases = append(h.phases, p)
			h.mu.Unlock()
		},
	}
}

func (h *harness) run(ctx context.Context, batchID string, paths ...string) (Plan, Result, error) {
	e := h.engine()
	defer e.Journal.Close()
	plan := Scan(paths, PlanOptions{Supported: map[Kind]bool{KindImage: true, KindDocument: true}})
	res, err := e.Run(ctx, BatchState{ID: batchID, Paths: paths}, plan, plan.Entries)
	return plan, res, err
}

func (h *harness) files() []FileUpdate {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []FileUpdate
	for _, u := range h.latest {
		out = append(out, u)
	}
	return out
}

func sourceTree(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "案例")
	write(t, filepath.Join(root, "a.jpg"), noisyJPEG(t, 1, 95))
	write(t, filepath.Join(root, "子 目录 (1)", "b.jpg"), noisyJPEG(t, 2, 95))
	write(t, filepath.Join(root, "参数表.pdf"), randomBytes(3000, 3))
	write(t, filepath.Join(root, "notes.txt"), []byte("hello"))
	write(t, filepath.Join(root, ".DS_Store"), []byte("junk"))
	return root
}

// Acceptance 2, 3, 4, 15, 16.
func TestEndToEndDedupAndManifest(t *testing.T) {
	h := newHarness(t)
	root := sourceTree(t)

	plan, res, err := h.run(context.Background(), "b-1", root)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 4 || len(res.Failed) != 0 {
		t.Fatalf("first run: %+v", res)
	}
	got := strings.Join(h.f.userObjects(), "|")
	want := "案例/a.webp|案例/notes.txt|案例/参数表.pdf|案例/子 目录 (1)/b.webp"
	if got != want {
		t.Fatalf("objects\n got %s\nwant %s", got, want)
	}
	ms := h.f.manifests()
	if len(ms) == 0 || !ms[len(ms)-1].Final {
		t.Fatalf("no final manifest: %+v", ms)
	}
	if len(ms[0].Skipped) != 1 || ms[0].Skipped[0].Reason != ReasonHidden {
		t.Fatalf("skipped not reported: %+v", ms[0].Skipped)
	}
	meta := h.f.meta[testPrefix+"案例/参数表.pdf"]
	if meta.Get("X-Amz-Meta-Compressed") != "false" || !strings.HasPrefix(meta.Get("X-Amz-Meta-Source-Path"), "=?UTF-8?B?") ||
		meta.Get("Content-Type") != "application/pdf" || meta.Get("X-Amz-Meta-Batch-Id") != "b-1" {
		t.Fatalf("metadata: %v", meta)
	}
	if s := Summarize(plan, h.files()); !strings.HasPrefix(s.Text, "已送达 4 个（共 5 个）；重复跳过 0；跳过 1（隐藏或系统文件）") {
		t.Fatalf("summary: %s", s.Text)
	}

	// Same folder again: zero uploads.
	before := h.f.count("PUT 案例/") + h.f.count("PUT "+testPrefix+"案例/")
	_, res, err = h.run(context.Background(), "b-2", root)
	if err != nil || res.Uploaded != 0 || res.Duplicate != 4 {
		t.Fatalf("second run: %+v %v", res, err)
	}
	if after := h.f.count("PUT " + testPrefix + "案例/"); after != before {
		t.Fatalf("re-drop uploaded %d objects", after-before)
	}

	// Change one file: only that one goes.
	write(t, filepath.Join(root, "notes.txt"), []byte("changed"))
	_, res, err = h.run(context.Background(), "b-3", root)
	if err != nil || res.Uploaded != 1 || res.Duplicate != 3 {
		t.Fatalf("third run: %+v %v", res, err)
	}
	if string(h.f.objects[testPrefix+"案例/notes.txt"]) != "changed" {
		t.Fatal("changed file not overwritten")
	}
	h.f.assertNoReads(t)
}

// Acceptance 5, 7: interrupted multipart upload resumes from the journal.
func TestMultipartResumeAfterInterrupt(t *testing.T) {
	h := newHarness(t)
	root := filepath.Join(t.TempDir(), "大文件")
	data := randomBytes(2*PartSize+PartSize/2+MultipartThreshold-2*PartSize+1, 9) // > threshold, 3 parts
	write(t, filepath.Join(root, "manual.pdf"), data)

	ctx, cancel := context.WithCancelCause(context.Background())
	var once sync.Once
	h.f.hook = func(r *http.Request, op string) (int, string) {
		if op == "UploadPart" && r.URL.Query().Get("partNumber") == "2" {
			stop := false
			once.Do(func() { stop = true })
			if stop {
				cancel(ErrPaused) // like quitting the app mid-upload
				return 503, "SlowDown"
			}
		}
		return 0, ""
	}
	_, _, err := h.run(ctx, "b-1", root)
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("want paused, got %v", err)
	}
	if _, ok := h.f.objects[testPrefix+"大文件/manual.pdf"]; ok {
		t.Fatal("object must not exist yet")
	}

	_, res, err := h.run(context.Background(), "b-1", root)
	if err != nil || res.Uploaded != 1 {
		t.Fatalf("resume: %+v %v", res, err)
	}
	if n := h.f.count("partNumber=1&"); n != 1 {
		t.Fatalf("part 1 sent %d times", n)
	}
	if n := h.f.count("?uploads="); n != 1 {
		t.Fatalf("multipart created %d times", n)
	}
	if !bytes.Equal(h.f.objects[testPrefix+"大文件/manual.pdf"], data) {
		t.Fatal("assembled object differs")
	}
	h.f.assertNoReads(t)
}

// Acceptance 8: credentials rejected mid-batch are refreshed transparently.
func TestCredentialsRefreshOnExpiry(t *testing.T) {
	h := newHarness(t)
	fc := newFakeCreds(t, func(n int) Credentials { return h.f.creds("tok-"+itoa(n), time.Hour) })
	h.src = TokenSource{URL: fc.srv.URL, Token: "pp_test"}
	h.f.validToken = map[string]bool{"tok-1": true}
	var puts atomic.Int32
	h.f.hook = func(_ *http.Request, op string) (int, string) {
		if op == "PutObject" && puts.Add(1) == 2 {
			h.f.mu.Lock()
			h.f.validToken = map[string]bool{"tok-2": true} // tok-1 expired
			h.f.mu.Unlock()
		}
		return 0, ""
	}
	_, res, err := h.run(context.Background(), "b-1", sourceTree(t))
	if err != nil || len(res.Failed) != 0 || res.Uploaded != 4 {
		t.Fatalf("%+v %v", res, err)
	}
	if fc.calls != 2 {
		t.Fatalf("credentials fetched %d times, want 2", fc.calls)
	}
	h.f.assertNoReads(t)
}

func TestSessionRefreshesBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	calls := 0
	s := &Session{Now: func() time.Time { return now }, Source: sourceFunc(func() Credentials {
		calls++
		return Credentials{ExpiresAt: now.Add(10 * time.Minute)}
	})}
	s.Get(context.Background())
	s.Get(context.Background())
	now = now.Add(6 * time.Minute) // 4 minutes left < 5 minute margin
	s.Get(context.Background())
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}

	// 30-second credentials (acceptance 8) refresh at half-life, not always.
	calls = 0
	s = &Session{Now: func() time.Time { return now }, Source: sourceFunc(func() Credentials {
		calls++
		return Credentials{ExpiresAt: now.Add(30 * time.Second)}
	})}
	s.Get(context.Background())
	now = now.Add(10 * time.Second)
	s.Get(context.Background())
	now = now.Add(6 * time.Second) // 14 s left < 15 s
	s.Get(context.Background())
	if calls != 2 {
		t.Fatalf("short-lived calls %d", calls)
	}
}

type sourceFunc func() Credentials

func (f sourceFunc) Fetch(context.Context) (Credentials, error) { return f(), nil }

// Acceptance 1 / 9: a revoked token stops the batch with a clear message.
func TestRevokedTokenIsFatal(t *testing.T) {
	h := newHarness(t)
	fc := newFakeCreds(t, nil)
	fc.status = 401
	h.src = TokenSource{URL: fc.srv.URL, Token: "pp_test"}
	_, _, err := h.run(context.Background(), "b-1", sourceTree(t))
	var fe *FatalError
	if !errors.As(err, &fe) || fe.Message != msgRevoked {
		t.Fatalf("got %v", err)
	}
	if fc.calls > DefaultConcurrency+1 {
		t.Fatalf("retried a revoked token %d times", fc.calls)
	}
}

// Acceptance 18: a skewed clock is reported as such, not as a network error.
func TestClockSkewIsFatal(t *testing.T) {
	h := newHarness(t)
	h.f.hook = func(*http.Request, string) (int, string) { return 403, "RequestTimeTooSkewed" }
	_, _, err := h.run(context.Background(), "b-1", sourceTree(t))
	var fe *FatalError
	if !errors.As(err, &fe) || fe.Message != msgClockSkew {
		t.Fatalf("got %v", err)
	}
}

// Acceptance 6: losing the network pauses everything instead of failing files.
func TestOfflineWaitsInsteadOfFailing(t *testing.T) {
	h := newHarness(t)
	var offline atomic.Bool
	var probes atomic.Int32
	offline.Store(true)
	h.f.hook = func(*http.Request, string) (int, string) {
		if offline.Load() {
			return -1, "" // connection dropped
		}
		return 0, ""
	}
	h.probe = func(context.Context) bool {
		if offline.Load() {
			if probes.Add(1) >= 3 {
				offline.Store(false)
			}
			return false
		}
		return true
	}
	_, res, err := h.run(context.Background(), "b-1", sourceTree(t))
	if err != nil || len(res.Failed) != 0 || res.Uploaded != 4 {
		t.Fatalf("%+v %v", res, err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !containsPhase(h.phases, PhaseOffline) {
		t.Fatalf("phases %v", h.phases)
	}
}

func containsPhase(ps []Phase, p Phase) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
}

func TestRetryPolicy(t *testing.T) {
	h := newHarness(t)
	var fails sync.Map
	h.f.hook = func(r *http.Request, op string) (int, string) {
		key := r.URL.Path
		switch {
		case strings.HasSuffix(key, "notes.txt"):
			return 400, "InvalidArgument" // not retryable
		case op == "PutObject" && !strings.Contains(key, manifestDir):
			n, _ := fails.LoadOrStore(key, new(atomic.Int32))
			if n.(*atomic.Int32).Add(1) <= 2 {
				return 500, "InternalError"
			}
		}
		return 0, ""
	}
	_, res, err := h.run(context.Background(), "b-1", sourceTree(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 3 || len(res.Failed) != 1 || res.Failed[0].Path != "案例/notes.txt" {
		t.Fatalf("%+v", res)
	}
	if n := h.f.count("notes.txt?"); n != 1 {
		t.Fatalf("400 retried: %d requests", n)
	}
	ms := h.f.manifests()
	if len(ms[len(ms)-1].Failed) != 1 {
		t.Fatalf("failure not in manifest: %+v", ms)
	}
}

func TestRetriesAreBounded(t *testing.T) {
	h := newHarness(t)
	h.f.hook = func(r *http.Request, op string) (int, string) {
		if strings.HasSuffix(r.URL.Path, "notes.txt") {
			return 503, "SlowDown"
		}
		return 0, ""
	}
	_, res, _ := h.run(context.Background(), "b-1", sourceTree(t))
	if len(res.Failed) != 1 || res.Failed[0].Reason != ReasonNetwork {
		t.Fatalf("%+v", res)
	}
	if n := h.f.count("notes.txt?"); n != MaxRetries+1 {
		t.Fatalf("%d attempts, want %d", n, MaxRetries+1)
	}
}

// Acceptance 17: cancel aborts in-flight multipart uploads.
func TestCancelAbortsMultipart(t *testing.T) {
	h := newHarness(t)
	root := filepath.Join(t.TempDir(), "r")
	write(t, filepath.Join(root, "big.pdf"), randomBytes(MultipartThreshold+1, 5))
	ctx, cancel := context.WithCancelCause(context.Background())
	h.f.hook = func(r *http.Request, op string) (int, string) {
		if op == "UploadPart" && r.URL.Query().Get("partNumber") == "2" {
			cancel(ErrCanceled)
			return 503, "SlowDown"
		}
		return 0, ""
	}
	_, _, err := h.run(ctx, "b-1", root)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("got %v", err)
	}
	if n := h.f.count("DELETE "); n != 1 {
		t.Fatalf("aborts: %d", n)
	}
	if len(h.f.uploads) != 0 {
		t.Fatal("upload left open")
	}
	ms := h.f.manifests()
	if last := ms[len(ms)-1]; !last.Final || !last.Canceled {
		t.Fatalf("last manifest %+v", last)
	}
	h.f.assertNoReads(t)
}

// F5: when compression does not help, the original goes up unchanged.
func TestImageFallsBackToOriginal(t *testing.T) {
	dir := t.TempDir()
	src := noisyJPEG(t, 7, 30)
	p := filepath.Join(dir, "r", "noise.jpg")
	write(t, p, src)
	prod := ImageProducer{Options: imagepress.Options{Format: imagepress.FormatPNG}}
	out, err := prod.Produce(context.Background(), Entry{Path: p, Key: "r/noise.jpg", Kind: KindImage}, filepath.Join(dir, "stage"))
	if err != nil {
		t.Fatal(err)
	}
	a := out.Artifacts[0]
	staged, _ := os.ReadFile(a.Staged)
	if out.Compressed || a.Key != "r/noise.jpg" || !bytes.Equal(staged, src) || a.ContentType != "image/jpeg" {
		t.Fatalf("%+v", out)
	}
}

func TestImageDecodeErrorFailsOnlyThatFile(t *testing.T) {
	h := newHarness(t)
	root := sourceTree(t)
	write(t, filepath.Join(root, "坏图.jpg"), []byte("not a jpeg"))
	_, res, err := h.run(context.Background(), "b-1", root)
	if err != nil || res.Uploaded != 4 || len(res.Failed) != 1 || res.Failed[0].Reason != ReasonDecode {
		t.Fatalf("%+v %v", res, err)
	}
}

// Acceptance 11 (needs ffmpeg; the shipped app bundles it).
func TestVideoFrames(t *testing.T) {
	ffmpeg, ffprobe := FindFFmpeg()
	if ffmpeg == "" {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "车间视频", "a.mov")
	os.MkdirAll(filepath.Dir(clip), 0o755)
	cmd := execCommand(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=3:size=1920x1080:rate=25", "-pix_fmt", "yuv420p", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(clip)
	prod := VideoProducer{FFmpeg: ffmpeg, FFprobe: ffprobe, Frames: 12}
	out, err := prod.Produce(context.Background(), Entry{Path: clip, Source: "车间视频/a.mov", Key: "车间视频/a.mov", Kind: KindVideo, Size: st.Size()}, filepath.Join(dir, "stage"))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, a := range out.Artifacts {
		keys = append(keys, a.Key)
		if strings.HasSuffix(a.Key, ".mov") {
			t.Fatal("original must never be an artifact")
		}
	}
	want := "车间视频/a.mov.frames/frame-01.jpg|车间视频/a.mov.frames/frame-02.jpg|车间视频/a.mov.frames/frame-03.jpg|车间视频/a.mov.frames/contact.jpg|车间视频/a.mov.frames/meta.json"
	if strings.Join(keys, "|") != want {
		t.Fatalf("keys %v", keys)
	}
	if out.Video.Width != 1920 || out.Video.DurationSec < 2.9 || len(out.Video.Frames) != 3 {
		t.Fatalf("info %+v", out.Video)
	}
	img, err := jpeg.Decode(bytes.NewReader(mustRead(t, out.Artifacts[0].Staged)))
	if err != nil || img.Bounds().Dx() != frameLongEdge {
		t.Fatalf("frame size %v %v", img.Bounds(), err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var execCommand = exec.Command
