// qrterm prints QR Codes in your terminal (24-bit ANSI half blocks) and can
// also save them as a PNG. No third-party packages are used.
package main

import (
	"bufio"
	"errors"
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

const usageText = `Usage: qrterm [flags] "text"

With no text argument, input is read from stdin: one line in a terminal,
or the whole stream when piped.

Flags:
  -ec L|M|Q|H          error correction level (default M)
  -v auto|1..40        QR version (default auto)
  -mode auto|numeric|alnum|byte|kanji
                       data encoding mode (default auto)
  -eci                 force ECI 26 (UTF-8) in byte mode
  -color               24-bit ANSI colors and half blocks (default true)
  -invert              swap dark and light modules
  -border N            quiet zone in modules (default 4)
  -png FILE            also save the symbol as a PNG
  -svg FILE            also save the symbol as an SVG
  -logo FILE           image to place in the center (forces level H)
  -scale N             pixels per module in the PNG (default 8)
  -view                interactive viewer (arrows, +/-, c, i, q)

Examples:
  go run . "привіт"
  go run . -color=false "https://example.com"
  echo "привіт" | go run .
  go run . -png out.png -svg out.svg -ec H "текст"
  go run . -mode kanji "漢字テスト"
  go run . -logo logo.png -ec H "https://example.com"
  go run . -view -ec H "https://example.com"
`

func printUsage(w io.Writer) {
	io.WriteString(w, usageText)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "qrterm:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("qrterm", flag.ContinueOnError)
	ecFlag := fs.String("ec", "M", "error correction level: L, M, Q or H")
	verFlag := fs.String("v", "auto", "QR version 1-40, or auto")
	modeFlag := fs.String("mode", "auto", "data mode: auto, numeric, alnum, byte or kanji")
	eciFlag := fs.Bool("eci", false, "force ECI 26 (UTF-8) in byte mode")
	colorFlag := fs.Bool("color", true, "use 24-bit ANSI colors and half blocks")
	invertFlag := fs.Bool("invert", false, "swap dark and light modules")
	borderFlag := fs.Int("border", 4, "quiet zone in modules")
	pngFlag := fs.String("png", "", "also write the symbol to this PNG file")
	svgFlag := fs.String("svg", "", "also write the symbol to this SVG file")
	logoFlag := fs.String("logo", "", "center image for PNG/SVG output (forces level H)")
	scaleFlag := fs.Int("scale", 8, "pixels per module in the PNG")
	viewFlag := fs.Bool("view", false, "interactive raw-mode viewer")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return nil
		}
		return err
	}

	ec, err := qr.ParseECLevel(*ecFlag)
	if err != nil {
		return err
	}
	mode, err := qr.ParseMode(*modeFlag)
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
	var logo image.Image
	if *logoFlag != "" {
		logo, err = loadLogo(*logoFlag)
		if err != nil {
			return err
		}
		if ec != qr.ECLevelH {
			fmt.Fprintln(os.Stderr, "qrterm: -logo forces error correction level H")
			ec = qr.ECLevelH
		}
	}
	if *borderFlag < 0 {
		return fmt.Errorf("-border must be non-negative")
	}
	if *scaleFlag < 1 {
		return fmt.Errorf("-scale must be at least 1")
	}

	text, err := readText(fs, os.Stdin)
	if err != nil {
		return err
	}
	if text == "" {
		printUsage(stdout)
		return nil
	}

	matrix, err := qr.Encode(text, qr.Options{EC: ec, Version: version, Mode: mode, ECI: *eciFlag})
	if err != nil {
		return err
	}

	if *viewFlag {
		if err := runViewer(matrix, *colorFlag, *invertFlag, *borderFlag); err != nil {
			return err
		}
	} else if err := renderTerminal(stdout, matrix, *colorFlag, *invertFlag, *borderFlag); err != nil {
		return err
	}
	if *pngFlag != "" {
		if err := writePNG(*pngFlag, matrix, *invertFlag, *borderFlag, *scaleFlag, logo); err != nil {
			return err
		}
	}
	if *svgFlag != "" {
		if err := writeSVG(*svgFlag, matrix, *invertFlag, *borderFlag, logo); err != nil {
			return err
		}
	}
	return nil
}

// readText takes the text from CLI arguments, or from stdin when none are
// given. In a terminal it reads just one line so Enter is enough; for a pipe
// or a redirect it reads everything up to EOF.
func readText(fs *flag.FlagSet, stdin *os.File) (string, error) {
	if text := strings.Join(fs.Args(), " "); text != "" {
		return text, nil
	}
	info, err := stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin: %w", err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		if stdin == os.Stdin {
			fmt.Fprint(os.Stderr, "Text: ")
		}
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(input), "\r\n"), nil
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

func writePNG(path string, m *qr.Matrix, invert bool, border, scale int, logo image.Image) error {
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

	if logo != nil {
		drawLogoPixels(img, logo, m, border, scale, paperColor)
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
