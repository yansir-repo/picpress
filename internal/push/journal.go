package push

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Artifact is one object produced from a source file. Key is relative to
// the target prefix.
type Artifact struct {
	Key         string            `json:"key"`
	Staged      string            `json:"staged"` // local bytes that are (being) uploaded
	Size        int64             `json:"size"`
	SHA256      string            `json:"sha256"`
	ContentType string            `json:"contentType"`
	Meta        map[string]string `json:"meta,omitempty"`
	UploadID    string            `json:"uploadId,omitempty"`
	Parts       []Part            `json:"parts,omitempty"`
	Done        bool              `json:"done"`
}

// VideoInfo is what meta.json and the manifest say about a video.
type VideoInfo struct {
	DurationSec  float64      `json:"durationSec"`
	Width        int          `json:"width"`
	Height       int          `json:"height"`
	FPS          float64      `json:"fps"`
	Codec        string       `json:"codec"`
	SizeBytes    int64        `json:"sizeBytes"`
	OriginalName string       `json:"originalName"`
	OriginalPath string       `json:"originalPath"`
	Frames       []FrameStamp `json:"frames"`
}

// FrameStamp locates one extracted frame.
type FrameStamp struct {
	File  string  `json:"file"`
	AtSec float64 `json:"atSec"`
}

// Record is the journal's state for one entry key. It is the only source of
// truth for "was this sent".
type Record struct {
	Key            string     `json:"key"`
	Target         string     `json:"target"` // Credentials.Target() it was uploaded to
	Source         string     `json:"source"`
	Kind           Kind       `json:"kind"`
	OriginalSHA256 string     `json:"originalSha256"`
	OriginalSize   int64      `json:"originalSize"`
	Params         string     `json:"params"`
	Compressed     bool       `json:"compressed"`
	Artifacts      []Artifact `json:"artifacts"`
	Video          *VideoInfo `json:"video,omitempty"`
	Done           bool       `json:"done"`
	Batch          string     `json:"batch,omitempty"`
	Manifested     bool       `json:"manifested"`
	At             time.Time  `json:"at"`
}

// BatchState is the persisted state of one confirmed drop.
type BatchState struct {
	ID            string          `json:"id"`
	Seq           int             `json:"seq"`
	Paths         []string        `json:"paths"`   // absolute dropped paths, re-scanned on resume
	Options       json.RawMessage `json:"options"` // producer settings, opaque to the journal
	Status        BatchStatus     `json:"status"`
	SkipsReported bool            `json:"skipsReported"`
	Started       time.Time       `json:"started"`
}

// BatchStatus is the lifecycle of a batch.
type BatchStatus string

const (
	BatchRunning  BatchStatus = "running"
	BatchPaused   BatchStatus = "paused"
	BatchDone     BatchStatus = "done"
	BatchCanceled BatchStatus = "canceled"
)

// Open reports whether a batch should be resumed.
func (b BatchStatus) Open() bool { return b == BatchRunning || b == BatchPaused }

type line struct {
	T      string      `json:"t"` // "rec" | "batch"
	Record *Record     `json:"rec,omitempty"`
	Batch  *BatchState `json:"batch,omitempty"`
}

// Journal is an append-only JSON Lines log, replayed into memory on open and
// compacted. Every append is fsynced before it returns, so a record is either
// fully on disk or (torn last line) ignored on replay.
type Journal struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	records map[string]Record
	batch   *BatchState
}

// TargetDir is the per-target directory under base (journal + staged files).
func TargetDir(base, target string) string {
	sum := sha256.Sum256([]byte(target))
	return filepath.Join(base, "targets", hex.EncodeToString(sum[:])[:16])
}

// OpenJournal replays dir/journal.jsonl and compacts it.
func OpenJournal(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	j := &Journal{path: filepath.Join(dir, "journal.jsonl"), records: map[string]Record{}}
	if f, err := os.Open(j.path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
		for sc.Scan() {
			var l line
			if json.Unmarshal(sc.Bytes(), &l) != nil {
				continue // torn write from a crash
			}
			switch {
			case l.Record != nil:
				j.records[l.Record.Key] = *l.Record
			case l.Batch != nil:
				b := *l.Batch
				j.batch = &b
			}
		}
		f.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := j.compact(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(j.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	j.f = f
	return j, nil
}

func (j *Journal) compact() error {
	tmp := j.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	keys := make([]string, 0, len(j.records))
	for k := range j.records {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := j.records[k]
		if err := enc.Encode(line{T: "rec", Record: &r}); err != nil {
			f.Close()
			return err
		}
	}
	if j.batch != nil {
		if err := enc.Encode(line{T: "batch", Batch: j.batch}); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

func (j *Journal) append(l line) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if _, err := j.f.Write(append(data, '\n')); err != nil {
		return err
	}
	return j.f.Sync()
}

// Get returns the record for an entry key.
func (j *Journal) Get(key string) (Record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.records[key]
	return r, ok
}

// Put stores a record durably.
func (j *Journal) Put(r Record) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	r.At = time.Now().UTC()
	if err := j.append(line{T: "rec", Record: &r}); err != nil {
		return err
	}
	j.records[r.Key] = r
	return nil
}

// Unmanifested returns completed records not yet listed in any manifest.
func (j *Journal) Unmanifested() []Record {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Record
	for _, r := range j.records {
		if r.Done && !r.Manifested {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Key < out[b].Key })
	return out
}

// Pending returns records with an upload in flight (for abort on cancel).
func (j *Journal) Pending() []Record {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Record
	for _, r := range j.records {
		if !r.Done {
			out = append(out, r)
		}
	}
	return out
}

// MarkManifested flags records as listed in a written manifest.
func (j *Journal) MarkManifested(keys []string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, k := range keys {
		r, ok := j.records[k]
		if !ok || r.Manifested {
			continue
		}
		r.Manifested = true
		if err := j.append(line{T: "rec", Record: &r}); err != nil {
			return err
		}
		j.records[k] = r
	}
	return nil
}

// Batch returns the current batch, if any.
func (j *Journal) Batch() (BatchState, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.batch == nil {
		return BatchState{}, false
	}
	return *j.batch, true
}

// PutBatch stores the batch state durably.
func (j *Journal) PutBatch(b BatchState) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.append(line{T: "batch", Batch: &b}); err != nil {
		return err
	}
	j.batch = &b
	return nil
}

// Close closes the log file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		return nil
	}
	err := j.f.Close()
	j.f = nil
	return err
}
