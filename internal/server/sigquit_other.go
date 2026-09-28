//go:build !unix

package server

import "io"

// installSIGQUITDump is a no-op where there is no SIGQUIT.
func installSIGQUITDump(func(io.Writer)) (stop func()) { return func() {} }
