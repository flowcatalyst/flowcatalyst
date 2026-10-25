//go:build !windows && !linux

package main

import "syscall"

// childSysProcAttr: there is no parent-death signal off Linux, so a JVM
// outlives an fcdev that is killed hard. A normal stop (and SIGINT/SIGTERM)
// stops it; see docs/fcdev-ingestor.md for the leftover case.
func childSysProcAttr() *syscall.SysProcAttr { return nil }
