package push

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// manifestDir is relative to the prefix. User files starting with "." are
// never uploaded (F10), so nothing a user drops can land here.
const manifestDir = ".picpress/batches/"

// ManifestFile is one uploaded object.
type ManifestFile struct {
	Key            string     `json:"key"`
	Size           int64      `json:"size"`
	SHA256         string     `json:"sha256"`
	OriginalSHA256 string     `json:"originalSha256"`
	OriginalSize   int64      `json:"originalSize"`
	Status         FileStatus `json:"status"`
}

// ManifestSkip is a skipped, renamed or failed source path.
type ManifestSkip struct {
	Path      string `json:"path"`
	Reason    Reason `json:"reason"`
	RenamedTo string `json:"renamedTo,omitempty"`
}

// ManifestVideo describes a video whose frames were uploaded.
type ManifestVideo struct {
	Path             string   `json:"path"`
	DurationSec      float64  `json:"durationSec"`
	Width            int      `json:"width"`
	Height           int      `json:"height"`
	Frames           []string `json:"frames"`
	Contact          string   `json:"contact"`
	OriginalUploaded bool     `json:"originalUploaded"`
}

// Manifest is the batch manifest object (docs/push-upload.md F9). Each one
// lists only what happened since the previous manifest of the batch.
type Manifest struct {
	V          int             `json:"v"`
	BatchID    string          `json:"batchId"`
	Seq        int             `json:"seq"`
	DeviceID   string          `json:"deviceId"`
	AppVersion string          `json:"appVersion"`
	SentAt     string          `json:"sentAt"`
	Roots      []string        `json:"roots"`
	Files      []ManifestFile  `json:"files"`
	Skipped    []ManifestSkip  `json:"skipped"`
	Failed     []ManifestSkip  `json:"failed"`
	Videos     []ManifestVideo `json:"videos"`
	Final      bool            `json:"final"`
	Canceled   bool            `json:"canceled,omitempty"`
}

func buildManifest(b BatchState, now time.Time, uploaded, dups []Record, skipped, failed []Skip, roots []string) Manifest {
	m := Manifest{
		V: 1, BatchID: b.ID, SentAt: now.Format(time.RFC3339), Roots: roots,
		Files: []ManifestFile{}, Skipped: []ManifestSkip{}, Failed: []ManifestSkip{}, Videos: []ManifestVideo{},
	}
	listed := map[string]bool{}
	add := func(rec Record, st FileStatus) {
		if listed[rec.Key] {
			return
		}
		listed[rec.Key] = true
		for _, a := range rec.Artifacts {
			m.Files = append(m.Files, ManifestFile{
				Key: a.Key, Size: a.Size, SHA256: a.SHA256,
				OriginalSHA256: rec.OriginalSHA256, OriginalSize: rec.OriginalSize, Status: st,
			})
		}
		if rec.Video != nil && st == StatusUploaded {
			v := ManifestVideo{
				Path: rec.Source, DurationSec: rec.Video.DurationSec,
				Width: rec.Video.Width, Height: rec.Video.Height, Frames: []string{},
			}
			for _, a := range rec.Artifacts {
				switch {
				case strings.HasSuffix(a.Key, "/contact.jpg"):
					v.Contact = a.Key
				case strings.Contains(a.Key, "/frame-"):
					v.Frames = append(v.Frames, a.Key)
				}
			}
			m.Videos = append(m.Videos, v)
		}
	}
	for _, r := range uploaded {
		add(r, StatusUploaded)
	}
	for _, r := range dups {
		add(r, StatusDuplicate)
	}
	for _, s := range skipped {
		m.Skipped = append(m.Skipped, ManifestSkip{Path: s.Path, Reason: s.Reason, RenamedTo: s.RenamedTo})
	}
	for _, s := range failed {
		m.Failed = append(m.Failed, ManifestSkip{Path: s.Path, Reason: s.Reason})
	}
	return m
}

func (m Manifest) encode() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// NewBatchID returns "b-YYYYMMDD-HHMMSS-xxxx"; unique per device in practice,
// and the device id disambiguates across devices.
func NewBatchID(now time.Time, rand4 uint16) string {
	return fmt.Sprintf("b-%s-%04x", now.Format("20060102-150405"), rand4)
}
