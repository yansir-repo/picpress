package imagepress

import (
	"image"

	"github.com/disintegration/imaging"
)

// Resize applies the configured resize mode. Images are never enlarged.
func Resize(img image.Image, opts Options) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return img
	}

	var nw, nh int
	switch opts.ResizeMode {
	case ResizeLongEdge:
		longest := w
		if h > longest {
			longest = h
		}
		if longest <= opts.MaxLongEdge {
			return img
		}
		scale := float64(opts.MaxLongEdge) / float64(longest)
		nw = max(1, int(float64(w)*scale+0.5))
		nh = max(1, int(float64(h)*scale+0.5))
	case ResizePercent:
		if opts.Percent >= 100 {
			return img
		}
		scale := float64(opts.Percent) / 100
		nw = max(1, int(float64(w)*scale+0.5))
		nh = max(1, int(float64(h)*scale+0.5))
	default:
		return img
	}

	if nw == w && nh == h {
		return img
	}
	return imaging.Resize(img, nw, nh, imaging.Lanczos)
}
