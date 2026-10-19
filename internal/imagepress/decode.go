package imagepress

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/disintegration/imaging"

	// Register the decoders this app understands with image.Decode.
	_ "github.com/gen2brain/avif"
	_ "github.com/gen2brain/heic"
	_ "github.com/gen2brain/webp"
)

// maxPixels guards against decompression bombs from absurdly large images.
const maxPixels = 400 * 1000 * 1000

// Decode reads an image and normalises its orientation using the EXIF tag
// (JPEG) or the container metadata (HEIC/AVIF/WebP where supported).
func Decode(data []byte) (image.Image, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("unsupported or corrupt image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", fmt.Errorf("image has no pixels")
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, "", fmt.Errorf("image is too large to process (%d×%d)", cfg.Width, cfg.Height)
	}

	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	return img, format, nil
}

// HasAlpha reports whether the image contains any partially transparent pixel.
func HasAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

// Flatten composites the image over a solid colour, removing transparency.
func Flatten(img image.Image, r, g, b uint8) *image.RGBA {
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	bg := color.RGBA{R: r, G: g, B: b, A: 0xff}
	draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Over)
	return dst
}
