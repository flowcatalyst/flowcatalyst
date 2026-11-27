//go:build !(darwin || linux)

package main

import (
	"fmt"
	"io"
	"runtime"
)

// lockAddonInstall: add-ons are built for macOS and Linux only, and the
// command refuses other platforms before it locks.
func lockAddonInstall(string, io.Writer) (func(), error) {
	return nil, fmt.Errorf("%w (%s/%s)", errAddonNoBuild, runtime.GOOS, runtime.GOARCH)
}
