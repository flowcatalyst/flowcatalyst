package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The sources: one table per database-poll ingestion and one, api_feed, that
// the api-poll feed serves. A row is one record; ids increase in commit order
// (one writer per table, each insert its own transaction), which a watermark
// on id needs.

func createSourceTables(ctx context.Context, db *pgxpool.Pool, specs []ingestionSpec) error {
	for _, s := range specs {
		_, err := db.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (
			id     BIGSERIAL PRIMARY KEY,
			fed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
			note   TEXT NOT NULL)`, s.table))
		if err != nil {
			return fmt.Errorf("create %s: %w", s.table, err)
		}
	}
	return nil
}

// feeder inserts a row into every source table at the configured rate until
// stopped.
type feeder struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	errs   map[string]int
}

func startFeeder(parent context.Context, db *pgxpool.Pool, specs []ingestionSpec, rate float64) *feeder {
	ctx, cancel := context.WithCancel(parent)
	f := &feeder{cancel: cancel, errs: map[string]int{}}
	every := time.Duration(float64(time.Second) / rate)
	for _, s := range specs {
		f.wg.Go(func() {
			tick := time.NewTicker(every)
			defer tick.Stop()
			n := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				n++
				if _, err := db.Exec(ctx, "INSERT INTO "+s.table+" (note) VALUES ($1)", s.code+" record "+strconv.Itoa(n)); err != nil && ctx.Err() == nil {
					f.mu.Lock()
					f.errs[s.table]++
					first := f.errs[s.table] == 1
					f.mu.Unlock()
					if first {
						log.Printf("feeder: insert into %s failed: %v", s.table, err)
					}
				}
			}
		})
	}
	return f
}

// stop ends the feeding and answers the failed inserts per table.
func (f *feeder) stop() map[string]int {
	f.cancel()
	f.wg.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errs
}

// feedHandler is the tiny HTTP API the api-poll ingestion reads:
// GET /feed?after=N answers {"items":[{"id","fed_at","note"}...], "next"} with
// the rows past N in id order (at most 100) and the cursor for the next
// request (the last id, or N again when there is nothing new). The answer to
// a cursor only ever grows at its end, so a record keeps its position, which
// is what the api-poll idempotency key is made of.
func feedHandler(db *pgxpool.Pool, table string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /feed", func(w http.ResponseWriter, r *http.Request) {
		after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		if err != nil {
			after = 0
		}
		rows, err := db.Query(r.Context(), "SELECT id, fed_at, note FROM "+table+" WHERE id > $1 ORDER BY id LIMIT 100", after)
		if err != nil {
			http.Error(w, "query failed", http.StatusServiceUnavailable)
			return
		}
		type item struct {
			ID    int64     `json:"id"`
			FedAt time.Time `json:"fed_at"`
			Note  string    `json:"note"`
		}
		items := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.FedAt, &it.Note); err != nil {
				rows.Close()
				http.Error(w, "scan failed", http.StatusServiceUnavailable)
				return
			}
			items = append(items, it)
		}
		rows.Close()
		if rows.Err() != nil {
			http.Error(w, "query failed", http.StatusServiceUnavailable)
			return
		}
		next := after
		if len(items) > 0 {
			next = items[len(items)-1].ID
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "next": strconv.FormatInt(next, 10)})
	})
	return mux
}

// receiverHandler takes every delivery with 200 {"ack":true}: the jobs are
// what the test checks, the deliveries only keep the platform's dispatch
// path busy as it would be.
func receiverHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ack":true}`))
	})
}

// serve listens on 127.0.0.1:port and serves h until ctx ends.
func serve(ctx context.Context, port int, h http.Handler) (*http.Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(l) }()
	go func() {
		<-ctx.Done()
		// ctx is over by now; the shutdown gets its own short bound.
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	return srv, nil
}

// writeHostsFile is the runtimes' name table (-Djdk.net.hosts.file): with it
// set the JVM resolves only these names, so it lists localhost and this
// machine's own name too.
func writeHostsFile(state string) error {
	lines := "127.0.0.1 localhost\n127.0.0.1 " + sourceHost + "\n127.0.0.1 " + feedHost + "\n"
	if h, err := os.Hostname(); err == nil && h != "" {
		lines += "127.0.0.1 " + h + "\n"
	}
	return os.WriteFile(filepath.Join(state, "hosts"), []byte(lines), 0o600)
}
