package saveditems

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileVersionDistinguishesMissingFromEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	missing, err := Read(path)
	if err != nil || missing != nil {
		t.Fatalf("missing = %v, %v", missing, err)
	}
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, missing, []byte("alerts: []\n")); !errors.Is(err, ErrChanged) {
		t.Fatalf("empty replacement after missing: %v", err)
	}
	version, err := Read(path)
	if err != nil || version == nil || len(version) != 0 {
		t.Fatalf("empty version = %#v, %v", version, err)
	}
	if err := Write(path, version, []byte("alerts: []\n")); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(path); err != nil || string(got) != "alerts: []\n" {
		t.Fatalf("new version = %q, %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}
