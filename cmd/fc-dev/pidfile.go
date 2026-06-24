package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// pidFilePath is the deterministic, flag-independent location of the running
// `fc-dev start` instance's PID file, so `fc-dev stop` can find it without
// being told the same flags. Lives alongside the embedded-pg data dir under the
// per-user data directory. Override with --pid-file (or FC_DEV_PID_FILE).
func pidFilePath() string {
	return filepath.Join(userDataDir(), "flowcatalyst", "fc-dev.pid")
}

// writePIDFile records the current process's PID at path, creating parent
// directories as needed. Overwrites any existing file (a stale one from a
// crashed instance shouldn't block a fresh start).
func writePIDFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// readPIDFile returns the PID recorded at path. A missing file returns
// os.ErrNotExist so callers can distinguish "nothing running" from a real error.
func readPIDFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("malformed pid file %s: %w", path, err)
	}
	return pid, nil
}

// removePIDFileIfOwned deletes path only when it still records pid, so an
// instance's deferred cleanup never removes a newer instance's pid file.
func removePIDFileIfOwned(path string, pid int) {
	if cur, err := readPIDFile(path); err == nil && cur == pid {
		_ = os.Remove(path)
	}
}

// processAlive reports whether a process with pid currently exists. On Unix,
// signal 0 performs existence/permission checking without delivering a signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but is owned by another user.
	return errors.Is(err, syscall.EPERM)
}
