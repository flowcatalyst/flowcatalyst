package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// ConfigSource fetches the live RouterConfig from a remote endpoint.
type ConfigSource struct {
	URL    string
	Client *http.Client

	mu   sync.Mutex
	last []byte // last seen body for change detection
}

// NewConfigSource builds a source pointing at url.
func NewConfigSource(url string) *ConfigSource {
	return &ConfigSource{
		URL:    url,
		Client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fetch returns the current config. Returns (nil, ErrUnchanged) when
// the body matches the previous fetch — callers can skip reconfigure
// in that case.
func (cs *ConfigSource) Fetch(ctx context.Context) (*common.RouterConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cs.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cs.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("config fetch: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	cs.mu.Lock()
	unchanged := len(cs.last) > 0 && bytesEqual(cs.last, body)
	cs.last = body
	cs.mu.Unlock()
	if unchanged {
		return nil, ErrUnchanged
	}

	var cfg common.RouterConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("config decode: %w", err)
	}
	return &cfg, nil
}

// ErrUnchanged is returned by Fetch when the body matches the previous fetch.
var ErrUnchanged = errors.New("config unchanged")

// Watch polls cs every interval and applies the result to manager.
// Blocks until ctx is cancelled.
func Watch(ctx context.Context, cs *ConfigSource, manager *Manager, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	apply := func() {
		cfg, err := cs.Fetch(ctx)
		if errors.Is(err, ErrUnchanged) {
			return
		}
		if err != nil {
			slog.Warn("config fetch failed", "err", err)
			return
		}
		if err := manager.Reconfigure(ctx, *cfg); err != nil {
			slog.Warn("manager reconfigure failed", "err", err)
		}
	}

	apply()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			apply()
		}
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
