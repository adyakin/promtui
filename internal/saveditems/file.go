// Package saveditems contains persistence helpers shared by the saved collections.
package saveditems

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrChanged = errors.New("file changed on disk; close and reopen the saved items browser")

// CheckName trims a proposed name and checks it against the collection.
func CheckName(name string, count, except int, nameAt func(int) string, kind string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("enter a name")
	}
	for i := range count {
		if i != except && strings.EqualFold(nameAt(i), name) {
			return "", fmt.Errorf("a %s named %q already exists", kind, nameAt(i))
		}
	}
	return name, nil
}

// Read returns a nil version only when the file does not exist.
func Read(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, &os.PathError{Op: "read", Path: path, Err: errors.New("expected a regular file")}
	}
	data, err := os.ReadFile(path)
	if err == nil && data == nil {
		data = []byte{}
	}
	return data, err
}

func CheckVersion(path string, version []byte) error {
	current, err := Read(path)
	if err != nil {
		return err
	}
	if (current == nil) != (version == nil) || !bytes.Equal(current, version) {
		return ErrChanged
	}
	return nil
}

// Write atomically replaces a collection if its on-disk version still matches.
func Write(path string, version, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := CheckVersion(path, version); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
