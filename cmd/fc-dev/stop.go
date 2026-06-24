package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// newStopCmd stops a running `fc-dev start` instance by sending it a graceful
// SIGTERM, which drains the server and cleanly shuts the embedded Postgres
// (start's deferred pg.Stop). The target is located via the PID file start
// writes; no daemon or socket is involved.
func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running fc-dev instance (graceful; also stops embedded Postgres)",
		Long: `Stop the fc-dev monolith started by ` + "`fc-dev start`" + `.

A running instance records its PID in a file (default alongside the embedded
database). stop reads it, sends SIGTERM so the server drains and the embedded
Postgres shuts down cleanly, then waits for the process to exit — escalating to
SIGKILL only if it overruns --timeout.

Stopping when nothing is running is not an error; a stale PID file is cleaned up.`,
		Args: cobra.NoArgs,
		RunE: runStop,
	}
	cmd.Flags().String("pid-file", envStrDefault("FC_DEV_PID_FILE", pidFilePath()), "PID file written by `fc-dev start`")
	cmd.Flags().Duration("timeout", 20*time.Second, "how long to wait for graceful exit before SIGKILL")
	return cmd
}

func runStop(cmd *cobra.Command, _ []string) error {
	pidFile, _ := cmd.Flags().GetString("pid-file")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	out := cmd.OutOrStdout()

	pid, err := readPIDFile(pidFile)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "No running fc-dev instance found (no pid file at %s).\n", pidFile)
		return nil
	}
	if err != nil {
		return err
	}

	if !processAlive(pid) {
		_ = os.Remove(pidFile)
		fmt.Fprintf(out, "No running fc-dev instance (pid %d not alive); removed stale pid file.\n", pid)
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	fmt.Fprintf(out, "Stopping fc-dev (pid %d)…\n", pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}

	if waitForExit(pid, timeout) {
		removePIDFileIfOwned(pidFile, pid)
		fmt.Fprintf(out, "Stopped fc-dev (pid %d).\n", pid)
		return nil
	}

	// Graceful window elapsed — force-kill. This skips start's clean shutdown,
	// so the embedded Postgres may need a moment to release its lock on next boot.
	fmt.Fprintf(out, "fc-dev did not exit within %s; sending SIGKILL.\n", timeout)
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("force-kill pid %d: %w", pid, err)
	}
	if !waitForExit(pid, 5*time.Second) {
		return fmt.Errorf("pid %d still running after SIGKILL", pid)
	}
	removePIDFileIfOwned(pidFile, pid)
	fmt.Fprintf(out, "Force-stopped fc-dev (pid %d).\n", pid)
	return nil
}

// waitForExit polls until pid is gone or timeout elapses, returning true if the
// process exited in time.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !processAlive(pid)
}
