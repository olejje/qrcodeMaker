package main

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
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

func TestRunModesAndLargeVersion(t *testing.T) {
	cases := [][]string{
		{"-mode", "numeric", "1234567890"},
		{"-mode", "alnum", "HELLO 123"},
		{"-mode", "byte", "hello world"},
		{"-mode", "kanji", "漢字テスト"},
		{"-eci", "-mode", "byte", "привіт"},
		{"-v", "40", "-ec", "L", "version forty"},
	}
	for _, args := range cases {
		var out bytes.Buffer
		if err := run(append([]string{"-color=false"}, args...), &out); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		if out.Len() == 0 {
			t.Fatalf("run(%v) produced no output", args)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"-mode", "numeric", "abc"}, &out); err == nil {
		t.Fatal("non-numeric text must be rejected in numeric mode")
	}
}

func TestRunWritesSVG(t *testing.T) {
	file := t.TempDir() + "/out.svg"
	var out bytes.Buffer
	if err := run([]string{"-svg", file, "hello"}, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(data)
	if !strings.HasPrefix(svg, `<svg`) || !strings.Contains(svg, `<rect`) || !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		t.Fatalf("unexpected SVG: %q", svg[:min(len(svg), 120)])
	}
}

func TestRunLogoForcesHighCorrection(t *testing.T) {
	logoPath := t.TempDir() + "/logo.png"
	logo := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			logo.Set(x, y, color.RGBA{231, 76, 60, 255})
		}
	}
	lf, err := os.Create(logoPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(lf, logo); err != nil {
		t.Fatal(err)
	}
	lf.Close()

	pngPath := t.TempDir() + "/logo-qr.png"
	var out bytes.Buffer
	if err := run([]string{"-color=false", "-ec", "L", "-logo", logoPath, "-png", pngPath, "https://example.com/logo"}, &out); err != nil {
		t.Fatal(err)
	}
	cfgFile, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.DecodeConfig(cfgFile); err != nil {
		t.Fatal(err)
	}
	cfgFile.Close()

	zbar, err := exec.LookPath("zbarimg")
	if err != nil {
		t.Skip("zbarimg is not installed")
	}
	decoded, err := exec.Command(zbar, "-q", "--raw", pngPath).Output()
	if err != nil {
		t.Fatalf("logo QR did not scan: %v (%s)", err, decoded)
	}
	if !strings.Contains(string(decoded), "https://example.com/logo") {
		t.Fatalf("decoded %q", decoded)
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	cases := [][]string{
		{"-ec", "X", "text"},
		{"-v", "41", "text"},
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
