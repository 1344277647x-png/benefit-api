//go:build !windows

package controller

import (
	"math"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func createTeamSyncStagingFile(dir string) (*os.File, error) {
	return os.CreateTemp(dir, "team-relay-*.response")
}

func teamStagingFreeBytes(path string) (int64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(filepath.Dir(path), &stats); err != nil {
		return 0, err
	}
	if stats.Bsize <= 0 {
		return 0, unix.EIO
	}
	if stats.Bavail > uint64(math.MaxInt64)/uint64(stats.Bsize) {
		return math.MaxInt64, nil
	}
	return int64(stats.Bavail) * int64(stats.Bsize), nil
}
