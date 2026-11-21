//go:build !linux

package main

import "fmt"

// rawTerminal is a stub for platforms without an ioctl implementation here.
type rawTerminal struct{}

func enterRaw() (*rawTerminal, error) {
	return nil, fmt.Errorf("interactive viewer is only supported on Linux")
}

func (t *rawTerminal) restore()                   {}
func (t *rawTerminal) size() (cols, rows int)     { return 80, 24 }
func (t *rawTerminal) readKey() viewerKey         { return keyQuit }
func (t *rawTerminal) readByte(bool) (byte, bool) { return 0, false }
