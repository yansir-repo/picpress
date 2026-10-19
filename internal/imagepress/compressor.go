package imagepress

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
)

// Encoded is the result of encoding an image.
type Encoded struct {
	Data   []byte
	Format Format
	Width  int
	Height int
}

// Compress resizes and encodes an image. In auto mode several encoders are
// tried and the smallest result wins.
func Compress(img image.Image, opts Options) (Encoded, error) {
	opts = opts.Normalized()
	img = Resize(img, opts)
	if img == nil {
		return Encoded{}, fmt.Errorf("image is empty")
	}
	b := img.Bounds()

	alpha := HasAlpha(img)

	candidates := make([]Format, 0, 3)
	switch opts.Format {
	case FormatAuto:
		if alpha {
			candidates = append(candidates, FormatWebP, FormatPNG)
		} else {
			candidates = append(candidates, FormatWebP, FormatJPEG)
		}
	case FormatJPEG:
		candidates = append(candidates, FormatJPEG)
	case FormatPNG:
		candidates = append(candidates, FormatPNG)
	case FormatWebP:
		candidates = append(candidates, FormatWebP)
	case FormatAVIF:
		candidates = append(candidates, FormatAVIF)
	}

	var best Encoded
	var firstErr error
	for _, f := range candidates {
		data, err := Encode(img, f, opts)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", f.Label(), err)
			}
			continue
		}
		if best.Data == nil || len(data) < len(best.Data) {
			best = Encoded{Data: data, Format: f, Width: b.Dx(), Height: b.Dy()}
		}
	}

	if best.Data == nil {
		if firstErr != nil {
			return Encoded{}, firstErr
		}
		return Encoded{}, fmt.Errorf("no encoder available")
	}
	return best, nil
}

// Encode encodes img in the requested format. Transparency is flattened onto
// the configured background colour for formats that cannot store alpha.
func Encode(img image.Image, format Format, opts Options) ([]byte, error) {
	opts = opts.Normalized()
	var buf bytes.Buffer

	switch format {
	case FormatJPEG:
		if HasAlpha(img) {
			r, g, b := opts.BackgroundColor()
			img = Flatten(img, r, g, b)
		}
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: opts.Quality}); err != nil {
			return nil, err
		}
	case FormatPNG:
		// PNG is lossless; quality is not applicable.
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, err
		}
	case FormatWebP:
		if err := webp.Encode(&buf, img, webp.Options{
			Quality: opts.Quality,
			Method:  4,
		}); err != nil {
			return nil, err
		}
	case FormatAVIF:
		if err := avif.Encode(&buf, img, avif.Options{
			Quality:      opts.Quality,
			QualityAlpha: opts.Quality,
			Speed:        8,
		}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}

	if buf.Len() == 0 {
		return nil, fmt.Errorf("encoder produced no data")
	}
	return buf.Bytes(), nil
}

// DecodeEncoded decodes freshly encoded bytes so the preview reflects the
// actual compression artefacts.
func DecodeEncoded(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}
