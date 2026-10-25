//go:build linux

package main

import "syscall"

// childSysProcAttr makes the kernel kill the JVM if fcdev dies without
// stopping it (a crash, SIGKILL): no orphan keeps the port.
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
