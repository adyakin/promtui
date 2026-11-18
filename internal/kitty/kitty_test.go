package kitty

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestChunkedPlacement(t *testing.T) {
	png := bytes.Repeat([]byte("test"), 3000)
	s := Frame(png, BaseID, 4, 80, 16)
	if !strings.HasPrefix(s, "\x1b7\x1b[4;1H") || !strings.HasSuffix(s, Delete(BaseID+1)+"\x1b8") {
		t.Fatal("cursor or cleanup")
	}
	chunks := strings.Split(s, "\x1b_G")[1:]
	var payload string
	for i, c := range chunks[:len(chunks)-1] {
		h, data, ok := strings.Cut(c, ";")
		if !ok {
			t.Fatal(h)
		}
		data = strings.TrimSuffix(data, "\x1b\\")
		if len(data) > 4096 || strings.Contains(h, "m=0") != (i == len(chunks)-2) {
			t.Fatal(h, len(data))
		}
		if i == 0 && !strings.Contains(h, "c=80,r=16,C=1") {
			t.Fatal(h)
		}
		payload += data
	}
	got, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || !bytes.Equal(png, got) {
		t.Fatal("corrupted payload", err)
	}
}
