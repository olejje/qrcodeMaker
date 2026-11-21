package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"

	"qrterm/qr"
)

// loadLogo decodes a PNG or JPEG image to be placed in the center of the code.
func loadLogo(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open logo %s: %w", path, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode logo %s: %w", path, err)
	}
	return img, nil
}

// logoRegion picks an even-sized square in the center of the symbol. It is
// deliberately small enough to leave the finder patterns untouched.
func logoRegion(m *qr.Matrix) (mods, x0, y0 int) {
	mods = int(float64(m.Size) * 0.18)
	mods -= mods % 2
	if mods < 2 {
		mods = 2
	}
	if m.Size-8 < mods {
		mods = m.Size - 8
		mods -= mods % 2
		if mods < 2 {
			mods = 2
		}
	}
	return mods, (m.Size - mods) / 2, (m.Size - mods) / 2
}

// drawLogoPixels clears a small white frame around the center and paints a
// nearest-neighbor scaled copy of the logo inside it.
func drawLogoPixels(dst *image.RGBA, logo image.Image, m *qr.Matrix, border, scale int, paper color.Color) {
	mods, x0, y0 := logoRegion(m)
	outer := (mods + 2) * scale
	ox, oy := (x0-1+border)*scale, (y0-1+border)*scale
	fillRect(dst, ox, oy, outer, outer, paper)

	inner := mods * scale
	ix, iy := ox+scale, oy+scale
	src := logo.Bounds()
	for y := 0; y < inner; y++ {
		sy := src.Min.Y + y*src.Dy()/inner
		for x := 0; x < inner; x++ {
			sx := src.Min.X + x*src.Dx()/inner
			dst.Set(ix+x, iy+y, blendOver(paper, logo.At(sx, sy)))
		}
	}
}

func fillRect(img *image.RGBA, x0, y0, w, h int, c color.Color) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			if x >= 0 && y >= 0 && x < img.Bounds().Dx() && y < img.Bounds().Dy() {
				img.Set(x, y, c)
			}
		}
	}
}

func blendOver(back, front color.Color) color.Color {
	br, bg, bb, _ := back.RGBA()
	fr, fg, fb, fa := front.RGBA()
	inv := 0xffff - fa
	return color.RGBA{
		R: uint8((fr + br*inv/0xffff) >> 8),
		G: uint8((fg + bg*inv/0xffff) >> 8),
		B: uint8((fb + bb*inv/0xffff) >> 8),
		A: 255,
	}
}

// writeSVG emits the symbol as an SVG. When a logo is given, it is embedded
// as a base64 PNG data URI so the file stays self-contained.
func writeSVG(path string, m *qr.Matrix, invert bool, border int, logo image.Image) error {
	side := m.Size + 2*border
	bg, fg := "white", "black"
	if invert {
		bg, fg = "black", "white"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, side, side, side, side)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`, bg)
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if m.Get(x, y) {
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="1" height="1" fill="%s"/>`, x+border, y+border, fg)
			}
		}
	}

	if logo != nil {
		mods, x0, y0 := logoRegion(m)
		paper := "white"
		if invert {
			paper = "black"
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x0-1+border, y0-1+border, mods+2, mods+2, paper)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, logo); err != nil {
			return err
		}
		uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
		fmt.Fprintf(&b, `<image x="%d" y="%d" width="%d" height="%d" preserveAspectRatio="none" href="%s"/>`, x0+border, y0+border, mods, mods, uri)
	}
	b.WriteString("</svg>\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
