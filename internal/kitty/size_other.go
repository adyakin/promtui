//go:build !darwin && !linux

package kitty

func CellSize() (int, int) { return 8, 16 }
