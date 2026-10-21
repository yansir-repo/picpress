package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

// Frame extraction parameters (F7).
const (
	frameLongEdge   = 1280
	frameQuality    = 78
	contactColumns  = 4
	contactCellEdge = 320
	trimFraction    = 0.02
)

// VideoProducer extracts frames locally with ffmpeg. The original is read
// only by ffmpeg on this machine and is never an artifact (F7).
type VideoProducer struct {
	FFmpeg  string
	FFprobe string
	Frames  int
}

func (p VideoProducer) Params() string {
	return fmt.Sprintf("frames=%d;edge=%d;q=%d", p.Frames, frameLongEdge, frameQuality)
}

// FindFFmpeg looks for ffmpeg/ffprobe bundled next to the executable (macOS:
// Contents/Resources), then on PATH. Empty strings mean video is unsupported.
func FindFFmpeg() (ffmpeg, ffprobe string) {
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	var dirs []string
	if self, err := os.Executable(); err == nil {
		d := filepath.Dir(self)
		dirs = append(dirs, d, filepath.Join(d, "..", "Resources"))
	}
	for _, d := range dirs {
		a, b := filepath.Join(d, "ffmpeg"+exe), filepath.Join(d, "ffprobe"+exe)
		if fileExists(a) && fileExists(b) {
			return a, b
		}
	}
	a, errA := exec.LookPath("ffmpeg")
	b, errB := exec.LookPath("ffprobe")
	if errA != nil || errB != nil {
		return "", ""
	}
	return a, b
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

type probeResult struct {
	Streams []struct {
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func (p VideoProducer) Produce(ctx context.Context, e Entry, stageDir string) (Output, error) {
	fail := func(detail string) (Output, error) {
		return Output{}, &FileError{Reason: ReasonDecode, Detail: detail}
	}
	raw, err := exec.CommandContext(ctx, p.FFprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,avg_frame_rate,r_frame_rate:format=duration",
		"-of", "json", e.Path).Output()
	if err != nil {
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return fail("视频读不出来（可能已损坏或加密）")
	}
	var pr probeResult
	if json.Unmarshal(raw, &pr) != nil || len(pr.Streams) == 0 {
		return fail("视频里没有画面")
	}
	st := pr.Streams[0]
	dur, _ := strconv.ParseFloat(pr.Format.Duration, 64)
	if dur <= 0 {
		return fail("读不出视频时长")
	}
	fps := parseRate(st.AvgFrameRate)
	if fps == 0 {
		fps = parseRate(st.RFrameRate)
	}

	n := p.Frames
	if n <= 0 {
		n = DefaultVideoFrames
	}
	if short := int(dur); short < n {
		n = max(short, 1) // at most one frame per second for short clips
	}

	dir := e.Key + framesSuffix
	info := &VideoInfo{
		DurationSec: round3(dur), Width: st.Width, Height: st.Height, FPS: round3(fps), Codec: st.CodecName,
		SizeBytes: e.Size, OriginalName: path.Base(e.Source), OriginalPath: e.Source,
	}
	var arts []Artifact
	var thumbs []image.Image
	start, end := dur*trimFraction, dur*(1-trimFraction)
	for i := 0; i < n; i++ {
		at := start + (end-start)*(float64(i)+0.5)/float64(n)
		img, err := p.grab(ctx, e.Path, at)
		if err != nil {
			if ctx.Err() != nil {
				return Output{}, ctx.Err()
			}
			return fail("抽帧失败")
		}
		img = fitInside(img, frameLongEdge)
		name := fmt.Sprintf("frame-%02d.jpg", i+1)
		data, err := encodeJPEG(img, frameQuality)
		if err != nil {
			return fail("抽帧失败")
		}
		a, err := stageBytes(stageDir, dir+"/"+name, data)
		if err != nil {
			return Output{}, err
		}
		arts = append(arts, a)
		thumbs = append(thumbs, fitInside(img, contactCellEdge))
		info.Frames = append(info.Frames, FrameStamp{File: name, AtSec: round3(at)})
	}

	contact, err := encodeJPEG(contactSheet(thumbs), frameQuality)
	if err != nil {
		return fail("生成联系表失败")
	}
	a, err := stageBytes(stageDir, dir+"/contact.jpg", contact)
	if err != nil {
		return Output{}, err
	}
	arts = append(arts, a)

	meta, _ := json.MarshalIndent(info, "", "  ")
	a, err = stageBytes(stageDir, dir+"/meta.json", meta)
	if err != nil {
		return Output{}, err
	}
	arts = append(arts, a)
	return Output{Artifacts: arts, Video: info}, nil
}

func (p VideoProducer) grab(ctx context.Context, file string, at float64) (image.Image, error) {
	out, err := exec.CommandContext(ctx, p.FFmpeg, "-nostdin", "-v", "error",
		"-ss", strconv.FormatFloat(at, 'f', 3, 64), "-i", file,
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(out))
}

func fitInside(img image.Image, edge int) image.Image {
	b := img.Bounds()
	if b.Dx() <= edge && b.Dy() <= edge {
		return img
	}
	return imaging.Fit(img, edge, edge, imaging.Lanczos)
}

func contactSheet(thumbs []image.Image) image.Image {
	cols := min(contactColumns, len(thumbs))
	rows := (len(thumbs) + cols - 1) / cols
	cw, ch := 0, 0
	for _, t := range thumbs {
		cw = max(cw, t.Bounds().Dx())
		ch = max(ch, t.Bounds().Dy())
	}
	sheet := image.NewRGBA(image.Rect(0, 0, cols*cw, rows*ch))
	for i, t := range thumbs {
		x, y := (i%cols)*cw, (i/cols)*ch
		draw.Draw(sheet, t.Bounds().Sub(t.Bounds().Min).Add(image.Pt(x, y)), t, t.Bounds().Min, draw.Src)
	}
	return sheet
}

func encodeJPEG(img image.Image, q int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	a, err1 := strconv.ParseFloat(num, 64)
	if !ok {
		if err1 != nil {
			return 0
		}
		return a
	}
	b, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || b == 0 {
		return 0
	}
	return a / b
}

func round3(f float64) float64 { return float64(int64(f*1000+0.5)) / 1000 }
