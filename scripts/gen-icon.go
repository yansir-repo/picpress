//go:build ignore

// Generates build/appicon.png. Run with: go run scripts/gen-icon.go
package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"

	"github.com/disintegration/imaging"
)

const (
	base = 1024
	ss   = 4
	size = base * ss
)

func main() {
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))

	// Rounded square background with a warm diagonal gradient.
	margin := 64.0 * ss
	radius := 210.0 * ss
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x), float64(y)
			if !inRounded(fx, fy, margin, margin, size-2*margin, size-2*margin, radius) {
				continue
			}
			t := (fx + fy) / (2 * size)
			c := mix(color.NRGBA{255, 154, 98, 255}, color.NRGBA{232, 69, 27, 255}, t)
			// Soft highlight towards the top-left.
			dx, dy := fx-330*ss, fy-260*ss
			glow := math.Max(0, 1-math.Hypot(dx, dy)/(720*ss))
			c = mix(c, color.NRGBA{255, 236, 220, 255}, glow*0.18)
			set(canvas, x, y, c)
		}
	}

	// Card layer: a white photo print with a warm landscape.
	cw, ch := 700*ss, 600*ss
	card := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	cardX, cardY := 70.0*ss, 90.0*ss
	cardW, cardH := 560.0*ss, 420.0*ss
	cardR := 56.0 * ss

	// Soft shadow under the card.
	for i := 0; i < 9; i++ {
		off := float64(12+i*4) * ss
		grow := float64(i) * 3 * ss
		fillRounded(card, cardX-grow, cardY+off-grow, cardW+2*grow, cardH+2*grow, cardR+grow,
			color.NRGBA{70, 30, 12, 5})
	}

	fillRounded(card, cardX, cardY, cardW, cardH, cardR, color.NRGBA{255, 253, 248, 255})
	strokeRounded(card, cardX, cardY, cardW, cardH, cardR, 1.5*ss, color.NRGBA{120, 70, 40, 26})

	// Inner artwork (a little landscape), clipped to a rounded rectangle.
	artX, artY := 100.0*ss, 120.0*ss
	artW, artH := 500.0*ss, 360.0*ss
	artR := 34.0 * ss
	for y := int(artY); y < int(artY+artH); y++ {
		for x := int(artX); x < int(artX+artW); x++ {
			if !inRounded(float64(x), float64(y), artX, artY, artW, artH, artR) {
				continue
			}
			t := (float64(y) - artY) / artH
			set(card, x, y, mix(color.NRGBA{255, 235, 213, 255}, color.NRGBA{255, 197, 154, 255}, t))
		}
	}

	// Sun.
	fillCircle(card, 468*ss, 218*ss, 52*ss, color.NRGBA{255, 181, 116, 255})

	// Mountain range.
	ridge := []image.Point{
		{100 * ss, 480 * ss},
		{268 * ss, 302 * ss},
		{356 * ss, 390 * ss},
		{432 * ss, 336 * ss},
		{600 * ss, 480 * ss},
	}
	fillPolygon(card, ridge, color.NRGBA{240, 90, 42, 255})
	ridge2 := []image.Point{
		{100 * ss, 480 * ss},
		{226 * ss, 396 * ss},
		{312 * ss, 452 * ss},
		{390 * ss, 410 * ss},
		{600 * ss, 480 * ss},
	}
	fillPolygon(card, ridge2, color.NRGBA{232, 69, 27, 255})

	// Rotate the print slightly and place it on the icon.
	rotated := imaging.Rotate(card, -5.5, color.NRGBA{0, 0, 0, 0})
	dst := image.Rect(
		int(512*ss-rotated.Bounds().Dx()/2),
		int(486*ss-rotated.Bounds().Dy()/2),
		int(512*ss-rotated.Bounds().Dx()/2)+rotated.Bounds().Dx(),
		int(486*ss-rotated.Bounds().Dy()/2)+rotated.Bounds().Dy(),
	)
	draw.Draw(canvas, dst, rotated, rotated.Bounds().Min, draw.Over)

	// Badge with a download / compress arrow.
	bx, by, br := 742.0*ss, 724.0*ss, 118.0*ss
	for i := 0; i < 8; i++ {
		grow := float64(i) * 3 * ss
		fillCircle(canvas, bx, by+float64(10+i*3)*ss, br+grow, color.NRGBA{60, 20, 8, 6})
	}
	fillCircle(canvas, bx, by, br, color.NRGBA{255, 253, 248, 255})
	fillCircle(canvas, bx, by-br*0.38, br*0.62, color.NRGBA{255, 255, 255, 40})

	// Arrow stem and head.
	orange := color.NRGBA{238, 82, 37, 255}
	fillRounded(canvas, (bx - 13*ss), by-78*ss, 26*ss, 96*ss, 13*ss, orange)
	tri := []image.Point{
		{int(bx - 58*ss), int(by - 4*ss)},
		{int(bx + 58*ss), int(by - 4*ss)},
		{int(bx), int(by + 62*ss)},
	}
	fillPolygon(canvas, tri, orange)

	// Downsample for crisp edges.
	out := imaging.Resize(canvas, base, base, imaging.Lanczos)
	f, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := imaging.Encode(f, out, imaging.PNG); err != nil {
		panic(err)
	}
}

func inRounded(x, y, rx, ry, w, h, r float64) bool {
	if x < rx-r || x > rx+w+r || y < ry-r || y > ry+h+r {
		return false
	}
	cx := math.Min(math.Max(x, rx+r), rx+w-r)
	cy := math.Min(math.Max(y, ry+r), ry+h-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func fillRounded(img *image.NRGBA, x, y, w, h, r float64, c color.NRGBA) {
	for yy := int(y - r); yy <= int(y+h+r); yy++ {
		for xx := int(x - r); xx <= int(x+w+r); xx++ {
			if inRounded(float64(xx)+0.5, float64(yy)+0.5, x, y, w, h, r) {
				blend(img, xx, yy, c)
			}
		}
	}
}

func strokeRounded(img *image.NRGBA, x, y, w, h, r, thick float64, c color.NRGBA) {
	for yy := int(y - r); yy <= int(y+h+r); yy++ {
		for xx := int(x - r); xx <= int(x+w+r); xx++ {
			fx, fy := float64(xx)+0.5, float64(yy)+0.5
			if inRounded(fx, fy, x, y, w, h, r) && !inRounded(fx, fy, x+thick, y+thick, w-2*thick, h-2*thick, r-thick) {
				blend(img, xx, yy, c)
			}
		}
	}
}

func fillCircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				blend(img, x, y, c)
			}
		}
	}
}

func fillPolygon(img *image.NRGBA, pts []image.Point, c color.NRGBA) {
	minX, minY := pts[0].X, pts[0].Y
	maxX, maxY := pts[0].X, pts[0].Y
	for _, p := range pts {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInPolygon(float64(x)+0.5, float64(y)+0.5, pts) {
				blend(img, x, y, c)
			}
		}
	}
}

func pointInPolygon(x, y float64, pts []image.Point) bool {
	inside := false
	j := len(pts) - 1
	for i := 0; i < len(pts); i++ {
		xi, yi := float64(pts[i].X), float64(pts[i].Y)
		xj, yj := float64(pts[j].X), float64(pts[j].Y)
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

func set(img *image.NRGBA, x, y int, c color.NRGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	i := img.PixOffset(x, y)
	img.Pix[i] = c.R
	img.Pix[i+1] = c.G
	img.Pix[i+2] = c.B
	img.Pix[i+3] = c.A
}

func blend(img *image.NRGBA, x, y int, c color.NRGBA) {
	if !image.Pt(x, y).In(img.Bounds()) || c.A == 0 {
		return
	}
	if c.A == 255 {
		set(img, x, y, c)
		return
	}
	i := img.PixOffset(x, y)
	a := float64(c.A) / 255
	img.Pix[i] = uint8(float64(c.R)*a + float64(img.Pix[i])*(1-a))
	img.Pix[i+1] = uint8(float64(c.G)*a + float64(img.Pix[i+1])*(1-a))
	img.Pix[i+2] = uint8(float64(c.B)*a + float64(img.Pix[i+2])*(1-a))
	img.Pix[i+3] = uint8(math.Min(255, float64(c.A)+float64(img.Pix[i+3])*(1-a)))
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.NRGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}
