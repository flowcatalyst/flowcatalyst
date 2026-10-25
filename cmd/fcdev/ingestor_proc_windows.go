//go:build windows

package main

import (
	"os"
	"os/exec"
)

// terminateProcess: a Windows process has no SIGTERM, and a JVM does not shut
// down on the console events Go can send, so it is ended outright.
func terminateProcess(p *os.Process) error { return p.Kill() }

func prepareChild(*exec.Cmd) {}
