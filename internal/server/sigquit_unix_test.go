//go:build unix

package server

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// SIGQUIT writes the dump and the process keeps running. (Go's default
// SIGQUIT dumps the stacks and exits; if the handler were not installed,
// this test binary would die here.)
func TestSIGQUITWritesTheDumpAndKeepsRunning(t *testing.T) {
	var out syncBuffer
	stop := installSIGQUITDump(func(io.Writer) {
		routerapi.WriteDump(&out, &routerapi.State{})
	})
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGQUIT))
	// The heading is written before the stacks, so wait for the stacks
	// themselves rather than read between the two writes.
	assert.Eventually(t, func() bool {
		s := out.String()
		return strings.Contains(s, "== Goroutines") && strings.Contains(s, "goroutine ")
	}, 2*time.Second, 10*time.Millisecond, "the dump is written, goroutine stacks included")
}
