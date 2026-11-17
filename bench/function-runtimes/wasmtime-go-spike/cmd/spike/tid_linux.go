//go:build wasmtime && linux

package main

import "syscall"

func threadID() int { return syscall.Gettid() }
