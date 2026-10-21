package push

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// Upload tuning (F8). Defaults live here and nowhere else.
const (
	DefaultConcurrency = 3
	MaxConcurrency     = 6
	MultipartThreshold = 16 << 20
	PartSize           = 8 << 20
	MaxRetries         = 6
	manifestEvery      = 50
	manifestInterval   = 60 * time.Second
	cleanupTimeout     = 20 * time.Second
	maxUploadRestarts  = 2
)

// Stopping is expressed as the cancellation cause of the context passed to
// Run. The cause decides the persisted batch status, i.e. what happens on
// the next launch:
//   - ErrPaused: the user paused; stays paused until they continue.
//   - ErrInterrupted: the app is quitting; resumes automatically next launch.
//   - ErrCanceled: the batch is over; in-flight multipart uploads are aborted.
var (
	ErrPaused      = errors.New("paused")
	ErrInterrupted = errors.New("interrupted")
	ErrCanceled    = errors.New("canceled")
)

// FileStatus is a per-file UI state.
type FileStatus string

const (
	StatusWaiting     FileStatus = "waiting"
	StatusCompressing FileStatus = "compressing"
	StatusUploading   FileStatus = "uploading"
	StatusUploaded    FileStatus = "uploaded"
	StatusDuplicate   FileStatus = "skipped-duplicate"
	StatusFailed      FileStatus = "failed"
)

// FileUpdate is emitted whenever a file changes state or makes progress.
type FileUpdate struct {
	Source string     `json:"source"`
	Key    string     `json:"key"`
	Kind   Kind       `json:"kind"`
	Status FileStatus `json:"status"`
	Reason Reason     `json:"reason,omitempty"`
	Detail string     `json:"detail,omitempty"`
	Sent   int64      `json:"sent"`
	Total  int64      `json:"total"`
}

// Phase is the engine-wide state.
type Phase string

const (
	PhaseRunning  Phase = "running"
	PhaseOffline  Phase = "waiting-network"
	PhasePaused   Phase = "paused"
	PhaseCanceled Phase = "canceled"
	PhaseDone     Phase = "done"
	PhaseFatal    Phase = "fatal"
)

// Engine runs batches against one target. Everything it needs is injected,
// which is also how tests replace the network and the clock.
type Engine struct {
	S3          *S3
	Session     *Session
	Journal     *Journal
	StageDir    string
	Producers   map[Kind]Producer
	Concurrency int
	BytesPerSec int64
	DeviceID    string
	AppVersion  string
	ProbeAddrs  []string                       // extra host:port to probe (credentials endpoint)
	Probe       func(ctx context.Context) bool // nil: dial the S3 endpoint and ProbeAddrs
	Sleep       func(ctx context.Context, d time.Duration) error
	Now         func() time.Time
	OnFile      func(FileUpdate)
	OnPhase     func(phase Phase, message string)
}

// Result is the outcome of one Run.
type Result struct {
	Uploaded  int    `json:"uploaded"`
	Duplicate int    `json:"duplicate"`
	Failed    []Skip `json:"failed"`
	Videos    int    `json:"videos"`
}

type fileResult struct {
	entry  Entry
	status FileStatus
	fail   *Skip
	rec    Record
}

type runner struct {
	*Engine
	gate    *netGate
	limiter *rateLimiter
	batch   BatchState
}

// Run uploads the plan's entries as part of batch. It returns nil when the
// batch finished (failed files are in Result), ErrPaused / ErrCanceled when
// the context was canceled with that cause, or a *FatalError.
func (e *Engine) Run(parent context.Context, batch BatchState, plan Plan, entries []Entry) (Result, error) {
	r := &runner{Engine: e, batch: batch, limiter: &rateLimiter{rate: float64(e.BytesPerSec)}}
	if r.Sleep == nil {
		r.Sleep = sleepCtx
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)

	r.gate = &netGate{probe: e.Probe, sleep: r.Sleep, onChange: func(off bool) {
		if off {
			r.phase(PhaseOffline, "正在等待网络…")
		} else {
			r.phase(PhaseRunning, "")
		}
	}}
	if r.gate.probe == nil {
		r.gate.probe = dialProbe(func() []string {
			return append(hostPorts(e.Session.Endpoint()), e.ProbeAddrs...)
		})
	}

	r.batch.Status = BatchRunning
	if err := r.Journal.PutBatch(r.batch); err != nil {
		return Result{}, err
	}
	r.phase(PhaseRunning, "")

	conc := e.Concurrency
	if conc <= 0 {
		conc = DefaultConcurrency
	}
	conc = min(conc, MaxConcurrency)

	jobs := make(chan Entry)
	results := make(chan fileResult)
	var wg sync.WaitGroup
	for range conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for en := range jobs {
				res, err := r.process(ctx, en)
				if err != nil {
					if fe := (*FatalError)(nil); errors.As(err, &fe) {
						cancel(fe)
					}
					continue // canceled or fatal: not a per-file result
				}
				results <- res
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, en := range entries {
			select {
			case jobs <- en:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()

	col := &collector{plan: plan, lastFlush: r.Now()}
	var out Result
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
loop:
	for {
		select {
		case res, ok := <-results:
			if !ok {
				break loop
			}
			col.add(res, &out)
			if col.since >= manifestEvery {
				_ = r.flush(ctx, col, false, false)
			}
		case <-tick.C:
			if col.since > 0 && r.Now().Sub(col.lastFlush) >= manifestInterval {
				_ = r.flush(ctx, col, false, false)
			}
		}
	}

	cause := context.Cause(ctx)
	if cause == nil {
		if err := r.flush(ctx, col, true, false); err != nil {
			cause = context.Cause(ctx)
			if cause == nil {
				r.batch.Status = BatchPaused
				_ = r.Journal.PutBatch(r.batch)
				msg := "批次清单没能写入，请稍后点「继续」。"
				r.phase(PhaseFatal, msg)
				return out, &FatalError{Reason: ReasonNetwork, Message: msg}
			}
		} else {
			r.batch.Status = BatchDone
			_ = r.Journal.PutBatch(r.batch)
			r.phase(PhaseDone, "")
			return out, nil
		}
	}

	// Stopped early. Write what we have with a short, uncancelled context.
	cctx, ccancel := context.WithTimeout(context.WithoutCancel(parent), cleanupTimeout)
	defer ccancel()
	var fatal *FatalError
	switch {
	case errors.Is(cause, ErrCanceled):
		r.abortPending(cctx)
		_ = r.flush(cctx, col, true, true)
		r.batch.Status = BatchCanceled
		_ = r.Journal.PutBatch(r.batch)
		r.phase(PhaseCanceled, "")
		return out, ErrCanceled
	case errors.As(cause, &fatal):
		_ = r.flush(cctx, col, false, false)
		r.batch.Status = BatchPaused
		_ = r.Journal.PutBatch(r.batch)
		r.phase(PhaseFatal, fatal.Message)
		return out, fatal
	case errors.Is(cause, ErrInterrupted):
		_ = r.flush(cctx, col, false, false)
		r.batch.Status = BatchRunning
		_ = r.Journal.PutBatch(r.batch)
		return out, ErrInterrupted
	default:
		_ = r.flush(cctx, col, false, false)
		r.batch.Status = BatchPaused
		_ = r.Journal.PutBatch(r.batch)
		r.phase(PhasePaused, "")
		return out, ErrPaused
	}
}

func (r *runner) phase(p Phase, msg string) {
	if r.OnPhase != nil {
		r.OnPhase(p, msg)
	}
}

func (r *runner) file(en Entry, st FileStatus, fail *Skip, sent, total int64) {
	if r.OnFile == nil {
		return
	}
	u := FileUpdate{Source: en.Source, Key: en.Key, Kind: en.Kind, Status: st, Sent: sent, Total: total}
	if fail != nil {
		u.Reason, u.Detail = fail.Reason, fail.Detail
	}
	r.OnFile(u)
}

// process handles one entry. A non-nil error means the run is stopping
// (canceled or fatal); everything else is a fileResult.
func (r *runner) process(ctx context.Context, en Entry) (fileResult, error) {
	failed := func(reason Reason, detail string) (fileResult, error) {
		s := &Skip{Path: en.Source, Reason: reason, Detail: detail}
		r.file(en, StatusFailed, s, 0, 0)
		return fileResult{entry: en, status: StatusFailed, fail: s}, nil
	}
	stop := func(err error) (fileResult, error) {
		if classify(err) == classFatal {
			return fileResult{}, asFatal(err)
		}
		if ctx.Err() != nil {
			return fileResult{}, ctx.Err()
		}
		var fe *FileError
		if errors.As(err, &fe) {
			return failed(fe.Reason, fe.Detail)
		}
		return failed(failReason(err), "上传失败："+err.Error())
	}

	prod := r.Producers[en.Kind]
	if prod == nil {
		return failed(ReasonUnsupported, "格式暂不支持")
	}
	sum, size, err := hashFile(en.Path)
	if err != nil {
		return stop(readError(err))
	}
	params := prod.Params()
	var target string
	if err := r.withRetry(ctx, func(c Credentials) error { target = c.Target(); return nil }); err != nil {
		return stop(err)
	}

	// Dedup identity: (target, key, original bytes, transformation).
	rec, ok := r.Journal.Get(en.Key)
	same := ok && rec.Target == target && rec.OriginalSHA256 == sum && rec.Params == params
	if same && rec.Done {
		r.file(en, StatusDuplicate, nil, 0, 0)
		return fileResult{entry: en, status: StatusDuplicate, rec: rec}, nil
	}
	if !(same && stagedIntact(rec)) {
		if ok && !rec.Done {
			r.discard(ctx, rec)
		}
		r.file(en, StatusCompressing, nil, 0, 0)
		out, err := prod.Produce(ctx, en, r.StageDir)
		if err != nil {
			return stop(err)
		}
		rec = Record{
			Key: en.Key, Target: target, Source: en.Source, Kind: en.Kind,
			OriginalSHA256: sum, OriginalSize: size, Params: params,
			Compressed: out.Compressed || en.Kind == KindVideo, Video: out.Video, Batch: r.batch.ID,
		}
		for _, a := range out.Artifacts {
			a.Meta = map[string]string{
				"original-sha256":  sum,
				"original-size":    strconv.FormatInt(size, 10),
				"source-path":      en.Source,
				"compressed":       strconv.FormatBool(rec.Compressed),
				"params":           params,
				"device-id":        r.DeviceID,
				"batch-id":         r.batch.ID,
				"picpress-version": r.AppVersion,
				"sha256":           a.SHA256,
			}
			rec.Artifacts = append(rec.Artifacts, a)
		}
		if err := r.Journal.Put(rec); err != nil {
			return failed(ReasonRead, "写入本机日志失败："+err.Error())
		}
	}

	var total, sent int64
	for _, a := range rec.Artifacts {
		total += a.Size
		if a.Done {
			sent += a.Size
		}
	}
	r.file(en, StatusUploading, nil, sent, total)
	for i := range rec.Artifacts {
		if rec.Artifacts[i].Done {
			continue
		}
		base := sent
		if err := r.upload(ctx, &rec, i, func(n int64) { r.file(en, StatusUploading, nil, base+n, total) }); err != nil {
			return stop(err)
		}
		sent += rec.Artifacts[i].Size
	}

	rec.Done, rec.Manifested, rec.Batch = true, false, r.batch.ID
	if err := r.Journal.Put(rec); err != nil {
		return failed(ReasonRead, "写入本机日志失败："+err.Error())
	}
	for _, a := range rec.Artifacts {
		_ = os.Remove(a.Staged)
	}
	r.file(en, StatusUploaded, nil, total, total)
	return fileResult{entry: en, status: StatusUploaded, rec: rec}, nil
}

// stagedIntact: every artifact still to upload has its exact staged bytes.
func stagedIntact(rec Record) bool {
	if len(rec.Artifacts) == 0 {
		return false
	}
	for _, a := range rec.Artifacts {
		if a.Done {
			continue
		}
		sum, _, err := hashFile(a.Staged)
		if err != nil || sum != a.SHA256 {
			return false
		}
	}
	return true
}

// discard aborts a superseded in-flight upload and removes its staged files.
func (r *runner) discard(ctx context.Context, rec Record) {
	for _, a := range rec.Artifacts {
		if a.UploadID != "" && !a.Done {
			_ = r.withRetry(ctx, func(c Credentials) error {
				return r.S3.AbortMultipart(ctx, c, c.Prefix+a.Key, a.UploadID)
			})
		}
		if a.Staged != "" {
			_ = os.Remove(a.Staged)
		}
	}
}

// upload sends one artifact, persisting multipart progress after every part.
func (r *runner) upload(ctx context.Context, rec *Record, i int, progress func(int64)) error {
	a := &rec.Artifacts[i]
	meta := ObjectMeta{ContentType: a.ContentType, Meta: a.Meta}
	if a.Size <= MultipartThreshold {
		data, err := os.ReadFile(a.Staged)
		if err != nil {
			return readError(err)
		}
		if err := r.limiter.wait(ctx, len(data)); err != nil {
			return err
		}
		if err := r.withRetry(ctx, func(c Credentials) error {
			return r.S3.Put(ctx, c, c.Prefix+a.Key, data, meta)
		}); err != nil {
			return err
		}
		a.Done = true
		progress(a.Size)
		return r.Journal.Put(*rec)
	}

	f, err := os.Open(a.Staged)
	if err != nil {
		return readError(err)
	}
	defer f.Close()
	nparts := int((a.Size + PartSize - 1) / PartSize)
	buf := make([]byte, PartSize)

	for restarts := 0; ; restarts++ {
		if a.UploadID == "" {
			var id string
			if err := r.withRetry(ctx, func(c Credentials) error {
				var err error
				id, err = r.S3.CreateMultipart(ctx, c, c.Prefix+a.Key, meta)
				return err
			}); err != nil {
				return err
			}
			a.UploadID, a.Parts = id, nil
			if err := r.Journal.Put(*rec); err != nil {
				return err
			}
		}
		err := r.sendParts(ctx, rec, a, f, buf, nparts, progress)
		if err == nil {
			err = r.withRetry(ctx, func(c Credentials) error {
				return r.S3.CompleteMultipart(ctx, c, c.Prefix+a.Key, a.UploadID, a.Parts)
			})
		}
		var se *S3Error
		if errors.As(err, &se) && se.Code == "NoSuchUpload" && restarts < maxUploadRestarts {
			// The upload expired or was aborted remotely: start this file over.
			a.UploadID, a.Parts = "", nil
			continue
		}
		if err != nil {
			return err
		}
		a.Done, a.UploadID, a.Parts = true, "", nil
		return r.Journal.Put(*rec)
	}
}

func (r *runner) sendParts(ctx context.Context, rec *Record, a *Artifact, f *os.File, buf []byte, nparts int, progress func(int64)) error {
	done := map[int]bool{}
	for _, p := range a.Parts {
		done[p.N] = true
	}
	for n := 1; n <= nparts; n++ {
		if done[n] {
			continue
		}
		off := int64(n-1) * PartSize
		size := min(int64(PartSize), a.Size-off)
		chunk := buf[:size]
		if _, err := f.ReadAt(chunk, off); err != nil && !errors.Is(err, io.EOF) {
			return readError(err)
		}
		if err := r.limiter.wait(ctx, len(chunk)); err != nil {
			return err
		}
		var etag string
		if err := r.withRetry(ctx, func(c Credentials) error {
			var err error
			etag, err = r.S3.UploadPart(ctx, c, c.Prefix+a.Key, a.UploadID, n, chunk)
			return err
		}); err != nil {
			return err
		}
		a.Parts = append(a.Parts, Part{N: n, ETag: etag})
		if err := r.Journal.Put(*rec); err != nil {
			return err
		}
		progress(min(int64(len(a.Parts))*PartSize, a.Size))
	}
	sortParts(a.Parts)
	return nil
}

func sortParts(ps []Part) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].N < ps[j-1].N; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

// withRetry runs op with fresh credentials and the F8 retry policy.
func (r *runner) withRetry(ctx context.Context, op func(Credentials) error) error {
	attempt, refreshed := 0, false
	for {
		c, gen, err := r.Session.Get(ctx)
		if err == nil {
			err = op(c)
		}
		if err == nil {
			return nil
		}
		switch classify(err) {
		case classCanceled:
			return err
		case classFatal:
			return asFatal(err)
		case classRefresh:
			if !refreshed {
				refreshed = true
				r.Session.Invalidate(gen)
				continue
			}
			return err
		case classTransport:
			waited, werr := r.gate.check(ctx)
			if werr != nil {
				return werr
			}
			if waited {
				continue // the network was down; this attempt does not count
			}
		case classRetry:
		default:
			return err
		}
		if attempt >= MaxRetries {
			return err
		}
		d := max(backoff(attempt), retryAfter(err))
		attempt++
		if serr := r.Sleep(ctx, d); serr != nil {
			return serr
		}
	}
}

// abortPending aborts every unfinished multipart upload (cancel, F8).
func (r *runner) abortPending(ctx context.Context) {
	for _, rec := range r.Journal.Pending() {
		changed := false
		for i, a := range rec.Artifacts {
			if a.UploadID == "" || a.Done {
				continue
			}
			c, _, err := r.Session.Get(ctx)
			if err == nil {
				err = r.S3.AbortMultipart(ctx, c, c.Prefix+a.Key, a.UploadID)
			}
			if err == nil {
				rec.Artifacts[i].UploadID, rec.Artifacts[i].Parts = "", nil
				changed = true
			}
		}
		if changed {
			_ = r.Journal.Put(rec)
		}
	}
}

// collector accumulates per-run results between manifests. Uploaded files
// are read back from the journal at flush time, so a crash between upload and
// manifest loses nothing.
type collector struct {
	plan      Plan
	dups      []Record
	failed    []Skip
	since     int
	lastFlush time.Time
}

func (c *collector) add(res fileResult, out *Result) {
	c.since++
	switch res.status {
	case StatusUploaded:
		out.Uploaded++
		if res.entry.Kind == KindVideo {
			out.Videos++
		}
	case StatusDuplicate:
		out.Duplicate++
		if res.entry.Kind == KindVideo {
			out.Videos++
		}
		c.dups = append(c.dups, res.rec)
	case StatusFailed:
		out.Failed = append(out.Failed, *res.fail)
		c.failed = append(c.failed, *res.fail)
	}
}

func (r *runner) flush(ctx context.Context, col *collector, final, canceled bool) error {
	recs := r.Journal.Unmanifested()
	var skipped []Skip
	if !r.batch.SkipsReported {
		skipped = col.plan.Skipped
	}
	if !final && len(recs) == 0 && len(col.dups) == 0 && len(col.failed) == 0 && len(skipped) == 0 {
		return nil
	}
	m := buildManifest(r.batch, r.Now(), recs, col.dups, skipped, col.failed, col.plan.Roots)
	m.DeviceID, m.AppVersion = r.DeviceID, r.AppVersion
	m.Seq = r.batch.Seq + 1
	m.Final, m.Canceled = final, canceled
	data, err := m.encode()
	if err != nil {
		return err
	}
	rel := fmt.Sprintf("%s%s-%d.json", manifestDir, r.batch.ID, m.Seq)
	if err := r.withRetry(ctx, func(c Credentials) error {
		return r.S3.Put(ctx, c, c.Prefix+rel, data, ObjectMeta{ContentType: "application/json"})
	}); err != nil {
		return err
	}
	keys := make([]string, len(recs))
	for i, rec := range recs {
		keys[i] = rec.Key
	}
	if err := r.Journal.MarkManifested(keys); err != nil {
		return err
	}
	r.batch.Seq = m.Seq
	r.batch.SkipsReported = true
	if err := r.Journal.PutBatch(r.batch); err != nil {
		return err
	}
	col.dups, col.failed, col.since, col.lastFlush = nil, nil, 0, r.Now()
	return nil
}
