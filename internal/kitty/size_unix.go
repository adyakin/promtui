//go:build darwin || linux

package kitty

import (
	"golang.org/x/sys/unix"
	"os"
)

func CellSize() (int, int) {
	s, e := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if e == nil && s.Col > 0 && s.Row > 0 && s.Xpixel >= s.Col && s.Ypixel >= s.Row {
		return int(s.Xpixel / s.Col), int(s.Ypixel / s.Row)
	}
	return 8, 16
}
