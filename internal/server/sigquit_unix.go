//go:build unix

package server

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// sigquitRepeatWindow is how soon a second SIGQUIT must follow the first to
// get Go's default behaviour back (dump and exit).
const sigquitRepeatWindow = 5 * time.Second

// installSIGQUITDump makes SIGQUIT (kill -QUIT, Ctrl-\) write the router dump
// — what every worker is on, the held groups, every goroutine's stack — to
// stderr and keep running, the way a JVM thread dump does. Go's default
// SIGQUIT dumps the stacks and exits, which made the one tool an operator has
// on a wedged process also kill it. A second SIGQUIT within
// sigquitRepeatWindow restores the default and re-raises, so the old "dump
// and exit" is still one more keypress away.
//
// dump writes the report; stop uninstalls the handler.
func installSIGQUITDump(dump func(io.Writer)) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGQUIT)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		var last time.Time
		for {
			select {
			case <-done: // stop
				return
			case <-ch: // SIGQUIT
				now := time.Now()
				if !last.IsZero() && now.Sub(last) < sigquitRepeatWindow {
					slog.Warn("second SIGQUIT: restoring the default (dump every goroutine and exit)")
					signal.Reset(syscall.SIGQUIT)
					_ = syscall.Kill(os.Getpid(), syscall.SIGQUIT)
					return
				}
				last = now
				slog.Warn("SIGQUIT: writing the router dump to stderr; send another within 5s to dump and exit")
				func() {
					// A dump must never take the process down.
					defer func() {
						if r := recover(); r != nil {
							fmt.Fprintf(os.Stderr, "SIGQUIT dump failed: %v\n", r)
						}
					}()
					dump(os.Stderr)
				}()
			}
		}
	}()
	return func() {
		once.Do(func() {
			signal.Stop(ch)
			close(done)
		})
	}
}
