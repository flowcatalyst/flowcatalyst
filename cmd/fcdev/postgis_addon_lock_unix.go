//go:build darwin || linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// lockAddonInstall takes the cross-process lock that serialises
// `fcdev install postgis` runs (flock on a file beside the embedded tree; the
// kernel releases it if the process dies). It waits for a holder, saying so.
func lockAddonInstall(path string, out io.Writer) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, err
		}
		fmt.Fprintln(out, "Another fcdev install postgis is running; waiting for it to finish.")
		if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
