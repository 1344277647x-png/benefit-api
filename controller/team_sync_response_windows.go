//go:build windows

package controller

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Create the response with delete-on-close from the first handle. Creating a
// regular file and reopening it would leave a crash window with an orphaned
// response containing private upstream data.
func createTeamSyncStagingFile(dir string) (*os.File, error) {
	for i := 0; i < 10; i++ {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, "team-relay-"+hex.EncodeToString(nonce[:])+".response")
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}
		handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE,
			windows.FILE_SHARE_DELETE, nil, windows.CREATE_NEW,
			windows.FILE_ATTRIBUTE_TEMPORARY|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(handle), path), nil
	}
	return nil, errors.New("unable to allocate unique team response staging file")
}

func teamStagingFreeBytes(path string) (int64, error) {
	dir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(dir, &free, nil, nil); err != nil {
		return 0, err
	}
	if free > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1), nil
	}
	return int64(free), nil
}
