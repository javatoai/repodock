//go:build windows

package storage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func openPreferences(path string) (*os.File, error) {
	for attempt := 0; ; attempt++ {
		file, err := os.Open(path)
		if err == nil || attempt == 49 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return file, err
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func replaceFile(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// Windows 的短暂共享冲突不能通过删除旧文件来规避，否则读者会看到空档。
	for attempt := 0; ; attempt++ {
		err = windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil || attempt == 49 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		time.Sleep(2 * time.Millisecond)
	}
}
