package main

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"strings"
	"testing"

	"qrterm/qr"
)

func TestRenderTerminalPlain(t *testing.T) {
	m, err := qr.Encode("hi", qr.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := renderTerminal(&out, m, false, false, 2); err != nil {
		t.Fatal(err)
	}
	width := m.Size + 4
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != (width+1)/2 {
		t.Fatalf("got %d lines, want %d", len(lines), (width+1)/2)
	}
	for i, line := range lines {
		if len([]rune(line)) != width {
			t.Errorf("line %d has %d runes, want %d", i, len([]rune(line)), width)
		}
		for _, r := range line {
			if !strings.ContainsRune("█▀▄ ", r) {
				t.Fatalf("unexpected rune %q in plain output", r)
			}
		}
	}
}

func TestRenderTerminalColor(t *testing.T) {
	m, err := qr.Encode("hi", qr.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := renderTerminal(&out, m, true, true, 4); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "\x1b[38;2;") {
		t.Fatal("colored output must start with a foreground color escape")
	}
	if strings.Count(got, "\x1b[0m") != (m.Size+8+1)/2 {
		t.Fatal("each row must end with a reset sequence")
	}
	if strings.Count(got, "▀") != (m.Size+8)*((m.Size+8+1)/2) {
		t.Fatal("each cell must be drawn as one upper half block")
	}
}

func TestRunWritesTerminalAndPNG(t *testing.T) {
	file := t.TempDir() + "/out.png"
	var out bytes.Buffer
	if err := run([]string{"-color=false", "-border", "2", "-scale", "3", "-png", file, "hello world"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("nothing printed to stdout")
	}
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != (21+4)*3 || cfg.Height != (21+4)*3 {
		t.Fatalf("PNG size %dx%d, want %dx%d", cfg.Width, cfg.Height, (21+4)*3, (21+4)*3)
	}
}

func TestReadText(t *testing.T) {
	withArgs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := withArgs.Parse([]string{"hello", "world"}); err != nil {
		t.Fatal(err)
	}
	if got, err := readText(withArgs, nil); err != nil || got != "hello world" {
		t.Fatalf("args: got %q, err %v", got, err)
	}

	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("piped text\nsecond line\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	empty := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := empty.Parse(nil); err != nil {
		t.Fatal(err)
	}
	got, err := readText(empty, file)
	if err != nil {
		t.Fatal(err)
	}
	if got != "piped text\nsecond line" {
		t.Fatalf("pipe: got %q", got)
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if got, err := readText(empty, devNull); err != nil || got != "" {
		t.Fatalf("terminal EOF: got %q, err %v", got, err)
	}
}

func TestRunShowsUsage(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-h"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage: qrterm") || !strings.Contains(out.String(), "go run . \"привіт\"") {
		t.Fatalf("unexpected help output: %q", out.String())
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	cases := [][]string{
		{"-ec", "X", "text"},
		{"-v", "11", "text"},
		{"-v", "abc", "text"},
		{"-border", "-1", "text"},
		{"-scale", "0", "text"},
	}
	for _, args := range cases {
		var out bytes.Buffer
		if err := run(args, &out); err == nil {
			t.Errorf("run(%v) should have failed", args)
		}
	}
}
