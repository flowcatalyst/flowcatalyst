package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"

	outboxmongo "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/mongo"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/outboxsql"
)

// newOutboxCreateTableCmd provisions the SDK `outbox_messages` table (or, for
// MongoDB, the collection's indexes) in a consumer app's database. It is the
// runnable form of the per-language SDK migrations
// (clients/*/migrations/{postgresql,mysql}/001_create_outbox_messages.sql and
// the Laravel/Mongo equivalents) so a consumer app on any supported store can
// provision the outbox with one command instead of hand-running DDL.
//
// The schema is identical across all SDKs and matches what the outbox
// processor (fc-server / fc-dev outbox / the Rust + Java processors) reads, so
// it doesn't matter which SDK or runtime writes the rows.
func newOutboxCreateTableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-table",
		Short: "Create the outbox_messages table/collection in a consumer app's DB",
		Long: `Create the SDK outbox storage in a consumer application's database.

Supports the three stores the FlowCatalyst SDKs target:

  postgres  CREATE TABLE outbox_messages (+ indexes)        [--db-type pg]
  mysql     CREATE TABLE outbox_messages (+ indexes)        [--db-type mysql]
  mongodb   create the outbox_messages collection's indexes [--db-type mongodb]

The shape is identical to every SDK migration and to what the outbox processor
reads, so it doesn't matter whether the SDK or this command provisions it
first. All three are idempotent — safe to re-run.

Examples:
  fc-dev outbox create-table --db-type pg     --db-url postgres://user:pass@localhost:5432/app
  fc-dev outbox create-table --db-type mysql  --db-url 'user:pass@tcp(localhost:3306)/app'
  fc-dev outbox create-table --db-type mongodb --db-url mongodb://localhost:27017 --db-name app`,
		Args: cobra.NoArgs,
		RunE: runOutboxCreateTable,
	}
	cmd.Flags().String("db-type", envFirst("postgres", "FC_OUTBOX_BACKEND", "FC_OUTBOX_DB_TYPE"),
		"target store: postgres | mysql | mongodb")
	cmd.Flags().String("db-url", envFirst("", "FC_OUTBOX_SOURCE_DB_URL", "FC_OUTBOX_DB_URL", "FC_OUTBOX_MONGO_URI"),
		"connection string/URL (postgres + mongo URLs, or a Go MySQL DSN) (required)")
	cmd.Flags().String("db-name", envStrDefault("FC_OUTBOX_MONGO_DB", "flowcatalyst"),
		"MongoDB database name (mongodb only)")
	return cmd
}

func runOutboxCreateTable(cmd *cobra.Command, _ []string) error {
	// Re-resolve from env after the parent's PersistentPreRunE loaded ./.env
	// (flag defaults were baked at command-build time). Explicit flags win.
	dbType, _ := cmd.Flags().GetString("db-type")
	if !cmd.Flags().Changed("db-type") {
		dbType = envFirst(dbType, "FC_OUTBOX_BACKEND", "FC_OUTBOX_DB_TYPE")
	}
	dbURL, _ := cmd.Flags().GetString("db-url")
	if !cmd.Flags().Changed("db-url") {
		dbURL = envFirst(dbURL, "FC_OUTBOX_SOURCE_DB_URL", "FC_OUTBOX_DB_URL", "FC_OUTBOX_MONGO_URI")
	}
	dbName, _ := cmd.Flags().GetString("db-name")
	if !cmd.Flags().Changed("db-name") {
		dbName = envStrDefault("FC_OUTBOX_MONGO_DB", dbName)
	}

	if dbURL == "" {
		return errors.New("--db-url (or FC_OUTBOX_SOURCE_DB_URL / FC_OUTBOX_DB_URL / FC_OUTBOX_MONGO_URI) is required")
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	switch normalizeOutboxDBType(dbType) {
	case "postgres":
		return createOutboxPostgres(ctx, out, dbURL)
	case "mysql":
		return createOutboxMySQL(ctx, out, dbURL)
	case "mongodb":
		return createOutboxMongo(ctx, out, dbURL, dbName)
	default:
		return fmt.Errorf("unknown --db-type %q: want postgres, mysql, or mongodb", dbType)
	}
}

// normalizeOutboxDBType folds the accepted aliases onto a canonical key.
func normalizeOutboxDBType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "pg", "postgres", "postgresql":
		return "postgres"
	case "mysql", "mariadb", "maria":
		return "mysql"
	case "mongo", "mongodb":
		return "mongodb"
	default:
		return ""
	}
}

func createOutboxPostgres(ctx context.Context, out io.Writer, dbURL string) error {
	pool, err := database.NewPool(ctx, database.Config{URL: dbURL})
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := outboxpg.New(pool).InitSchema(ctx); err != nil {
		return fmt.Errorf("create outbox table: %w", err)
	}
	fmt.Fprintln(out, "Created outbox_messages table + indexes (postgres).")
	return nil
}

func createOutboxMySQL(ctx context.Context, out io.Writer, dbURL string) error {
	dsn, err := mysqlDSN(dbURL)
	if err != nil {
		return err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}
	if _, err := db.ExecContext(ctx, outboxsql.CreateOutboxTableSQLMySQL); err != nil {
		return fmt.Errorf("create outbox table: %w", err)
	}
	fmt.Fprintln(out, "Created outbox_messages table + indexes (mysql).")
	return nil
}

func createOutboxMongo(ctx context.Context, out io.Writer, uri, dbName string) error {
	repo, err := outboxmongo.Connect(ctx, uri, dbName)
	if err != nil {
		return fmt.Errorf("connect mongodb: %w", err)
	}
	defer func() { _ = repo.Close(ctx) }()
	if err := repo.InitSchema(ctx); err != nil {
		return fmt.Errorf("create outbox indexes: %w", err)
	}
	fmt.Fprintf(out, "Created outbox_messages collection indexes (mongodb, db %q).\n", dbName)
	return nil
}

// mysqlDSN accepts either a Go MySQL DSN (passed through verbatim, so callers
// keep full control of TLS/params) or a mysql://user:pass@host:port/db URL,
// which it reformats to a DSN. The go-sql-driver does not understand URLs.
func mysqlDSN(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		return raw, nil // already a DSN
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse mysql url: %w", err)
	}
	cfg := gomysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = u.Host
	if cfg.Addr != "" && !strings.Contains(cfg.Addr, ":") {
		cfg.Addr += ":3306"
	}
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Passwd, _ = u.User.Password()
	}
	if q := u.Query(); len(q) > 0 {
		cfg.Params = make(map[string]string, len(q))
		for k, vs := range q {
			if len(vs) > 0 {
				cfg.Params[k] = vs[0]
			}
		}
	}
	return cfg.FormatDSN(), nil
}
