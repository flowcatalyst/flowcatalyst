//go:build integration

package mongo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	mongodrv "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxmongo "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/mongo"
)

// collection connects to FC_TEST_MONGO_URI (skipping without it) and gives the test a database of
// its own, dropped afterwards.
func collection(t *testing.T) (*outboxmongo.Repository, *mongodrv.Collection) {
	t.Helper()
	uri := os.Getenv("FC_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("FC_TEST_MONGO_URI not set")
	}
	client, err := mongodrv.Connect(t.Context(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := "fc_outbox_" + strings.ToLower(strings.NewReplacer("/", "_", "=", "_").Replace(t.Name()))
	db = db[:min(len(db), 60)]
	t.Cleanup(func() {
		_ = client.Database(db).Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	repo := outboxmongo.New(client, db)
	require.NoError(t, repo.InitSchema(t.Context()))
	return repo, client.Database(db).Collection("outbox_messages")
}

// insert writes a doc in the SDK's shape; group "" omits message_group, "<empty>" stores "".
func insert(t *testing.T, coll *mongodrv.Collection, id, typ, group string, created time.Time, status, retries int, msg string) {
	t.Helper()
	d := bson.M{
		"id": id, "type": typ,
		"payload": fmt.Sprintf(`{"topic":"shop:a","type":"x","data":{},"pushId":%q}`, id),
		"status":  status, "retry_count": retries,
		"created_at": created.UTC().Format(time.RFC3339), "updated_at": created.UTC().Format(time.RFC3339),
	}
	switch group {
	case "":
	case "<empty>":
		d["message_group"] = ""
	default:
		d["message_group"] = group
	}
	if msg != "" {
		d["error_message"] = msg
	}
	_, err := coll.InsertOne(t.Context(), d)
	require.NoError(t, err)
}

type stored struct {
	Status       int    `bson:"status"`
	RetryCount   int    `bson:"retry_count"`
	ErrorMessage string `bson:"error_message"`
}

func find(t *testing.T, coll *mongodrv.Collection, id string) (stored, bool) {
	t.Helper()
	var s stored
	if err := coll.FindOne(t.Context(), bson.M{"id": id}).Decode(&s); err != nil {
		return stored{}, false
	}
	return s, true
}

func ids(items []outbox.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	slices.Sort(out)
	return out
}

func TestMongoClaimPassesOverTheFilter(t *testing.T) {
	repo, coll := collection(t)
	at := time.Now().Add(-time.Hour)
	for i := range 3 {
		insert(t, coll, fmt.Sprintf("a-%02d", i+1), "PUSH", "aaa", at.Add(time.Duration(i)*time.Second), 0, 0, "")
	}
	insert(t, coll, "b-01", "PUSH", "bbb", at, 0, 0, "")
	insert(t, coll, "u-01", "PUSH", "", at, 0, 0, "")
	insert(t, coll, "e-01", "EVENT", "<empty>", at, 0, 0, "")

	got, err := repo.ClaimPending(t.Context(), 10, outbox.ClaimFilter{
		Groups: []string{"aaa"}, Types: []common.OutboxItemType{common.OutboxItemPush}, IDs: []string{"e-01"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"b-01"}, ids(got))
	require.NoError(t, repo.Release(t.Context(), []string{"b-01"}))

	got, err = repo.ClaimPending(t.Context(), 10, outbox.ClaimFilter{Groups: []string{"bbb"}, Types: []common.OutboxItemType{common.OutboxItemEvent}})
	require.NoError(t, err)
	assert.Equal(t, []string{"a-01", "a-02", "a-03", "u-01"}, ids(got), "an empty-string group counts as ungrouped")
}

func TestMongoSweepBlockingFailuresAndSkip(t *testing.T) {
	repo, coll := collection(t)
	at := time.Now().Add(-time.Hour)
	insert(t, coll, "l-01", "PUSH", "gl", at, int(common.OutboxInternalError), 3, "500")
	insert(t, coll, "l-02", "PUSH", "", at, int(common.OutboxUnauthorized), 3, "401")
	insert(t, coll, "l-03", "PUSH", "", at, int(common.OutboxGatewayError), 3, "503")
	insert(t, coll, "x-01", "PUSH", "gx", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"first")
	insert(t, coll, "x-02", "PUSH", "gx", at.Add(time.Second), int(common.OutboxForbidden), 1, outbox.BlockingMarker+"second")
	insert(t, coll, "y-01", "PUSH", "gy", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker)
	insert(t, coll, "v-01", "PUSH", "gv", at, int(common.OutboxBadRequest), 1, "400 old")
	insert(t, coll, "v-02", "PUSH", "gv2", at, int(common.OutboxForbidden), 1, "")
	insert(t, coll, "s-01", "PUSH", "gs", at, int(common.OutboxBadRequest), 1, outbox.SkippedMarker+"400")
	insert(t, coll, "z-01", "PUSH", "", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"ungrouped")

	n, err := repo.RequeueRetryableFailures(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	s, _ := find(t, coll, "l-01")
	assert.Equal(t, stored{Status: 0, RetryCount: 3, ErrorMessage: "500"}, s)

	// an earlier version's refusals (no marker) are marked skipped before upgrade, once
	n, err = repo.MarkLegacyRefusals(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	s, _ = find(t, coll, "v-01")
	assert.Equal(t, outbox.SkippedBeforeUpgradeMarker+"400 old", s.ErrorMessage)
	s, _ = find(t, coll, "v-02")
	assert.Equal(t, outbox.SkippedBeforeUpgradeMarker, s.ErrorMessage)
	s, _ = find(t, coll, "s-01")
	assert.Equal(t, outbox.SkippedMarker+"400", s.ErrorMessage)
	n, err = repo.MarkLegacyRefusals(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)

	heads, err := repo.BlockingFailures(t.Context())
	require.NoError(t, err)
	got := map[string]string{}
	for _, h := range heads {
		got[*h.MessageGroup] = h.ID
	}
	assert.Equal(t, map[string]string{"gx": "x-01", "gy": "y-01"}, got)

	require.NoError(t, repo.MarkSkipped(t.Context(), []string{"x-01", "y-01"}))
	require.NoError(t, repo.MarkSkipped(t.Context(), []string{"x-01"}))
	s, _ = find(t, coll, "x-01")
	assert.Equal(t, stored{Status: int(common.OutboxBadRequest), RetryCount: 1, ErrorMessage: outbox.SkippedMarker + "first"}, s)
	s, _ = find(t, coll, "y-01")
	assert.Equal(t, outbox.SkippedMarker, s.ErrorMessage)

	heads, err = repo.BlockingFailures(t.Context())
	require.NoError(t, err)
	require.Len(t, heads, 1)
	assert.Equal(t, "x-02", heads[0].ID)
}

// The processor on Mongo: a group blocked by a stored non-retryable failure stays blocked across
// a restart, and a retryable outage loses nothing.
func TestMongoProcessorRestartAndOutage(t *testing.T) {
	repo, coll := collection(t)
	at := time.Now().Add(-time.Hour)
	insert(t, coll, "p-01", "PUSH", "gp", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"400")
	insert(t, coll, "p-02", "PUSH", "gp", at.Add(time.Second), 0, 0, "")
	insert(t, coll, "o-01", "PUSH", "go", at, int(common.OutboxBadRequest), 1, "400 old") // an earlier version's
	insert(t, coll, "o-02", "PUSH", "go", at.Add(time.Second), 0, 0, "")
	for i := range 4 {
		insert(t, coll, fmt.Sprintf("q-%02d", i+1), "PUSH", "gq", at.Add(time.Duration(i)*time.Second), 0, 0, "")
	}

	var mu sync.Mutex
	var delivered []string
	attempts := map[string]int{}
	outageEnds := time.Now().Add(2 * time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		for _, it := range body.Items {
			attempts[it.PushID]++
		}
		if time.Now().Before(outageEnds) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		res := make([]map[string]string, len(body.Items))
		for i, it := range body.Items {
			res[i] = map[string]string{"status": "SUCCESS"}
			delivered = append(delivered, it.PushID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
	}))
	defer srv.Close()

	start := func() (*outbox.Processor, func()) {
		cfg := outbox.DefaultConfig()
		cfg.PlatformURL = srv.URL
		cfg.PollInterval = 20 * time.Millisecond
		cfg.RetryBackoffBase = 50 * time.Millisecond
		cfg.RetryBackoffMax = 300 * time.Millisecond
		p := outbox.NewProcessor(cfg, repo)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { p.Run(ctx); close(done) }()
		return p, func() { cancel(); <-done }
	}

	_, stop := start()
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, q4 := find(t, coll, "q-04")
		_, o2 := find(t, coll, "o-02")
		if !q4 && !o2 {
			break
		}
		require.False(t, time.Now().After(deadline), "gq and go not delivered through the outage")
		time.Sleep(20 * time.Millisecond)
	}
	stop()

	p, stop := start()
	time.Sleep(300 * time.Millisecond)
	stop()
	b := p.BlockedGroups()
	require.Len(t, b, 1)
	assert.Equal(t, "p-01", b[0].BlockedItemID)
	s, ok := find(t, coll, "p-02")
	require.True(t, ok)
	assert.Equal(t, 0, s.Status)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"o-02", "q-01", "q-02", "q-03", "q-04"}, func() []string {
		out := slices.Clone(delivered)
		slices.Sort(out)
		return out
	}(), "the earlier version's refusal does not block go")
	var q []string
	for _, id := range delivered {
		if strings.HasPrefix(id, "q-") {
			q = append(q, id)
		}
	}
	assert.Equal(t, []string{"q-01", "q-02", "q-03", "q-04"}, q, "gq in order")
	assert.Equal(t, "400", b[0].Error, "shown without the marker")
	assert.Greater(t, attempts["q-01"], 3, "the head row outlived the old cap")
	assert.Zero(t, attempts["p-02"])
}
