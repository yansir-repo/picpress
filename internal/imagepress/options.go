// Package imagepress contains the image decoding, resizing and encoding
// pipeline used by the PicPress desktop app.
package imagepress

import (
	"fmt"
	"strconv"
	"strings"
)

// Format identifies an output image format.
type Format string

const (
	// FormatAuto tries a few sensible encoders and keeps the smallest result.
	FormatAuto Format = "auto"
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
)

// Ext returns the usual file extension for the format, including the dot.
func (f Format) Ext() string {
	switch f {
	case FormatJPEG:
		return ".jpg"
	case FormatPNG:
		return ".png"
	case FormatWebP:
		return ".webp"
	case FormatAVIF:
		return ".avif"
	default:
		return ""
	}
}

// Label returns a short human readable name.
func (f Format) Label() string {
	switch f {
	case FormatAuto:
		return "智能"
	case FormatJPEG:
		return "JPEG"
	case FormatPNG:
		return "PNG"
	case FormatWebP:
		return "WebP"
	case FormatAVIF:
		return "AVIF"
	default:
		return string(f)
	}
}

// ResizeMode controls how the output dimensions are derived from the source.
type ResizeMode string

const (
	ResizeNone     ResizeMode = "none"
	ResizeLongEdge ResizeMode = "longest"
	ResizePercent  ResizeMode = "percent"
)

// Options are the user adjustable compression parameters.
type Options struct {
	Format      Format     `json:"format"`
	Quality     int        `json:"quality"`
	ResizeMode  ResizeMode `json:"resizeMode"`
	MaxLongEdge int        `json:"maxLongEdge"`
	Percent     int        `json:"percent"`
	Background  string     `json:"background"`
}

// Normalized clamps values into a safe range and fills in defaults.
func (o Options) Normalized() Options {
	switch o.Format {
	case FormatAuto, FormatJPEG, FormatPNG, FormatWebP, FormatAVIF:
	default:
		o.Format = FormatAuto
	}

	if o.Quality <= 0 {
		o.Quality = 82
	}
	if o.Quality > 100 {
		o.Quality = 100
	}
	if o.Quality < 1 {
		o.Quality = 1
	}

	switch o.ResizeMode {
	case ResizeNone, ResizeLongEdge, ResizePercent:
	default:
		o.ResizeMode = ResizeNone
	}

	if o.MaxLongEdge <= 0 {
		o.MaxLongEdge = 2560
	}
	if o.MaxLongEdge < 32 {
		o.MaxLongEdge = 32
	}
	if o.MaxLongEdge > 30000 {
		o.MaxLongEdge = 30000
	}

	if o.Percent <= 0 {
		o.Percent = 100
	}
	if o.Percent > 100 {
		o.Percent = 100
	}
	if o.Percent < 1 {
		o.Percent = 1
	}

	if o.Background == "" {
		o.Background = "#ffffff"
	}

	return o
}

// BackgroundColor parses the background colour used when flattening
// transparent images for formats without an alpha channel.
func (o Options) BackgroundColor() (uint8, uint8, uint8) {
	hex := strings.TrimSpace(strings.TrimPrefix(o.Background, "#"))
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 255, 255, 255
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 255, 255, 255
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// String is useful in log messages.
func (o Options) String() string {
	return fmt.Sprintf("format=%s quality=%d resize=%s longEdge=%d percent=%d",
		o.Format, o.Quality, o.ResizeMode, o.MaxLongEdge, o.Percent)
}

// FileError describes a file that could not be read or decoded.
type FileError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// AddFilesResult is returned when files are added to the session.
type AddFilesResult struct {
	Items  []ItemInfo  `json:"items"`
	Errors []FileError `json:"errors"`
}

// ItemInfo describes an image that is part of the current session.
type ItemInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Format   string `json:"format"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int64  `json:"bytes"`
	Preview  string `json:"preview"`
	HasAlpha bool   `json:"hasAlpha"`
	Warning  string `json:"warning"`
}

// CompressResult describes one encoded image.
type CompressResult struct {
	ID             string  `json:"id"`
	Format         string  `json:"format"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Bytes          int64   `json:"bytes"`
	OriginalBytes  int64   `json:"originalBytes"`
	OriginalWidth  int     `json:"originalWidth"`
	OriginalHeight int     `json:"originalHeight"`
	SavedPercent   float64 `json:"savedPercent"`
	DurationMS     int64   `json:"durationMs"`
	Preview        string  `json:"preview"`
	OutputPath     string  `json:"outputPath"`
	Warning        string  `json:"warning"`
	Error          string  `json:"error"`
}
