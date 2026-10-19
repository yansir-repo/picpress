package imagepress

import (
	"bytes"
	"encoding/base64"
	"image"

	"github.com/disintegration/imaging"
)

// PreviewDataURL renders a small, self contained preview image that the
// frontend can display without a second round trip.
func PreviewDataURL(img image.Image, maxEdge int) string {
	if maxEdge <= 0 {
		maxEdge = 720
	}
	thumb := imaging.Fit(img, maxEdge, maxEdge, imaging.Lanczos)

	var buf bytes.Buffer
	if HasAlpha(thumb) {
		_ = imaging.Encode(&buf, thumb, imaging.PNG, imaging.PNGCompressionLevel(1))
		if buf.Len() == 0 {
			return ""
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	_ = imaging.Encode(&buf, thumb, imaging.JPEG, imaging.JPEGQuality(82))
	if buf.Len() == 0 {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
