//go:build integration

package postgres_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
)

// The outbox processor's cut of a batch by bytes, its split of a request the
// platform refuses as too large and its terminal refusal of a row too large
// on its own (docs/payload-offload-design.md, section 8), against the real
// Postgres outbox table: what the in-memory table of internal/outbox tests,
// here with the repository's own claims, marks and group rules.

// insertSized writes a PUSH row whose payload is about size bytes.
func insertSized(t *testing.T, pool *pgxpool.Pool, id, group string, created time.Time, size int) {
	t.Helper()
	head := `{"topic":"shop:a","type":"x","data":{"pad":"`
	tail := fmt.Sprintf(`"},"pushId":%q}`, id)
	payload := head + strings.Repeat("x", max(size-len(head)-len(tail), 0)) + tail
	var g *string
	if group != "" {
		g = &group
	}
	_, err := pool.Exec(t.Context(), `
INSERT INTO outbox_messages (id, type, message_group, payload, status, retry_count, created_at, updated_at)
VALUES ($1, 'PUSH', $2, $3, 0, 0, $4, $4)`, id, g, payload, created)
	require.NoError(t, err)
}

// sizedPlatform is a batch endpoint that answers 413 to a body over limit and
// SUCCESS to every item of any other, recording each request's ids and size.
type sizedPlatform struct {
	limit    int
	mu       sync.Mutex
	requests [][]string
	sizes    []int
	srv      *httptest.Server
}

func newSizedPlatform(t *testing.T, limit int) *sizedPlatform {
	p := &sizedPlatform{limit: limit}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.Unmarshal(raw, &body)
		ids := make([]string, len(body.Items))
		for i, it := range body.Items {
			ids[i] = it.PushID
		}
		p.mu.Lock()
		p.requests = append(p.requests, ids)
		p.sizes = append(p.sizes, len(raw))
		p.mu.Unlock()
		if len(raw) > p.limit {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		out := make([]map[string]string, len(ids))
		for i := range out {
			out[i] = map[string]string{"status": "SUCCESS"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// delivered is every id the platform answered SUCCESS for, in order.
func (p *sizedPlatform) delivered() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for i, req := range p.requests {
		if p.sizes[i] <= p.limit {
			out = append(out, req...)
		}
	}
	return out
}

func (p *sizedPlatform) sent() ([][]string, []int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.requests), slices.Clone(p.sizes)
}

func TestCutByBytesOnThePostgresTable(t *testing.T) {
	pool := table(t)
	at := time.Now().UTC().Add(-time.Minute)
	var ungrouped, grouped []string
	for i := range 12 {
		id := fmt.Sprintf("u-%02d", i+1)
		insertSized(t, pool, id, "", at.Add(time.Duration(i)*time.Millisecond), 900)
		ungrouped = append(ungrouped, id)
	}
	for i := range 4 {
		id := fmt.Sprintf("g-%02d", i+1)
		insertSized(t, pool, id, "grp-a", at.Add(time.Duration(i)*time.Millisecond), 900)
		grouped = append(grouped, id)
	}
	const big = "u-big"
	insertSized(t, pool, big, "", at.Add(20*time.Millisecond), 6000)

	// The processor sends up to 4000 bytes a request; the platform takes 2500.
	const maxRequest, platformLimit = 4000, 2500
	plat := newSizedPlatform(t, platformLimit)
	// run reads only the stand-in's URL.
	p, stop := run(t, pool, &platform{srv: plat.srv}, func(c *outbox.Config) {
		c.BatchSize = 20
		c.MaxRequestBytes = maxRequest
	})

	eventually(t, 15*time.Second, "every row but the oversized one delivered", func() bool { return count(t, pool) == 1 })
	time.Sleep(150 * time.Millisecond) // more polls: the refused row is not claimed again
	stop()

	got := plat.delivered()
	assert.ElementsMatch(t, append(slices.Clone(ungrouped), grouped...), got, "each row delivered exactly once")
	var groupOrder []string
	for _, id := range got {
		if strings.HasPrefix(id, "g-") {
			groupOrder = append(groupOrder, id)
		}
	}
	assert.Equal(t, grouped, groupOrder, "the group in written order")

	requests, sizes := plat.sent()
	split, bigSent := false, 0
	for i, req := range requests {
		if len(req) > 1 {
			assert.LessOrEqual(t, sizes[i], maxRequest, "a request of %d rows within the processor's cap", len(req))
			for _, id := range req {
				assert.False(t, strings.HasPrefix(id, "g-"), "a grouped row is sent alone")
			}
		}
		if sizes[i] > platformLimit && len(req) > 1 {
			split = true
		}
		if slices.Equal(req, []string{big}) {
			bigSent++
		}
	}
	assert.True(t, split, "a request over the platform's cap was refused and split")
	assert.Equal(t, 1, bigSent, "the oversized row is sent alone, once")

	r, ok := row(t, pool, big)
	require.True(t, ok)
	assert.Equal(t, common.OutboxBadRequest.Code(), r.status)
	assert.Equal(t, 1, r.retries)
	assert.True(t, strings.HasPrefix(r.msg, outbox.BlockingMarker+"PAYLOAD_TOO_LARGE: "), r.msg)
	_, failed := p.Totals()
	assert.EqualValues(t, 1, failed)
}
