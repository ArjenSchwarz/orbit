//go:build windows

package transcript

import (
	"os"
	"time"
)

// getFileInfo returns mtime, inode, and size for change detection.
// Windows does not expose Unix inode semantics, so inode is always 0.
func getFileInfo(path string) (time.Time, uint64, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0, 0, err
	}
	return info.ModTime(), 0, info.Size(), nil
}
