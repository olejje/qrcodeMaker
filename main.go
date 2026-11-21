// qrterm prints QR codes in the terminal and saves them as PNG or SVG.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
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

// readText reads arguments, or one terminal line / whole pipe from stdin.
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
