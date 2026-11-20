package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Connection string    `json:"connection"`
	Query      string    `json:"query"`
	At         time.Time `json:"at"`
}

// Add returns a new newest-first snapshot; older snapshots remain safe to save.
func Add(entries []Entry, e Entry, limit int) []Entry {
	if limit == 0 {
		return nil
	}
	out := []Entry{e}
	for _, old := range entries {
		if len(out) == limit {
			break
		}
		if e.Query != old.Query {
			out = append(out, old)
		}
	}
	return out
}

func Load(path string, limit int) ([]Entry, error) {
	if limit == 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	var out []Entry
	seen := make(map[string]bool)
	for line := 1; len(out) < limit && s.Scan(); line++ {
		var e Entry
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("history line %d: %w", line, err)
		}
		if !seen[e.Query] {
			seen[e.Query] = true
			out = append(out, e)
		}
	}
	return out, s.Err()
}

// Save atomically replaces the file, so interrupted writes preserve the prior snapshot.
func Save(path string, entries []Entry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err = enc.Encode(e); err != nil {
			f.Close()
			return err
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
