package outbox

import (
	"context"
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// The cut of a batch into requests by count and bytes, the split of a request
// the platform refuses as too large (413), and how both leave message groups
// alone (docs/payload-offload-design.md, section 8).

// sizedItem is a PUSH row whose payload is about size bytes.
func sizedItem(id, group string, size int) Item {
	it := pushItem(id, group)
	head := `{"topic":"shop:a","type":"x","data":{"pad":"`
	tail := `"},"pushId":"` + id + `"}`
	pad := max(size-len(head)-len(tail), 0)
	it.Payload = []byte(head + strings.Repeat("x", pad) + tail)
	return it
}

// requestLog is a fake platform that records each request's pushIds and body
// size, and answers 413 for a body over limit (0: none), else what answer
// gives (nil: SUCCESS for every item).
type requestLog struct {
	mu       sync.Mutex
	requests [][]string
	sizes    []int
	limit    int
	answer   func(w http.ResponseWriter, ids []string) bool // true: answered
}

func (l *requestLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	ids := pushIDs(r)
	l.mu.Lock()
	l.requests = append(l.requests, ids)
	l.sizes = append(l.sizes, len(body))
	answer := l.answer
	l.mu.Unlock()
	if l.limit > 0 && len(body) > l.limit {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	if answer != nil && answer(w, ids) {
		return
	}
	st := make([]string, len(ids))
	for i := range st {
		st[i] = "SUCCESS"
	}
	writeResults(w, st...)
}

func (l *requestLog) sent() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.requests)
}

// bodySizes is the body size of each request, in the order of sent.
func (l *requestLog) bodySizes() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.sizes)
}

func ids(items ...Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestCutParts(t *testing.T) {
	mk := func(sizes ...int) []part {
		ps := make([]part, len(sizes))
		for i, n := range sizes {
			ps[i] = part{item: Item{ID: fmt.Sprint(i)}, body: make([]byte, n)}
		}
		return ps
	}
	shape := func(chunks [][]part) [][]string {
		out := make([][]string, len(chunks))
		for i, c := range chunks {
			for _, p := range c {
				out[i] = append(out[i], p.item.ID)
			}
		}
		return out
	}
	// A request of n parts is emptyRequestBytes + the parts + n-1 commas.
	for name, c := range map[string]struct {
		sizes             []int
		maxItems, maxSize int
		want              [][]string
	}{
		"no limits: one request":       {[]int{10, 10, 10}, 0, 0, [][]string{{"0", "1", "2"}}},
		"by count":                     {[]int{1, 1, 1, 1, 1}, 2, 0, [][]string{{"0", "1"}, {"2", "3"}, {"4"}}},
		"by bytes, exactly at the cap": {[]int{40, 40, 40}, 0, emptyRequestBytes + 81, [][]string{{"0", "1"}, {"2"}}},
		"by bytes, one byte short":     {[]int{40, 40, 40}, 0, emptyRequestBytes + 80, [][]string{{"0"}, {"1"}, {"2"}}},
		"an oversized part goes alone": {[]int{10, 500, 10, 10}, 0, 100, [][]string{{"0"}, {"1"}, {"2", "3"}}},
		"count first, then bytes":      {[]int{10, 10, 10, 60, 10}, 2, 100, [][]string{{"0", "1"}, {"2", "3"}, {"4"}}},
		"empty":                        {nil, 2, 100, [][]string{}},
	} {
		got := shape(cutParts(mk(c.sizes...), c.maxItems, c.maxSize))
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// Every request but one holding a single oversized part is within the cap.
	parts := mk(100, 300, 50, 50, 900, 20, 20, 20, 400)
	for _, chunk := range cutParts(parts, 0, 500) {
		if len(chunk) > 1 && requestBytes(chunk) > 500 {
			t.Errorf("a request of %d parts is %d bytes, over 500", len(chunk), requestBytes(chunk))
		}
	}
}

// A batch is sent in requests cut at BatchSize rows or MaxRequestBytes bytes,
// in claim order; a row larger than the limit goes alone; every row is
// delivered once.
func TestDispatchBatch_CutsByBytes(t *testing.T) {
	log := &requestLog{}
	srv := httptest.NewServer(log)
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BatchSize = 4
	cfg.MaxRequestBytes = 1000
	p := NewProcessor(cfg, repo)

	batch := []Item{
		sizedItem("u-01", "", 300), sizedItem("u-02", "", 300), sizedItem("u-03", "", 300),
		sizedItem("u-04", "", 2500), // over the limit: alone
		sizedItem("u-05", "", 100), sizedItem("u-06", "", 100), sizedItem("u-07", "", 100),
		sizedItem("u-08", "", 100), sizedItem("u-09", "", 100), // the count cuts here
	}
	p.inFlight.Add(int64(len(batch))) // as tick does
	p.dispatchBatch(context.Background(), batch)

	want := [][]string{{"u-01", "u-02", "u-03"}, {"u-04"}, {"u-05", "u-06", "u-07", "u-08"}, {"u-09"}}
	if got := log.sent(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	sent := log.sent()
	for i, n := range log.bodySizes() {
		if len(sent[i]) > 1 && n > cfg.MaxRequestBytes {
			t.Errorf("request %d of %d rows is %d bytes, over %d", i, len(sent[i]), n, cfg.MaxRequestBytes)
		}
	}
	if !slices.Equal(repo.succeeded, ids(batch...)) {
		t.Fatalf("succeeded %v, want every row in order", repo.succeeded)
	}
	if len(repo.failed) != 0 || len(repo.released) != 0 || p.InFlight() != 0 {
		t.Fatalf("failed=%v released=%v inFlight=%d", repo.failed, repo.released, p.InFlight())
	}
	if s, f := p.Totals(); s != uint64(len(batch)) || f != 0 {
		t.Fatalf("totals %d/%d", s, f)
	}
}

// A request the platform refuses as too large (413) is split in two and the
// halves sent in order, down to what the platform takes; every row is
// delivered and none is failed.
func TestDispatchBatch_413SplitsAndDelivers(t *testing.T) {
	batch := make([]Item, 8)
	for i := range batch {
		batch[i] = sizedItem(fmt.Sprintf("u-%02d", i+1), "", 400)
	}
	// The processor would send all eight in one request; the platform takes
	// about two rows a request.
	log := &requestLog{limit: 900}
	srv := httptest.NewServer(log)
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)
	p.dispatchBatch(context.Background(), batch)

	want := [][]string{
		ids(batch...),      // 413
		ids(batch[:4]...),  // 413
		ids(batch[:2]...),  // delivered
		ids(batch[2:4]...), // delivered
		ids(batch[4:]...),  // 413
		ids(batch[4:6]...), // delivered
		ids(batch[6:]...),  // delivered
	}
	if got := log.sent(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	if !slices.Equal(repo.succeeded, ids(batch...)) || len(repo.failed) != 0 || len(repo.released) != 0 {
		t.Fatalf("succeeded=%v failed=%v released=%v", repo.succeeded, repo.failed, repo.released)
	}
	if h := p.Holds(); len(h) != 0 {
		t.Fatalf("holds %+v: a split is not a failure", h)
	}
}

// One row refused as too large on its own is a terminal BAD_REQUEST that says
// so: recorded once, never sent again, and the rest of its batch is delivered.
func TestDispatchBatch_413OnOneRowIsTerminal(t *testing.T) {
	big := sizedItem("u-03", "", 2000)
	batch := []Item{sizedItem("u-01", "", 100), sizedItem("u-02", "", 100), big, sizedItem("u-04", "", 100)}
	log := &requestLog{limit: 1000}
	srv := httptest.NewServer(log)
	defer srv.Close()
	repo := newMemRepo(batch...)
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.PollInterval = 2 * time.Millisecond
	p := NewProcessor(cfg, repo)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	waitFor(t, 5*time.Second, "the other rows delivered", func() bool { return repo.remaining() == 1 })
	time.Sleep(50 * time.Millisecond) // more polls: the refused row is not claimed again
	cancel()
	<-done

	r, ok := repo.row(big.ID)
	if !ok || r.status != common.OutboxBadRequest || r.retries != 1 {
		t.Fatalf("the oversized row: %+v, want BAD_REQUEST after one attempt", r)
	}
	if !strings.HasPrefix(r.msg, BlockingMarker+"PAYLOAD_TOO_LARGE: ") || !strings.Contains(r.msg, "larger than the platform accepts") {
		t.Fatalf("stored message %q", r.msg)
	}
	sentBig := 0
	for _, req := range log.sent() {
		if len(req) == 1 && req[0] == big.ID {
			sentBig++
		}
	}
	if sentBig != 1 {
		t.Fatalf("the oversized row was sent alone %d times, want once", sentBig)
	}
	if h := p.Holds(); len(h) != 0 {
		t.Fatalf("holds %+v", h)
	}
}

// A grouped row is always sent alone; one the platform refuses as too large
// is a terminal BAD_REQUEST, which blocks its group (BlockOnError) until an
// operator skips or retries it, as any refused row does. It is sent once, not
// retried for ever; with BlockOnError off its group carries on past it.
func TestDispatch_413OnAGroupedRow(t *testing.T) {
	for _, blockOnError := range []bool{true, false} {
		t.Run(fmt.Sprintf("BlockOnError=%v", blockOnError), func(t *testing.T) {
			log := &requestLog{limit: 1000}
			srv := httptest.NewServer(log)
			defer srv.Close()
			repo := &recordRepo{}
			cfg := DefaultConfig()
			cfg.PlatformURL = srv.URL
			cfg.BlockOnError = blockOnError
			p := NewProcessor(cfg, repo)
			if p.dispatch(context.Background(), sizedItem("g-01", "shop:a", 2000)) {
				t.Fatal("a row refused as too large is not delivered")
			}
			if len(log.sent()) != 1 || len(repo.requeues) != 1 || repo.requeues[0] {
				t.Fatalf("requests %v requeues %v: refused once, terminal", log.sent(), repo.requeues)
			}
			if !strings.Contains(repo.messages[0], "PAYLOAD_TOO_LARGE") {
				t.Fatalf("message %q", repo.messages[0])
			}
			if p.groups.IsActive("shop:a") == blockOnError {
				t.Fatalf("group active=%v with BlockOnError=%v", p.groups.IsActive("shop:a"), blockOnError)
			}
			if h := p.Holds(); len(h) != 0 {
				t.Fatalf("holds %+v: a terminal refusal is not retried", h)
			}
			if blockOnError {
				if ok, err := p.SkipGroup(context.Background(), "shop:a"); !ok || err != nil || !p.groups.IsActive("shop:a") {
					t.Fatalf("skip: %v %v", ok, err)
				}
			}
		})
	}
}

// A request that fails as a whole (503) stops its batch: the later requests
// are not sent, their rows go back to PENDING untouched (no attempt counted),
// the failed request's rows are requeued and the type is held.
func TestDispatchBatch_AFailedRequestStopsTheRest(t *testing.T) {
	log := &requestLog{answer: func(w http.ResponseWriter, ids []string) bool {
		if ids[0] == "u-01" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}}
	srv := httptest.NewServer(log)
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BatchSize = 2
	p := NewProcessor(cfg, repo)
	batch := rowsFor("u", "", 6, time.Now())
	p.dispatchBatch(context.Background(), batch)

	if got := log.sent(); len(got) != 1 {
		t.Fatalf("requests %v: only the first is sent", got)
	}
	if !slices.Equal(repo.failed, ids(batch[:2]...)) || slices.Contains(repo.requeues, false) {
		t.Fatalf("failed %v requeues %v: the failed request's rows are requeued", repo.failed, repo.requeues)
	}
	if !slices.Equal(repo.released, ids(batch[2:]...)) {
		t.Fatalf("released %v: the unsent rows go back untouched", repo.released)
	}
	if h := p.Holds(); len(h) != 1 || h[0].Key != "type:PUSH" {
		t.Fatalf("holds %+v, want the PUSH type held", h)
	}
}

// A 429 on a later request holds the type and puts its rows and the rest back
// untouched; the earlier requests' rows are delivered.
func TestDispatchBatch_A429StopsTheRest(t *testing.T) {
	log := &requestLog{answer: func(w http.ResponseWriter, ids []string) bool {
		if ids[0] == "u-03" {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			return true
		}
		return false
	}}
	srv := httptest.NewServer(log)
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BatchSize = 2
	p := NewProcessor(cfg, repo)
	batch := rowsFor("u", "", 6, time.Now())
	p.dispatchBatch(context.Background(), batch)

	if got := log.sent(); len(got) != 2 {
		t.Fatalf("requests %v", got)
	}
	if !slices.Equal(repo.succeeded, ids(batch[:2]...)) || len(repo.failed) != 0 {
		t.Fatalf("succeeded %v failed %v", repo.succeeded, repo.failed)
	}
	released := slices.Clone(repo.released)
	slices.Sort(released)
	if !slices.Equal(released, ids(batch[2:]...)) {
		t.Fatalf("released %v", repo.released)
	}
	if h := p.Holds(); len(h) != 1 || h[0].Key != "type:PUSH" || h[0].ConsecutiveFailures != 0 {
		t.Fatalf("holds %+v", h)
	}
}

// The platform's per-item refusals of a payload: PAYLOAD_TOO_LARGE and
// PAYLOAD_NOT_TEXT (BAD_REQUEST) are terminal, PAYLOAD_STORE_UNAVAILABLE
// (INTERNAL_ERROR) is retried; for a grouped row the first blocks its group,
// the second holds it.
func TestPayloadRefusalsPerItem(t *testing.T) {
	results := map[string][2]string{
		"u-1": {"BAD_REQUEST", "PAYLOAD_TOO_LARGE: payloadstore: payload too large: 6000000 bytes, the largest accepted is 5000000"},
		"u-2": {"BAD_REQUEST", "PAYLOAD_NOT_TEXT: payloadstore: payload is not UTF-8 text: contains a NUL byte"},
		"u-3": {"INTERNAL_ERROR", "PAYLOAD_STORE_UNAVAILABLE: the payload store could not be written and the payload is too large to keep in the database; send it again later"},
		"u-4": {"SUCCESS", ""},
		"g-1": {"BAD_REQUEST", "PAYLOAD_TOO_LARGE: payloadstore: payload too large"},
		"h-1": {"INTERNAL_ERROR", "PAYLOAD_STORE_UNAVAILABLE: send it again later"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := pushIDs(r)
		res := make([]map[string]string, len(got))
		for i, id := range got {
			res[i] = map[string]string{"status": results[id][0], "error": results[id][1]}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)

	p.dispatchBatch(context.Background(), []Item{pushItem("u-1", ""), pushItem("u-2", ""), pushItem("u-3", ""), pushItem("u-4", "")})
	want := map[string]bool{"u-1": false, "u-2": false, "u-3": true} // requeued?
	for i, id := range repo.failed {
		rq, ok := want[id]
		if !ok || repo.requeues[i] != rq {
			t.Errorf("%s: requeue=%v, want %v", id, repo.requeues[i], rq)
		}
		if !rq && !strings.HasPrefix(repo.messages[i], BlockingMarker+results[id][1][:16]) {
			t.Errorf("%s: message %q", id, repo.messages[i])
		}
	}
	if len(repo.failed) != 3 || !slices.Equal(repo.succeeded, []string{"u-4"}) {
		t.Fatalf("failed %v succeeded %v", repo.failed, repo.succeeded)
	}
	if h := p.Holds(); len(h) != 1 || h[0].Key != "item:u-3" {
		t.Fatalf("holds %+v: only the retryable row is held", h)
	}

	p.dispatch(context.Background(), pushItem("g-1", "shop:g"))
	if p.groups.IsActive("shop:g") {
		t.Fatal("PAYLOAD_TOO_LARGE blocks its group")
	}
	p.dispatch(context.Background(), pushItem("h-1", "shop:h"))
	if !p.groups.IsActive("shop:h") || !p.held(pushItem("h-2", "shop:h")) {
		t.Fatal("PAYLOAD_STORE_UNAVAILABLE holds its group, never blocks it")
	}
}

// Through Run against an in-memory table, with the cut and the platform's 413
// in play: no request ever carries two rows of a message group or a grouped
// row with any other row; no grouped row is sent while an earlier row of its
// group is undelivered; a grouped row refused as too large blocks its group
// and its later rows are never sent; a retryable failure of a grouped row
// holds the rest of its group behind it; every ungrouped row is delivered,
// an oversized one alone.
func TestCutAndSplitKeepGroupsInOrder(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	mk := func(prefix, group string, sizes ...int) []Item {
		out := make([]Item, len(sizes))
		for i, n := range sizes {
			out[i] = sizedItem(fmt.Sprintf("%s-%02d", prefix, i+1), group, n)
			out[i].CreatedAt = at.Add(time.Duration(i) * time.Millisecond)
		}
		return out
	}
	a := mk("a", "shop:a", 300, 300, 5000, 300, 300) // a-03 is over the platform's cap
	b := mk("b", "shop:b", 300, 2500, 300, 300, 300) // b-02 is over the processor's limit, fails once
	u := mk("u", "", 600, 600, 600, 600, 3500, 600, 600, 600, 600, 600, 600, 600)
	all := slices.Concat(a, b, u)
	repo := newMemRepo(all...)
	groupOf := map[string]string{}
	written := map[string][]string{}
	for _, it := range all {
		if grouped(it) {
			groupOf[it.ID] = *it.MessageGroup
			written[*it.MessageGroup] = append(written[*it.MessageGroup], it.ID)
		}
	}

	var mu sync.Mutex
	delivered := map[string]bool{}
	order := map[string][]string{}
	var violations []string
	failOnce := map[string]bool{"b-02": true}
	log := &requestLog{limit: 4000, answer: func(w http.ResponseWriter, ids []string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range ids {
			if g, ok := groupOf[id]; ok {
				if len(ids) > 1 {
					violations = append(violations, fmt.Sprintf("grouped row %s shares a request: %v", id, ids))
				}
				for _, earlier := range written[g] {
					if earlier == id {
						break
					}
					if !delivered[earlier] {
						violations = append(violations, id+" sent before "+earlier)
					}
				}
			}
		}
		if len(ids) == 1 && failOnce[ids[0]] {
			delete(failOnce, ids[0])
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		for _, id := range ids {
			delivered[id] = true
			if g, ok := groupOf[id]; ok {
				order[g] = append(order[g], id)
			}
		}
		return false
	}}
	srv := httptest.NewServer(log)
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.PollInterval = 2 * time.Millisecond
	cfg.BatchSize = 6
	cfg.MaxRequestBytes = 2000
	cfg.RetryBackoffBase = 5 * time.Millisecond
	cfg.RetryBackoffMax = 20 * time.Millisecond
	p := NewProcessor(cfg, repo)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	// Left: a-03 (refused) and a-04, a-05 behind it.
	waitFor(t, 15*time.Second, "everything but the blocked group delivered", func() bool { return repo.remaining() == 3 })
	time.Sleep(50 * time.Millisecond) // more polls: the blocked group stays put
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(violations) > 0 {
		t.Fatalf("violations: %v", violations)
	}
	if want := ids(a[:2]...); !slices.Equal(order["shop:a"], want) {
		t.Fatalf("shop:a delivered %v, want %v", order["shop:a"], want)
	}
	if want := ids(b...); !slices.Equal(order["shop:b"], want) {
		t.Fatalf("shop:b delivered %v, want %v", order["shop:b"], want)
	}
	for _, it := range u {
		if !delivered[it.ID] {
			t.Fatalf("%s not delivered", it.ID)
		}
	}
	if r, _ := repo.row("a-03"); r.status != common.OutboxBadRequest || !strings.Contains(r.msg, "PAYLOAD_TOO_LARGE") {
		t.Fatalf("a-03: %+v", r)
	}
	for _, id := range []string{"a-04", "a-05"} {
		if r, _ := repo.row(id); r.status != common.OutboxPending || r.retries != 0 {
			t.Fatalf("%s: %+v, want pending and never tried", id, r)
		}
	}
	if blocked := p.BlockedGroups(); len(blocked) != 1 || blocked[0].Group != "shop:a" || blocked[0].BlockedItemID != "a-03" {
		t.Fatalf("blocked %+v", blocked)
	}
	sizes := log.bodySizes()
	for i, req := range log.sent() {
		if len(req) > 1 && sizes[i] > cfg.MaxRequestBytes {
			t.Fatalf("request %v is %d bytes, over the processor's %d", req, sizes[i], cfg.MaxRequestBytes)
		}
	}
}
