package main

import (
	"errors"
	"os"
	"path/filepath"
)

func savedQueriesPath(configDir, dataDir string) string {
	path := filepath.Join(configDir, "queries.yaml")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		legacy := filepath.Join(dataDir, "queries.yaml")
		// Keep using an existing collection in place. Let its store report
		// permission or format errors instead of opening a new empty file.
		if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
			return legacy
		}
	}
	return path
}
