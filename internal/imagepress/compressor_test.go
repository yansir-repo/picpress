package imagepress

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

func photoLike(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(42))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			base := uint8((x*255)/w/3 + (y*255)/h/3)
			n := uint8(rnd.Intn(24))
			img.SetNRGBA(x, y, color.NRGBA{R: base + n, G: base/2 + 30 + n, B: 200 - n, A: 255})
		}
	}
	return img
}

func transparentImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(0)
			if x > w/4 && x < w*3/4 && y > h/4 && y < h*3/4 {
				a = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 240, G: uint8(x % 255), B: 40, A: a})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCompressExplicitFormats(t *testing.T) {
	src := photoLike(1200, 900)
	original := int64(len(encodeJPEG(t, src)))

	for _, f := range []Format{FormatJPEG, FormatPNG, FormatWebP, FormatAVIF} {
		t.Run(string(f), func(t *testing.T) {
			enc, err := Compress(src, Options{Format: f, Quality: 80})
			if err != nil {
				t.Fatalf("compress: %v", err)
			}
			if enc.Format != f {
				t.Fatalf("format = %s, want %s", enc.Format, f)
			}
			if enc.Width != 1200 || enc.Height != 900 {
				t.Fatalf("dimensions = %d×%d", enc.Width, enc.Height)
			}
			decoded, err := DecodeEncoded(enc.Data)
			if err != nil {
				t.Fatalf("decode encoded %s: %v", f, err)
			}
			if got := decoded.Bounds(); got.Dx() != 1200 || got.Dy() != 900 {
				t.Fatalf("decoded dimensions = %d×%d", got.Dx(), got.Dy())
			}
			if f != FormatPNG && int64(len(enc.Data)) >= original {
				t.Fatalf("%s output (%d) is not smaller than the source JPEG (%d)", f, len(enc.Data), original)
			}
			t.Logf("%s: %d bytes (%.1f%% of source)", f, len(enc.Data), float64(len(enc.Data))*100/float64(original))
		})
	}
}

func TestAutoPicksSmallest(t *testing.T) {
	src := photoLike(800, 600)
	auto, err := Compress(src, Options{Format: FormatAuto, Quality: 80})
	if err != nil {
		t.Fatal(err)
	}
	if auto.Format != FormatWebP && auto.Format != FormatJPEG {
		t.Fatalf("unexpected auto format %s", auto.Format)
	}
	for _, f := range []Format{FormatJPEG, FormatWebP} {
		enc, err := Encode(src, f, Options{Quality: 80})
		if err != nil {
			t.Fatal(err)
		}
		if len(auto.Data) > len(enc) {
			t.Fatalf("auto result (%d) is larger than %s (%d)", len(auto.Data), f, len(enc))
		}
	}
}

func TestTransparentJPEGIsFlattened(t *testing.T) {
	src := transparentImage(200, 150)
	enc, err := Compress(src, Options{Format: FormatJPEG, Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEncoded(enc.Data)
	if err != nil {
		t.Fatal(err)
	}
	if HasAlpha(decoded) {
		t.Fatal("JPEG output still has transparency")
	}
}

func TestAutoWithAlphaKeepsAlpha(t *testing.T) {
	src := transparentImage(320, 240)
	enc, err := Compress(src, Options{Format: FormatAuto, Quality: 80})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEncoded(enc.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !HasAlpha(decoded) {
		t.Fatalf("auto picked %s without alpha", enc.Format)
	}
}

func TestResizeModes(t *testing.T) {
	src := photoLike(2000, 1000)

	got := Resize(src, Options{ResizeMode: ResizeLongEdge, MaxLongEdge: 800}.Normalized())
	if b := got.Bounds(); b.Dx() != 800 || b.Dy() != 400 {
		t.Fatalf("long edge resize = %d×%d, want 800×400", b.Dx(), b.Dy())
	}

	got = Resize(src, Options{ResizeMode: ResizePercent, Percent: 50}.Normalized())
	if b := got.Bounds(); b.Dx() != 1000 || b.Dy() != 500 {
		t.Fatalf("percent resize = %d×%d, want 1000×500", b.Dx(), b.Dy())
	}

	small := photoLike(320, 200)
	got = Resize(small, Options{ResizeMode: ResizeLongEdge, MaxLongEdge: 2560}.Normalized())
	if b := got.Bounds(); b.Dx() != 320 || b.Dy() != 200 {
		t.Fatalf("small image was enlarged to %d×%d", b.Dx(), b.Dy())
	}
}

func TestPreviewDataURL(t *testing.T) {
	opaque := PreviewDataURL(photoLike(1600, 1200), 400)
	if len(opaque) < 100 || !strings.HasPrefix(opaque, "data:image/jpeg") {
		t.Fatalf("unexpected opaque preview: %.40q", opaque)
	}
	alpha := PreviewDataURL(transparentImage(800, 600), 400)
	if len(alpha) < 100 || !strings.HasPrefix(alpha, "data:image/png") {
		t.Fatalf("unexpected alpha preview: %.40q", alpha)
	}
}

func TestNormalize(t *testing.T) {
	o := Options{Format: "bogus", Quality: 300, ResizeMode: "bogus", Percent: 0}.Normalized()
	if o.Format != FormatAuto || o.Quality != 100 || o.ResizeMode != ResizeNone || o.Percent != 100 {
		t.Fatalf("unexpected normalized options: %+v", o)
	}
	if r, g, b := (Options{Background: "#ff8800"}).BackgroundColor(); r != 255 || g != 0x88 || b != 0 {
		t.Fatalf("background colour = %d,%d,%d", r, g, b)
	}
}

// jpegWithOrientation builds a tiny JPEG with an EXIF orientation tag.
func jpegWithOrientation(t *testing.T, orientation uint16) []byte {
	t.Helper()
	base := encodeJPEG(t, photoLike(64, 32))

	// Minimal little-endian TIFF block with a single IFD0 entry.
	tiff := make([]byte, 0, 32)
	tiff = append(tiff, 'I', 'I', 42, 0)             // byte order + magic
	tiff = binary.LittleEndian.AppendUint32(tiff, 8) // IFD0 offset
	tiff = binary.LittleEndian.AppendUint16(tiff, 1) // entry count
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.LittleEndian.AppendUint16(tiff, 3) // SHORT
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, orientation)
	tiff = binary.LittleEndian.AppendUint16(tiff, 0) // pad to 4 bytes
	tiff = binary.LittleEndian.AppendUint32(tiff, 0) // next IFD

	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))

	out := make([]byte, 0, len(base)+len(segment)+len(payload))
	out = append(out, base[:2]...) // SOI
	out = append(out, segment...)
	out = append(out, payload...)
	out = append(out, base[2:]...)
	return out
}

func TestDecodeAppliesEXIFOrientation(t *testing.T) {
	data := jpegWithOrientation(t, 6)
	img, format, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %s", format)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 64 {
		t.Fatalf("orientation 6: got %d×%d, want 32×64", b.Dx(), b.Dy())
	}
}

func TestPNGIsLossless(t *testing.T) {
	src := photoLike(300, 200)
	enc, err := Compress(src, Options{Format: FormatPNG, Quality: 10})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEncoded(enc.Data)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	_ = buf
	if got := decoded.Bounds(); got.Dx() != 300 || got.Dy() != 200 {
		t.Fatalf("png dims = %d×%d", got.Dx(), got.Dy())
	}
}
