//go:build windows

package main

import "os"

// fileOwnedByCurrentUser is never consulted on Windows (untrustedJarReason
// returns early there: POSIX owners and mode bits do not apply).
func fileOwnedByCurrentUser(os.FileInfo) bool { return true }
