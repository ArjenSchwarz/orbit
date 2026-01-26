//go:build !windows

package transcript

import (
	"os"
	"syscall"
	"time"
)

// getFileInfo returns mtime, inode, and size for change detection.
func getFileInfo(path string) (time.Time, uint64, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0, 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime(), 0, info.Size(), nil
	}

	return info.ModTime(), uint64(stat.Ino), info.Size(), nil
}
