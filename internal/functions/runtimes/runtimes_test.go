package runtimes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
)

func testLoader(compile func(ctx context.Context) (*engine.Module, error)) *Loader {
	return &Loader{jsLock: make(chan struct{}, 1), compileJS: compile}
}

// 2.7: a failed or cancelled first compile of the shared JS engine is not
// cached; the next caller compiles again.
func TestSharedJSEngineCompileRetriesAfterFailure(t *testing.T) {
	mod := &engine.Module{}
	calls := 0
	l := testLoader(func(context.Context) (*engine.Module, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient compile failure")
		}
		return mod, nil
	})
	script := []byte("export default () => {}")
	if _, err := l.Prepare(t.Context(), JS, script); err == nil {
		t.Fatal("the first compile should have failed")
	}
	p, err := l.Prepare(t.Context(), JS, script)
	if err != nil {
		t.Fatalf("the second compile did not recover: %v", err)
	}
	if p.Module != mod || !p.Shared {
		t.Fatalf("prepared = %+v", p)
	}
	if _, err := l.Prepare(t.Context(), JS, script); err != nil || calls != 2 {
		t.Fatalf("a successful compile must be cached: err=%v calls=%d", err, calls)
	}
}

// 2.7: cancellation is not cached, and a waiter gives up with its context
// instead of waiting for someone else's compile.
func TestSharedJSEngineCancellationIsNotCached(t *testing.T) {
	mod := &engine.Module{}
	started := make(chan struct{})
	release := make(chan struct{})
	first := true
	l := testLoader(func(ctx context.Context) (*engine.Module, error) {
		if first {
			first = false
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return mod, nil
	})
	script := []byte("x")
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() { _, err := l.Prepare(ctx, JS, script); errc <- err }()
	<-started

	waiter, wcancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer wcancel()
	if _, err := l.Prepare(waiter, JS, script); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a waiter behind a slow compile: %v, want its own deadline", err)
	}
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled compile: %v", err)
	}
	if p, err := l.Prepare(t.Context(), JS, script); err != nil || p.Module != mod {
		t.Fatalf("after a cancelled compile: %v", err)
	}
}
