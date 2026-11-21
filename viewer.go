package main

import (
	"bufio"
	"fmt"
	"os"

	"qrterm/qr"
)

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

func runViewer(m *qr.Matrix, useColor, invert bool, border int) error {
	term, err := enterRaw()
	if err != nil {
		return fmt.Errorf("interactive viewer: %w", err)
	}
	defer term.restore()

	out := bufio.NewWriter(os.Stdout)
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	defer func() {
		fmt.Fprint(out, "\x1b[0m\x1b[?25h\x1b[?1049l")
		out.Flush()
	}()

	cols, rows := term.size()
	if rows < 3 {
		rows = 3
	}
	zoom, offX, offY := 2, -border, -border
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
	var last moduleCell
	haveLast := false
	for row := 0; row < rows; row++ {
		my := offY + row*2/zoom
		for col := 0; col < cols; col++ {
			mx := offX + col/zoom
			writeCell(w, m.Get(mx, my), m.Get(mx, my+1), useColor, invert, &last, &haveLast)
		}
		fmt.Fprint(w, "\x1b[0m\x1b[K\n")
		haveLast = false
	}
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width == 1 {
		return s[:1]
	}
	return s[:width-1] + "›"
}
