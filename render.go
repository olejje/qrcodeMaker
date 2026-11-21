package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"

	"qrterm/qr"
)

type moduleCell struct{ upper, lower bool }

type outputConfig struct {
	color, invert, view bool
	border, scale       int
	png, svg            string
	logo                image.Image
}

func emit(stdout io.Writer, m *qr.Matrix, c outputConfig) error {
	if c.view {
		if err := runViewer(m, c.color, c.invert, c.border); err != nil {
			return err
		}
	} else if err := renderTerminal(stdout, m, c.color, c.invert, c.border); err != nil {
		return err
	}
	if c.png != "" {
		if err := writePNG(c.png, m, c.invert, c.border, c.scale, c.logo); err != nil {
			return err
		}
	}
	if c.svg != "" {
		if err := writeSVG(c.svg, m, c.invert, c.border, c.logo); err != nil {
			return err
		}
	}
	return nil
}

var halfBlockANSI = func() [2][4]string {
	var table [2][4]string
	for inv := 0; inv < 2; inv++ {
		for upper := 0; upper < 2; upper++ {
			for lower := 0; lower < 2; lower++ {
				u, l := shade(upper == 1, inv == 1), shade(lower == 1, inv == 1)
				table[inv][upper<<1|lower] = fmt.Sprintf(
					"\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm", u, u, u, l, l, l)
			}
		}
	}
	return table
}()

func shade(dark, invert bool) int {
	if dark != invert {
		return 0
	}
	return 255
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func writeCell(w io.Writer, upper, lower, useColor, invert bool, last *moduleCell, haveLast *bool) {
	if !useColor {
		u, l := upper != invert, lower != invert
		switch {
		case u && l:
			io.WriteString(w, "█")
		case u:
			io.WriteString(w, "▀")
		case l:
			io.WriteString(w, "▄")
		default:
			io.WriteString(w, " ")
		}
		return
	}
	cell := moduleCell{upper, lower}
	if !*haveLast || cell != *last {
		io.WriteString(w, halfBlockANSI[boolInt(invert)][boolInt(upper)<<1|boolInt(lower)])
		*last, *haveLast = cell, true
	}
	io.WriteString(w, "▀")
}

func renderTerminal(w io.Writer, m *qr.Matrix, useColor, invert bool, border int) error {
	out := bufio.NewWriter(w)
	width := m.Size + 2*border
	var last moduleCell
	haveLast := false

	for y := 0; y < width; y += 2 {
		for x := 0; x < width; x++ {
			upper := m.Get(x-border, y-border)
			lower := false
			if y+1 < width {
				lower = m.Get(x-border, y+1-border)
			}
			writeCell(out, upper, lower, useColor, invert, &last, &haveLast)
		}
		if useColor {
			out.WriteString("\x1b[0m")
			haveLast = false
		}
		out.WriteByte('\n')
	}
	return out.Flush()
}

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

func logoRegion(m *qr.Matrix) (mods, x0, y0 int) {
	mods = int(float64(m.Size)*0.18) &^ 1
	if mods < 2 {
		mods = 2
	}
	if max := m.Size - 8; mods > max {
		mods = max &^ 1
		if mods < 2 {
			mods = 2
		}
	}
	return mods, (m.Size - mods) / 2, (m.Size - mods) / 2
}

func rgba(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

func fillRect(img *image.RGBA, x0, y0, w, h int, c color.RGBA) {
	r := image.Rect(x0, y0, x0+w, y0+h).Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := img.PixOffset(r.Min.X, y)
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
			i += 4
		}
	}
}

func blendOver(back, front color.Color) color.RGBA {
	br, bg, bb, _ := back.RGBA()
	fr, fg, fb, fa := front.RGBA()
	inv := 0xffff - fa
	return color.RGBA{
		uint8((fr + br*inv/0xffff) >> 8),
		uint8((fg + bg*inv/0xffff) >> 8),
		uint8((fb + bb*inv/0xffff) >> 8),
		255,
	}
}

func drawLogoPixels(dst *image.RGBA, logo image.Image, m *qr.Matrix, border, scale int, paper color.RGBA) {
	mods, x0, y0 := logoRegion(m)
	ox, oy := (x0-1+border)*scale, (y0-1+border)*scale
	fillRect(dst, ox, oy, (mods+2)*scale, (mods+2)*scale, paper)

	inner := mods * scale
	src := logo.Bounds()
	for y := 0; y < inner; y++ {
		sy := src.Min.Y + y*src.Dy()/inner
		for x := 0; x < inner; x++ {
			sx := src.Min.X + x*src.Dx()/inner
			dst.Set(ox+scale+x, oy+scale+y, blendOver(paper, logo.At(sx, sy)))
		}
	}
}

func writePNG(path string, m *qr.Matrix, invert bool, border, scale int, logo image.Image) error {
	side := (m.Size + 2*border) * scale
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	ink, paper := rgba(color.Black), rgba(color.White)
	if invert {
		ink, paper = paper, ink
	}
	fillRect(img, 0, 0, side, side, paper)
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if m.Get(x, y) {
				fillRect(img, (x+border)*scale, (y+border)*scale, scale, scale, ink)
			}
		}
	}
	if logo != nil {
		drawLogoPixels(img, logo, m, border, scale, paper)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

func writeSVG(path string, m *qr.Matrix, invert bool, border int, logo image.Image) error {
	side := m.Size + 2*border
	bg, fg := "white", "black"
	if invert {
		bg, fg = "black", "white"
	}

	var b strings.Builder
	b.Grow(128 + m.Size*m.Size*48)
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
