//go:build linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// rawTerminal wraps stdin in raw mode: no echo, no line buffering, no signals.
// The caller must call restore before exiting.
type rawTerminal struct {
	f     *os.File
	fd    int
	old   syscall.Termios
	state syscall.Termios
}

func enterRaw() (*rawTerminal, error) {
	f := os.Stdin
	fd := int(f.Fd())
	var old syscall.Termios
	if err := rawIOCtl(fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	state := old
	state.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	state.Oflag &^= syscall.OPOST
	state.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	state.Cflag &^= syscall.CSIZE | syscall.PARENB
	state.Cflag |= syscall.CS8
	state.Cc[syscall.VMIN] = 1
	state.Cc[syscall.VTIME] = 0
	if err := rawIOCtl(fd, syscall.TCSETS, unsafe.Pointer(&state)); err != nil {
		return nil, err
	}
	return &rawTerminal{f: f, fd: fd, old: old, state: state}, nil
}

func rawIOCtl(fd int, request uint, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(request), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func (t *rawTerminal) restore() {
	_ = rawIOCtl(t.fd, syscall.TCSETS, unsafe.Pointer(&t.old))
}

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

func (t *rawTerminal) size() (cols, rows int) {
	var ws winsize
	if rawIOCtl(t.fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)) == nil && ws.Col > 0 && ws.Row > 0 {
		return int(ws.Col), int(ws.Row)
	}
	return 80, 24
}

func (t *rawTerminal) readKey() viewerKey {
	b, ok := t.readByte(false)
	if !ok {
		return keyQuit
	}
	switch b {
	case 'q', 'Q', 3, 13, ' ':
		return keyQuit
	case 27: // ESC or an escape sequence
		b2, ok := t.readByte(true)
		if !ok {
			return keyQuit
		}
		if b2 != '[' && b2 != 'O' {
			return keyUnknown
		}
		b3, ok := t.readByte(true)
		if !ok {
			return keyUnknown
		}
		switch b3 {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		case 'C':
			return keyRight
		case 'D':
			return keyLeft
		}
	case '+', '=':
		return keyZoomIn
	case '-', '_':
		return keyZoomOut
	case 'c', 'C':
		return keyColor
	case 'i', 'I':
		return keyInvert
	case 'r', 'R':
		return keyReset
	case 'k', 'K':
		return keyUp
	case 'j', 'J':
		return keyDown
	case 'h', 'H':
		return keyLeft
	case 'l', 'L':
		return keyRight
	}
	return keyUnknown
}

func (t *rawTerminal) readByte(withTimeout bool) (byte, bool) {
	if withTimeout {
		current := t.state
		current.Cc[syscall.VMIN] = 0
		current.Cc[syscall.VTIME] = 1 // 100 ms
		if rawIOCtl(t.fd, syscall.TCSETS, unsafe.Pointer(&current)) == nil {
			defer rawIOCtl(t.fd, syscall.TCSETS, unsafe.Pointer(&t.state))
		}
	}
	var buf [1]byte
	n, err := t.f.Read(buf[:])
	if n == 0 || err != nil {
		return 0, false
	}
	return buf[0], true
}
