//go:build !windows

package main

import (
	"os"
	"syscall"
)

// fileOwnedByCurrentUser reports whether the file belongs to the user running
// fcdev.
func fileOwnedByCurrentUser(st os.FileInfo) bool {
	sys, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(sys.Uid) == os.Getuid()
}
