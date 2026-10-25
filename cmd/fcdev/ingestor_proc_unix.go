//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// terminateProcess asks the JVM to shut down (SIGTERM runs its shutdown hooks).
func terminateProcess(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

func prepareChild(cmd *exec.Cmd) { cmd.SysProcAttr = childSysProcAttr() }
