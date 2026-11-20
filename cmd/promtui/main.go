package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/app"
	"promtui/internal/config"
	"promtui/internal/history"
)

func run() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "promtui")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := flag.String("config", "", "TOML configuration (default: user config directory/promtui/config.toml)")
	name := flag.String("connection", "", "configured connection name")
	hist := flag.String("history", filepath.Join(home, ".local", "share", "promtui", "history.jsonl"), "query history file")
	window := flag.Duration("range", 0, "start in range mode, e.g. 15m or 168h")
	start := flag.String("start", "", "range start (RFC3339; requires -end)")
	end := flag.String("end", "", "fixed evaluation time or range end (RFC3339)")
	step := flag.Duration("step", 0, "range step (default: automatic)")
	graphics := flag.String("graphics", "auto", "range graphs: auto (Rex/Kitty), kitty, or off")
	flag.Parse()
	optional := *path == ""
	if optional {
		*path = filepath.Join(dir, "config.toml")
	}
	cfg, err := config.Load(*path, optional)
	if err != nil {
		return err
	}
	if *name == "" {
		*name = cfg.General.Default
	}
	o := app.Options{Timeout: cfg.Query.Timeout, Window: cfg.Query.Range, Step: *step, Limit: cfg.Query.MaxSeries, HistoryPath: *hist, HistoryLimit: cfg.General.History}
	switch *graphics {
	case "auto":
		o.Graphics = os.Getenv("TERM_PROGRAM") == "rex" || os.Getenv("TERM") == "xterm-kitty"
		o.ProbeGraphics = !o.Graphics
	case "kitty":
		o.Graphics = true
	case "off":
	default:
		return fmt.Errorf("unknown -graphics %q", *graphics)
	}
	for _, c := range cfg.Connections {
		if c.Name == *name {
			o.Connection = c
			break
		}
	}
	if o.Connection.Name == "" {
		return fmt.Errorf("unknown connection %q", *name)
	}
	if *step < 0 || *window < 0 {
		return fmt.Errorf("-step and -range must not be negative")
	}
	if *end != "" {
		o.End, err = time.Parse(time.RFC3339Nano, *end)
		if err != nil {
			return err
		}
	}
	if *window > 0 {
		o.Window, o.Ranged = *window, true
	}
	if *start != "" {
		t, err := time.Parse(time.RFC3339Nano, *start)
		if err != nil {
			return err
		}
		if !o.End.After(t) || *window != 0 {
			return fmt.Errorf("-start requires a later -end and no -range")
		}
		o.Window, o.Ranged = o.End.Sub(t), true
	}
	entries, err := history.Load(*hist, o.HistoryLimit)
	if err != nil {
		return fmt.Errorf("load %s: %w", *hist, err)
	}
	m := app.New(o, entries)
	defer func() { fmt.Fprint(os.Stdout, m.CloseGraphics()) }()
	_, err = tea.NewProgram(m).Run()
	if err == nil && m.HistoryError() != "" {
		return fmt.Errorf("%s", m.HistoryError())
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "promtui:", err)
		os.Exit(1)
	}
}
