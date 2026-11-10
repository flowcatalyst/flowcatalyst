package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/keyrotation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

// Exit statuses of `fc-server reencrypt`.
const (
	reencryptOK         = 0
	reencryptFailed     = 1 // configuration, connection or database error (rerun to resume)
	reencryptUsage      = 2
	reencryptUnreadable = 3 // finished, but some values no configured key opens
)

const reencryptUsageText = `usage: fc-server reencrypt [--dry-run] [--batch-size N]

Re-encrypts every value stored encrypted under FLOWCATALYST_APP_KEY with the
current key, after a key rotation. Values are decrypted with the current key or
any key in FLOWCATALYST_APP_KEY_PREVIOUS (comma-separated) and rewritten one
row at a time by compare-and-set, so it is safe to run while the platform
serves traffic, and safe to rerun (values already on the current key are
skipped). Run it only once every platform process has been deployed with the
new key as current. See docs/key-rotation.md.

Connects to the database the server would use (FC_DATABASE_URL / DATABASE_URL,
or DB_SECRET_ARN + DB_HOST). Does not run migrations.

Exit status: 0 done; 1 error (rerun to resume); 2 usage; 3 done, but some
values could not be decrypted with any configured key (listed in the log by
row, never by value).

`

// runReencrypt implements `fc-server reencrypt`.
func runReencrypt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reencrypt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = io.WriteString(stderr, reencryptUsageText)
		fs.PrintDefaults()
	}
	dryRun := fs.Bool("dry-run", false, "count what would be re-encrypted; write nothing")
	batch := fs.Int("batch-size", 500, "rows read per query")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return reencryptOK
		}
		return reencryptUsage
	}
	if fs.NArg() > 0 || *batch <= 0 {
		fs.Usage()
		return reencryptUsage
	}

	enc, err := encryption.FromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "reencrypt: %v\n", err)
		return reencryptFailed
	}
	if enc == nil {
		_, _ = fmt.Fprintln(stderr, "reencrypt: FLOWCATALYST_APP_KEY is not set")
		return reencryptFailed
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := reencryptPool(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "reencrypt: %v\n", err)
		return reencryptFailed
	}
	defer pool.Close()

	slog.Info("reencrypt: starting", "dryRun", *dryRun, "batchSize", *batch)
	results, runErr := keyrotation.Run(ctx, pool, enc, keyrotation.Options{DryRun: *dryRun, BatchSize: *batch})
	unreadable := printReencryptResults(stdout, results, *dryRun)
	if runErr != nil {
		_, _ = fmt.Fprintf(stderr, "reencrypt: stopped: %v (rerun to resume)\n", runErr)
		return reencryptFailed
	}
	if unreadable > 0 {
		_, _ = fmt.Fprintf(stderr, "reencrypt: %d value(s) no configured key opens were left as they are\n", unreadable)
		return reencryptUnreadable
	}
	return reencryptOK
}

// reencryptPool connects to the database fc-server would use.
func reencryptPool(ctx context.Context) (*pgxpool.Pool, error) {
	url := server.ResolveDatabaseURL()
	if dbURL, ok, err := server.ResolveDBSecretURL(ctx); err != nil {
		return nil, fmt.Errorf("resolve DB secret: %w", err)
	} else if ok {
		url = dbURL
	}
	if url == "" {
		return nil, errors.New("no database configured (FC_DATABASE_URL / DATABASE_URL)")
	}
	return database.NewPool(ctx, database.Config{URL: url, MaxConnections: 2})
}

// printReencryptResults writes one line per target and returns the total of
// unreadable values.
func printReencryptResults(w io.Writer, results []keyrotation.Result, dryRun bool) int {
	moved := "REENCRYPTED"
	if dryRun {
		moved = "WOULD_REENCRYPT"
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "TARGET\tSCANNED\tCURRENT\t%s\tCHANGED_MEANWHILE\tUNREADABLE\tHASHED\tEXTERNAL\tNOT_ENCRYPTED\n", moved)
	unreadable := 0
	for _, r := range results {
		unreadable += r.Unreadable
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			r.Target, r.Scanned, r.Current, r.Reencrypted, r.Raced, r.Unreadable, r.Hashed, r.External, r.NotEncrypted)
	}
	_ = tw.Flush()
	return unreadable
}
