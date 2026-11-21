package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"promtui/internal/app"
	"promtui/internal/config"
	"promtui/internal/history"
)

func defaultDirs(home string) (configDir, dataDir string) {
	return filepath.Join(home, ".config", "promtui"),
		filepath.Join(home, ".local", "share", "promtui")
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 1 && args[0] == "version" {
		return printVersion(stdout)
	}
	home, homeErr := os.UserHomeDir()
	dir, dataDir := defaultDirs(home)
	flags := flag.NewFlagSet("promtui", flag.ExitOnError)
	path := flags.String("config", "", "configuration file (.yaml, .yml, or .toml; default: first match in ~/.config/promtui/)")
	name := flags.String("connection", "", "configured connection name")
	hist := flags.String("history", filepath.Join(dataDir, "history.jsonl"), "query history file")
	window := flags.Duration("range", 0, "start in range mode, e.g. 15m or 168h")
	start := flags.String("start", "", "range start (RFC3339; requires -end)")
	end := flags.String("end", "", "fixed evaluation time or range end (RFC3339)")
	step := flags.Duration("step", 0, "range step (default: automatic)")
	graphics := flags.String("graphics", "auto", "range graphs: auto (Rex/Kitty), kitty, or off")
	showVersion := flags.Bool("version", false, "print the PromTUI version and exit")
	flags.Parse(args)
	if *showVersion {
		return printVersion(stdout)
	}
	if homeErr != nil {
		return homeErr
	}
	var (
		cfg config.Config
		err error
	)
	if *path == "" {
		cfg, err = config.LoadDefault(dir)
	} else {
		cfg, err = config.Load(*path, false)
	}
	if err != nil {
		return err
	}
	if *name == "" {
		*name = cfg.General.Default
	}
	o := app.Options{Connections: cfg.Connections, Timeout: cfg.Query.Timeout, Window: cfg.Query.Range, Step: *step, Limit: cfg.Query.MaxSeries, HistoryPath: *hist, HistoryLimit: cfg.General.History}
	o.SavedQueriesPath = savedQueriesPath(dir, dataDir)
	o.SavedAlertsPath = filepath.Join(dir, "alerts.yaml")
	o.SilenceCreator = cfg.SilenceCreator
	o.ProgramStatus = cfg.Terminal.ProgramStatus && term.IsTerminal(os.Stdout.Fd())
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
	defer func() { fmt.Fprint(os.Stdout, m.CloseTerminal()) }()
	_, err = tea.NewProgram(m).Run()
	if err == nil && m.HistoryError() != "" {
		return fmt.Errorf("%s", m.HistoryError())
	}
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "promtui:", err)
		os.Exit(1)
	}
}

// version is set by GoReleaser for releases and snapshots.
var version string

func printVersion(w io.Writer) error {
	info, _ := debug.ReadBuildInfo()
	_, err := fmt.Fprintln(w, "promtui", buildVersion(version, info))
	return err
}

func buildVersion(version string, info *debug.BuildInfo) string {
	if version != "" {
		return version
	}
	if info == nil {
		return "devel"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	v := "devel"
	if revision != "" {
		v += "+" + revision[:min(len(revision), 12)]
	}
	if dirty {
		v += "-dirty"
	}
	return v
}
