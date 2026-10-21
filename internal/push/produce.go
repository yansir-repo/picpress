package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"picpress/internal/imagepress"
)

// FileError fails one file without affecting the batch.
type FileError struct {
	Reason Reason
	Detail string
}

func (e *FileError) Error() string { return string(e.Reason) + ": " + e.Detail }

// Output is what a producer made from one source file. Artifact keys are
// relative to the prefix; the producer writes staged files under stageDir.
type Output struct {
	Artifacts  []Artifact
	Compressed bool
	Video      *VideoInfo
}

// Producer turns a source file into the bytes that get uploaded. Params is
// the identity of the transformation: same source bytes + same Params means
// same upload, which is what dedup relies on.
type Producer interface {
	Params() string
	Produce(ctx context.Context, e Entry, stageDir string) (Output, error)
}

// ImageProducer compresses with imagepress and falls back to the original
// bytes when compression does not make the file smaller (F5).
type ImageProducer struct{ Options imagepress.Options }

func (p ImageProducer) Params() string { return p.Options.Params() }

func (p ImageProducer) Produce(_ context.Context, e Entry, stageDir string) (Output, error) {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return Output{}, readError(err)
	}
	img, _, err := imagepress.Decode(data)
	if err != nil {
		return Output{}, &FileError{Reason: ReasonDecode, Detail: "图片已损坏或无法解码"}
	}
	enc, err := imagepress.Compress(img, p.Options)
	if err != nil {
		return Output{}, &FileError{Reason: ReasonDecode, Detail: "压缩失败：" + err.Error()}
	}
	stem, origExt := splitExt(e.Key)
	out, ext, compressed := enc.Data, enc.Format.Ext(), true
	if int64(len(enc.Data)) >= int64(len(data)) {
		out, ext, compressed = data, origExt, false
	}
	a, err := stageBytes(stageDir, stem+ext, out)
	if err != nil {
		return Output{}, err
	}
	return Output{Artifacts: []Artifact{a}, Compressed: compressed}, nil
}

// DocumentProducer uploads the file unchanged (F6). The bytes are copied to
// the stage first so the uploaded bytes are exactly the hashed bytes, even if
// the user edits the original during the upload.
type DocumentProducer struct{}

func (DocumentProducer) Params() string { return "raw" }

func (DocumentProducer) Produce(_ context.Context, e Entry, stageDir string) (Output, error) {
	src, err := os.Open(e.Path)
	if err != nil {
		return Output{}, readError(err)
	}
	defer src.Close()
	a, err := stageReader(stageDir, e.Key, src)
	if err != nil {
		return Output{}, err
	}
	return Output{Artifacts: []Artifact{a}}, nil
}

func readError(err error) error {
	return &FileError{Reason: ReasonRead, Detail: "读不到这个文件（如果它在云盘里，请先下载到本机）"}
}

func stageName(stageDir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(stageDir, hex.EncodeToString(sum[:12])+path.Ext(key))
}

func stageBytes(stageDir, key string, data []byte) (Artifact, error) {
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return Artifact{}, err
	}
	p := stageName(stageDir, key)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("写入临时文件失败：%w", err)
	}
	sum := sha256.Sum256(data)
	return Artifact{Key: key, Staged: p, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), ContentType: contentType(key)}, nil
}

func stageReader(stageDir, key string, r io.Reader) (Artifact, error) {
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return Artifact{}, err
	}
	p := stageName(stageDir, key)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Artifact{}, fmt.Errorf("写入临时文件失败：%w", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(p)
		return Artifact{}, readError(err)
	}
	return Artifact{Key: key, Staged: p, Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), ContentType: contentType(key)}, nil
}

var extraTypes = map[string]string{
	".heic": "image/heic", ".heif": "image/heif", ".avif": "image/avif", ".webp": "image/webp",
	".md": "text/markdown; charset=utf-8", ".csv": "text/csv; charset=utf-8", ".json": "application/json",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".doc":  "application/msword", ".xls": "application/vnd.ms-excel", ".ppt": "application/vnd.ms-powerpoint",
}

func contentType(key string) string {
	ext := strings.ToLower(path.Ext(key))
	if t, ok := extraTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// hashFile returns the hex SHA-256 and size of a file.
func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
