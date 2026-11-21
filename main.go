// qrterm prints QR Codes in your terminal (24-bit ANSI half blocks) and can
// also save them as a PNG. No third-party packages are used.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"

	"qrterm/qr"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "qrterm:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("qrterm", flag.ContinueOnError)
	ecFlag := fs.String("ec", "M", "error correction level: L, M, Q or H")
	verFlag := fs.String("v", "auto", "QR version 1-10, or auto")
	colorFlag := fs.Bool("color", true, "use 24-bit ANSI colors and half blocks")
	invertFlag := fs.Bool("invert", false, "swap dark and light modules")
	borderFlag := fs.Int("border", 4, "quiet zone in modules")
	pngFlag := fs.String("png", "", "also write the symbol to this PNG file")
	scaleFlag := fs.Int("scale", 8, "pixels per module in the PNG")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ec, err := qr.ParseECLevel(*ecFlag)
	if err != nil {
		return err
	}
	version := 0
	if *verFlag != "auto" {
		version, err = strconv.Atoi(*verFlag)
		if err != nil {
			return fmt.Errorf("invalid -v %q: %w", *verFlag, err)
		}
	}
	if *borderFlag < 0 {
		return fmt.Errorf("-border must be non-negative")
	}
	if *scaleFlag < 1 {
		return fmt.Errorf("-scale must be at least 1")
	}

	text := strings.Join(fs.Args(), " ")
	if text == "" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		text = strings.TrimRight(string(input), "\r\n")
	}
	if text == "" {
		return fmt.Errorf("nothing to encode: pass text as arguments or on stdin")
	}

	matrix, err := qr.Encode(text, qr.Options{EC: ec, Version: version})
	if err != nil {
		return err
	}

	if err := renderTerminal(stdout, matrix, *colorFlag, *invertFlag, *borderFlag); err != nil {
		return err
	}
	if *pngFlag != "" {
		if err := writePNG(*pngFlag, matrix, *invertFlag, *borderFlag, *scaleFlag); err != nil {
			return err
		}
	}
	return nil
}

// renderTerminal draws two module rows per text row using the upper half block.
func renderTerminal(w io.Writer, m *qr.Matrix, useColor, invert bool, border int) error {
	out := bufio.NewWriter(w)
	width := m.Size + 2*border

	inkAt := func(x, y int) bool {
		return m.Get(x-border, y-border) != invert
	}
	rgb := func(ink bool) (int, int, int) {
		if ink != invert {
			return 0, 0, 0 // ink is black in normal mode, white when inverted
		}
		return 255, 255, 255
	}

	type colors struct{ upper, lower bool }
	var last colors
	haveLast := false

	for y := 0; y < width; y += 2 {
		for x := 0; x < width; x++ {
			upper, lower := inkAt(x, y), false
			if y+1 < width {
				lower = inkAt(x, y+1)
			}
			if !useColor {
				switch {
				case upper && lower:
					out.WriteString("█")
				case upper:
					out.WriteString("▀")
				case lower:
					out.WriteString("▄")
				default:
					out.WriteString(" ")
				}
				continue
			}
			pair := colors{upper, lower}
			if !haveLast || pair != last {
				ur, ug, ub := rgb(upper)
				lr, lg, lb := rgb(lower)
				fmt.Fprintf(out, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm", ur, ug, ub, lr, lg, lb)
				last, haveLast = pair, true
			}
			out.WriteString("▀")
		}
		if useColor {
			out.WriteString("\x1b[0m\n")
			haveLast = false
		} else {
			out.WriteString("\n")
		}
	}
	return out.Flush()
}

func writePNG(path string, m *qr.Matrix, invert bool, border, scale int) error {
	side := (m.Size + 2*border) * scale
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	inkColor, paperColor := color.Black, color.White
	if invert {
		inkColor, paperColor = color.White, color.Black
	}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, paperColor)
		}
	}
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if !m.Get(x, y) {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set((x+border)*scale+dx, (y+border)*scale+dy, inkColor)
				}
			}
		}
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
