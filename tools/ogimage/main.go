// Command ogimage draws the site's link-preview image (Open Graph and Twitter
// card), 1200×630, with the Go fonts, so it can be rebuilt byte for byte:
//
//	go run ./tools/ogimage -o internal/web/static/og.png
//
// It is one static image for every page. Nothing per home is ever drawn:
// previews are cached by third parties, so no address or PIN may appear in one.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	width, height = 1200, 630
	margin        = 96
	tagline       = "Free, open-source Cook County property tax appeal check"
)

// The site's palette (Tailwind stone-50, emerald-600/700, slate-900/600).
var (
	background = color.RGBA{0xfa, 0xfa, 0xf9, 0xff}
	emerald    = color.RGBA{0x05, 0x96, 0x69, 0xff}
	emeraldDk  = color.RGBA{0x04, 0x78, 0x57, 0xff}
	ink        = color.RGBA{0x0f, 0x17, 0x2a, 0xff}
	muted      = color.RGBA{0x47, 0x55, 0x69, 0xff}
)

func main() {
	out := flag.String("o", "internal/web/static/og.png", "output PNG")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "ogimage:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	// Emerald band along the bottom edge.
	draw.Draw(img, image.Rect(0, height-20, width, height), image.NewUniform(emerald), image.Point{}, draw.Src)

	bold, err := face(gobold.TTF, 112)
	if err != nil {
		return err
	}
	badgeFont, err := face(gobold.TTF, 64)
	if err != nil {
		return err
	}
	body, err := face(goregular.TTF, 48)
	if err != nil {
		return err
	}
	site, err := face(gomedium.TTF, 36)
	if err != nil {
		return err
	}

	// The "AG" badge, as in the site's header, and the name beside it.
	const badge, badgeTop = 136, 120
	roundedRect(img, image.Rect(margin, badgeTop, margin+badge, badgeTop+badge), 28, emerald)
	w := font.MeasureString(badgeFont, "AG").Ceil()
	text(img, badgeFont, color.White, margin+(badge-w)/2, badgeTop+badge/2+23, "AG")
	text(img, bold, ink, margin+badge+40, badgeTop+badge/2+40, "AssessGap")

	y := 380
	for _, line := range balance(body, tagline, width-2*margin) {
		text(img, body, muted, margin, y, line)
		y += 64
	}
	text(img, site, emeraldDk, margin, height-72, "assessgap.com")

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func face(ttf []byte, size float64) (font.Face, error) {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

// text draws s with its baseline at y.
func text(dst draw.Image, f font.Face, c color.Color, x, y int, s string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// wrap splits s into lines no wider than max pixels.
func wrap(f font.Face, s string, max int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := strings.TrimSpace(line + " " + word)
		if line != "" && font.MeasureString(f, next).Ceil() > max {
			lines = append(lines, line)
			next = word
		}
		line = next
	}
	return append(lines, line)
}

// balance wraps s in as few lines as wrap would, but as narrow as possible,
// so the last line isn't a short orphan.
func balance(f font.Face, s string, max int) []string {
	best := wrap(f, s, max)
	for w := max - 8; w > 0; w -= 8 {
		lines := wrap(f, s, w)
		if len(lines) > len(best) {
			break
		}
		best = lines
	}
	return best
}

// roundedRect fills r with corners of radius rad, antialiased by 4×4 supersampling.
func roundedRect(dst *image.RGBA, r image.Rectangle, rad int, c color.RGBA) {
	inside := func(px, py float64) bool {
		cx := min(max(px, float64(r.Min.X+rad)), float64(r.Max.X-rad))
		cy := min(max(py, float64(r.Min.Y+rad)), float64(r.Max.Y-rad))
		dx, dy := px-cx, py-cy
		return dx*dx+dy*dy <= float64(rad*rad)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			n := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					if inside(float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4) {
						n++
					}
				}
			}
			if n == 0 {
				continue
			}
			a := uint32(n) * 0xffff / 16
			draw.DrawMask(dst, image.Rect(x, y, x+1, y+1), image.NewUniform(c), image.Point{},
				image.NewUniform(color.Alpha16{A: uint16(a)}), image.Point{}, draw.Over)
		}
	}
}
