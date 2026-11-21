package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"qrterm/qr"
)

type menuState struct {
	ec, version, mode  string
	eci, color, invert bool
	border, scale      int
	output, png, svg   string
	logo               string
}

func newMenuState() menuState {
	return menuState{
		ec: "M", version: "auto", mode: "auto",
		color: true, border: 4, scale: 8,
		output: "terminal", png: "qr.png", svg: "qr.svg",
	}
}

type menuField struct {
	label string
	get   func() string
	set   func(string) error
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func boolField(label string, get func() bool, set func(bool)) menuField {
	return menuField{
		label: label,
		get:   func() string { return onOff(get()) },
		set: func(v string) error {
			switch strings.ToLower(v) {
			case "on", "y", "yes", "true", "1":
				set(true)
			case "off", "n", "no", "false", "0":
				set(false)
			default:
				return fmt.Errorf("want on or off")
			}
			return nil
		},
	}
}

func intField(label string, get func() int, set func(int), min int) menuField {
	return menuField{
		label: label,
		get:   func() string { return strconv.Itoa(get()) },
		set: func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n < min {
				return fmt.Errorf("want an integer >= %d", min)
			}
			set(n)
			return nil
		},
	}
}

func (s *menuState) fields() []menuField {
	return []menuField{
		{"EC", func() string { return s.ec }, func(v string) error {
			ec, err := qr.ParseECLevel(v)
			if err != nil {
				return err
			}
			s.ec = ec.String()
			return nil
		}},
		{"Mode", func() string { return s.mode }, func(v string) error {
			mode, err := qr.ParseMode(v)
			if err != nil {
				return err
			}
			s.mode = mode.String()
			return nil
		}},
		{"Version", func() string { return s.version }, func(v string) error {
			if strings.EqualFold(v, "auto") {
				s.version = "auto"
				return nil
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 40 {
				return fmt.Errorf("want auto or 1-40")
			}
			s.version = strconv.Itoa(n)
			return nil
		}},
		boolField("ECI", func() bool { return s.eci }, func(v bool) { s.eci = v }),
		boolField("Color", func() bool { return s.color }, func(v bool) { s.color = v }),
		boolField("Invert", func() bool { return s.invert }, func(v bool) { s.invert = v }),
		intField("Border", func() int { return s.border }, func(v int) { s.border = v }, 0),
		intField("Scale", func() int { return s.scale }, func(v int) { s.scale = v }, 1),
		{"Output", func() string { return s.output }, func(v string) error {
			switch strings.ToLower(v) {
			case "terminal", "term", "t":
				s.output = "terminal"
			case "png", "p":
				s.output = "png"
			case "svg", "s":
				s.output = "svg"
			case "both", "b":
				s.output = "both"
			default:
				return fmt.Errorf("want terminal, png, svg or both")
			}
			return nil
		}},
		{"PNG file", func() string { return s.png }, func(v string) error {
			if v == "" {
				return fmt.Errorf("path cannot be empty")
			}
			s.png = v
			return nil
		}},
		{"SVG file", func() string { return s.svg }, func(v string) error {
			if v == "" {
				return fmt.Errorf("path cannot be empty")
			}
			s.svg = v
			return nil
		}},
		{"Logo file", func() string { return s.logo }, func(v string) error {
			s.logo = v
			return nil
		}},
	}
}

func (s *menuState) outputConfig(view bool) (outputConfig, error) {
	c := outputConfig{
		color: s.color, invert: s.invert, view: view,
		border: s.border, scale: s.scale,
	}
	switch s.output {
	case "png":
		c.png = s.png
	case "svg":
		c.svg = s.svg
	case "both":
		c.png, c.svg = s.png, s.svg
	}
	if s.logo != "" {
		logo, err := loadLogo(s.logo)
		if err != nil {
			return c, err
		}
		c.logo = logo
	}
	return c, nil
}

func menuLine(r *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := r.ReadString('\n')
	value := strings.TrimSpace(line)
	if err != nil {
		if errors.Is(err, io.EOF) && value != "" {
			return value, nil
		}
		return value, err
	}
	return value, nil
}

func menuAsk(r *bufio.Reader, out io.Writer, label, current string) (string, error) {
	fmt.Fprintf(out, "%s [%s]: ", label, current)
	line, err := r.ReadString('\n')
	value := strings.TrimSpace(line)
	if err != nil {
		if errors.Is(err, io.EOF) && value != "" {
			return value, nil
		}
		return value, err
	}
	if value == "" {
		return current, nil
	}
	return value, nil
}

func runMenu(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	s := newMenuState()
	for {
		fmt.Fprintln(out, "\nqrterm menu")
		fmt.Fprintf(out, "  %s | %s | v%s | %s | color:%s | invert:%s\n",
			s.ec, s.mode, s.version, s.output, onOff(s.color), onOff(s.invert))
		fmt.Fprintln(out, "  1) Generate")
		fmt.Fprintln(out, "  2) Settings")
		fmt.Fprintln(out, "  3) Generate + viewer")
		fmt.Fprintln(out, "  0) Exit")

		choice, err := menuLine(r, out, "Choice: ")
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch strings.ToLower(choice) {
		case "1":
			err = menuGenerate(r, out, &s, false)
		case "2":
			err = menuSettings(r, out, &s)
		case "3":
			err = menuGenerate(r, out, &s, true)
		case "0", "q", "quit", "exit", "":
			return nil
		default:
			fmt.Fprintln(out, "Unknown choice.")
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			fmt.Fprintln(out, "Error:", err)
		}
	}
}

func menuSettings(r *bufio.Reader, out io.Writer, s *menuState) error {
	fields := s.fields()
	for {
		fmt.Fprintln(out, "\nSettings:")
		for i, f := range fields {
			fmt.Fprintf(out, "  %2d) %-10s %s\n", i+1, f.label, f.get())
		}
		fmt.Fprintln(out, "   0) Back")

		choice, err := menuLine(r, out, "Choice: ")
		if err != nil {
			return err
		}
		if choice == "0" || choice == "" {
			return nil
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(fields) {
			fmt.Fprintln(out, "Unknown choice.")
			continue
		}
		f := fields[n-1]
		value, err := menuAsk(r, out, f.label, f.get())
		if err != nil {
			return err
		}
		if err := f.set(value); err != nil {
			fmt.Fprintln(out, "Error:", err)
		}
	}
}

func menuGenerate(r *bufio.Reader, out io.Writer, s *menuState, view bool) error {
	text, err := menuLine(r, out, "Text: ")
	if err != nil {
		return err
	}
	if text == "" {
		fmt.Fprintln(out, "Nothing to encode.")
		return nil
	}
	ec, err := qr.ParseECLevel(s.ec)
	if err != nil {
		return err
	}
	mode, err := qr.ParseMode(s.mode)
	if err != nil {
		return err
	}
	version := 0
	if s.version != "auto" {
		version, err = strconv.Atoi(s.version)
		if err != nil {
			return err
		}
	}
	cfg, err := s.outputConfig(view)
	if err != nil {
		return err
	}
	if cfg.logo != nil && ec != qr.ECLevelH {
		fmt.Fprintln(out, "Logo forces level H.")
		ec = qr.ECLevelH
	}
	m, err := qr.Encode(text, qr.Options{EC: ec, Version: version, Mode: mode, ECI: s.eci})
	if err != nil {
		return err
	}
	if err := emit(out, m, cfg); err != nil {
		return err
	}
	if cfg.png != "" {
		fmt.Fprintf(out, "Saved %s\n", cfg.png)
	}
	if cfg.svg != "" {
		fmt.Fprintf(out, "Saved %s\n", cfg.svg)
	}
	return nil
}
