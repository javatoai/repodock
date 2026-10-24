//go:build darwin

package platform

import "golang.org/x/sys/unix"

func renameNoReplace(old, new string) error {
	return unix.RenamexNp(old, new, unix.RENAME_EXCL)
}
