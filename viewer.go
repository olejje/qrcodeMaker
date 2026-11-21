package main

import (
	"bufio"
	"fmt"
	"os"

	"qrterm/qr"
)

type cellPair struct{ upper, lower bool }

type viewerKey int

const (
	keyQuit viewerKey = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyZoomIn
	keyZoomOut
	keyColor
	keyInvert
	keyReset
	keyUnknown
)

// runViewer opens an alternate screen and lets the user pan and zoom the code.
func runViewer(m *qr.Matrix, useColor, invert bool, border int) error {
	term, err := enterRaw()
	if err != nil {
		return fmt.Errorf("interactive viewer: %w", err)
	}
	defer term.restore()

	out := bufio.NewWriter(os.Stdout)
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l") // alternate screen, hide cursor
	defer func() {
		fmt.Fprint(out, "\x1b[0m\x1b[?25h\x1b[?1049l")
		out.Flush()
	}()

	cols, rows := term.size()
	if rows < 3 {
		rows = 3
	}
	zoom := 2
	offX, offY := -border, -border

	for {
		drawView(out, m, cols, rows-1, offX, offY, zoom, useColor, invert)
		status := fmt.Sprintf(" QR v%d-%s | arrows:move +/-:zoom c:%t i:%t r:reset q:quit | z%d",
			m.Version, m.EC, useColor, invert, zoom)
		fmt.Fprintf(out, "\x1b[%d;1H%s\x1b[K", rows, truncate(status, cols))
		out.Flush()

		switch term.readKey() {
		case keyQuit:
			return nil
		case keyUp:
			offY--
		case keyDown:
			offY++
		case keyLeft:
			offX--
		case keyRight:
			offX++
		case keyZoomIn:
			if zoom < 16 {
				zoom *= 2
			}
		case keyZoomOut:
			if zoom > 1 {
				zoom /= 2
			}
		case keyColor:
			useColor = !useColor
		case keyInvert:
			invert = !invert
		case keyReset:
			zoom, offX, offY = 2, -border, -border
		}
	}
}

func drawView(w *bufio.Writer, m *qr.Matrix, cols, rows, offX, offY, zoom int, useColor, invert bool) {
	fmt.Fprint(w, "\x1b[H")
	var last cellPair
	haveLast := false

	for row := 0; row < rows; row++ {
		my := offY + row*2/zoom
		for col := 0; col < cols; col++ {
			mx := offX + col/zoom
			upper := m.Get(mx, my) != invert
			lower := m.Get(mx, my+1) != invert
			viewCell(w, upper, lower, useColor, &last, &haveLast)
		}
		fmt.Fprint(w, "\x1b[0m\x1b[K\n")
		haveLast = false
	}
}

func viewCell(w *bufio.Writer, upper, lower, useColor bool, last *cellPair, haveLast *bool) {
	if !useColor {
		switch {
		case upper && lower:
			w.WriteString("█")
		case upper:
			w.WriteString("▀")
		case lower:
			w.WriteString("▄")
		default:
			w.WriteString(" ")
		}
		return
	}
	if !*haveLast || last.upper != upper || last.lower != lower {
		fmt.Fprintf(w, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm",
			inkRGB(upper), inkRGB(upper), inkRGB(upper),
			inkRGB(lower), inkRGB(lower), inkRGB(lower))
		last.upper, last.lower = upper, lower
		*haveLast = true
	}
	w.WriteString("▀")
}

// inkRGB mirrors the normal viewer palette: ink is black, paper is white.
func inkRGB(ink bool) int {
	if ink {
		return 0
	}
	return 255
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:1]
	}
	return s[:width-1] + "›"
}
