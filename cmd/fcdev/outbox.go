package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
)

// newOutboxCmd runs the SDK outbox poller pointed at an external app's
// Postgres + an external FlowCatalyst platform. Use this when the
// consumer app's DB can't be the embedded one (e.g. it is a shared or
// production-like database of its own, or needs an extension the embedded
// Postgres lacks; PostGIS it can have, through `fcdev install postgis`).
func newOutboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outbox",
		Short: "Standalone outbox poller against an external app DB → external platform",
		Long: `Run the outbox poller against an application's Postgres database: it claims the rows
the application writes to outbox_messages and delivers them to the FlowCatalyst platform.

Who creates outbox_messages (--init-schema, env FC_OUTBOX_INIT_SCHEMA, default true):

  true   At start the poller runs CREATE TABLE IF NOT EXISTS outbox_messages and its three
         indexes (the same DDL as 'fcdev outbox create-table'). For an application that has
         no migration of its own for the table.

  false  The poller runs no DDL at all against the database. If outbox_messages is not there
         yet it waits for it (it checks every 2 seconds and logs that it is waiting, again
         every minute while it waits), then starts polling. A database it cannot reach, or a
         wrong password, is still an error.

Set --init-schema=false (FC_OUTBOX_INIT_SCHEMA=false) whenever the application's own
migrations create outbox_messages: the Go framework's outbox module, or an SDK migration
(clients/*/migrations) run by the application's migration tool. Those migrations use a plain
CREATE TABLE; if the poller starts first (a second container of the same task, started
together with the application) and creates the table, the application's first migration fails
with 'relation "outbox_messages" already exists', the application exits, and every restart
fails the same way because the table stays and no migration is recorded.

More than one poller per outbox (--leader, env FC_OUTBOX_LEADER, default true): the pollers of
one outbox (the outbox_messages table the connection's search_path resolves to) elect one leader
through a lock held in the source database itself, so an application may run any number of
tasks, each with a poller. Only the leader claims and sends; the others stand by on one connection
each and take over within seconds when the leader goes. The leader holds a transaction-level
advisory lock (classid 1717784386) in a transaction it keeps open and confirms every lease/5; it
stops sending one lease (--leader-lease-ms, FC_OUTBOX_LEADER_LEASE_MS, default 5000) after its
last confirmation, the server ends a holder silent for two leases, and a new leader waits 1.2
leases before it sends. Rows a previous leader left claimed are put back HTTP timeout + 5 s (35 s)
after the takeover; until then their message groups wait, so a group is never sent out of order.
The election works directly and through session- or transaction-mode poolers (PgBouncer, RDS
Proxy); a statement-mode pooler refuses it and the poller exits. --leader=false: no election (and
no lock connection), for a deployment that guarantees one poller some other way.

Health (--health-addr, FC_OUTBOX_HEALTH_ADDR, default 127.0.0.1:9180; off disables it): the poller
serves GET /health there, and 'fcdev outbox health' asks it (exit 0 healthy, 1 not), for a
container health check. Healthy: the leader's poll loop completed a cycle and a claim succeeded
within --stall-timeout-ms (FC_OUTBOX_STALL_TIMEOUT_MS, default 60000; claims timing out on a locked
table do not count) and the platform token could be obtained within it
(FC_OUTBOX_HEALTH_REQUIRE_TOKEN); a standby, or a poller waiting for outbox_messages, reached the
database within it. A leader whose poll loop stalls for the stall timeout gives the leadership up
and the command exits with status 1, as it does on any condition it cannot recover from.

The poller reads Postgres only; there is no other backend for this command.`,
		// Load a dotenv file (default ./.env) before resolving flags, so the
		// FC_OUTBOX_* vars can live in the app's .env instead of being exported.
		// Existing env vars always win; this also runs for `create-table`.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			envFile, _ := c.Flags().GetString("env-file")
			loadDotEnv(envFile)
			return nil
		},
		Args: cobra.NoArgs, // `fcdev outbox poll` / `init` do not exist: fail loudly rather than run the poller
		RunE: runOutbox,
	}
	cmd.PersistentFlags().String("env-file", ".env", "load environment from this dotenv file (does not override existing env)")
	cmd.Flags().String("source-db-url", envStrDefault("FC_OUTBOX_SOURCE_DB_URL", ""), "external app's Postgres URL (required)")
	cmd.Flags().String("target-url", envStrDefault("FC_OUTBOX_PLATFORM_URL", "http://localhost:8080"), "FlowCatalyst platform URL")
	cmd.Flags().String("auth-token", envStrDefault("FC_OUTBOX_PLATFORM_AUTH_TOKEN", ""), "static bearer token for the platform (used when client-id/secret are not set)")
	cmd.Flags().String("client-id", "", "OAuth client_credentials client id (env FC_OUTBOX_CLIENT_ID, falls back to FLOWCATALYST_CLIENT_ID)")
	cmd.Flags().String("client-secret", "", "OAuth client_credentials client secret (env FC_OUTBOX_CLIENT_SECRET, falls back to FLOWCATALYST_CLIENT_SECRET)")
	cmd.Flags().String("token-url", "", "OAuth token endpoint (env FC_OUTBOX_TOKEN_URL, default <target-url>/oauth/token)")
	cmd.Flags().String("scope", "", "optional requested scope to narrow the minted token (env FC_OUTBOX_SCOPE)")
	cmd.Flags().Int("batch-size", envIntDefault("FC_OUTBOX_BATCH_SIZE", 0), "rows per poll (0 = library default)")
	cmd.Flags().Int("max-in-flight", envIntDefault("FC_OUTBOX_MAX_IN_FLIGHT", 0), "outstanding HTTP requests cap (0 = library default)")
	cmd.Flags().Int("poll-interval-ms", envIntDefault("FC_OUTBOX_POLL_INTERVAL_MS", 0), "sleep between empty polls in ms (0 = library default)")
	cmd.Flags().Int("max-request-bytes", envIntDefault("FC_OUTBOX_MAX_REQUEST_BYTES", 0),
		"largest request body sent to the platform, in bytes; a batch is cut at --batch-size rows or this many bytes, and a larger row goes alone (env FC_OUTBOX_MAX_REQUEST_BYTES; 0 = library default, 8 MiB)")
	cmd.Flags().Bool("init-schema", envBoolDefault("FC_OUTBOX_INIT_SCHEMA", true),
		"create outbox_messages and its indexes at start if missing (env FC_OUTBOX_INIT_SCHEMA); false: run no DDL and wait for the application's migrations to create the table")
	cmd.Flags().Bool("leader", envBoolDefault("FC_OUTBOX_LEADER", true),
		"elect one dispatching poller per outbox through a lock in the source database (env FC_OUTBOX_LEADER); false: no election, for a deployment that guarantees one poller")
	cmd.Flags().Int("leader-lease-ms", envIntDefault("FC_OUTBOX_LEADER_LEASE_MS", 5000),
		"how long a leader keeps dispatching after it last confirmed its lock, in ms (env FC_OUTBOX_LEADER_LEASE_MS); takeover after a leader's death takes 1.4x to 3.4x this")
	cmd.Flags().String("health-addr", envStrDefault("FC_OUTBOX_HEALTH_ADDR", defaultOutboxHealthAddr),
		"serve GET /health here for 'fcdev outbox health' (env FC_OUTBOX_HEALTH_ADDR); off disables it")
	cmd.Flags().Int("stall-timeout-ms", envIntDefault("FC_OUTBOX_STALL_TIMEOUT_MS", 60000),
		"a poll loop, claim, database contact or token failing this long is unhealthy, and a leader whose poll loop stalls this long gives up and exits (env FC_OUTBOX_STALL_TIMEOUT_MS)")
	cmd.Flags().Bool("health-require-token", envBoolDefault("FC_OUTBOX_HEALTH_REQUIRE_TOKEN", true),
		"the leader is unhealthy when it cannot obtain a platform token for the stall timeout (env FC_OUTBOX_HEALTH_REQUIRE_TOKEN)")
	cmd.AddCommand(newOutboxCreateTableCmd())
	cmd.AddCommand(newOutboxHealthCmd())
	return cmd
}

// outboxTableWait paces the wait for outbox_messages when --init-schema is
// false (zero: the repository's defaults). Tests shorten it.
var outboxTableWait outboxpg.WaitOptions

// outboxTakeoverMargin is added to the HTTP timeout for the takeover recovery delay.
const outboxTakeoverMargin = 5 * time.Second

func runOutbox(cmd *cobra.Command, _ []string) error {
	// Re-resolve from env (the dotenv file is loaded in PersistentPreRunE,
	// after the flag defaults were baked at command-build time). Precedence:
	// explicit flag > env (incl. .env) > default.
	sourceURL := resolveEnvFlag(cmd, "source-db-url", "FC_OUTBOX_SOURCE_DB_URL")
	targetURL := resolveEnvFlag(cmd, "target-url", "FC_OUTBOX_PLATFORM_URL")
	authToken := resolveEnvFlag(cmd, "auth-token", "FC_OUTBOX_PLATFORM_AUTH_TOKEN")
	clientID := resolveEnvFlagMulti(cmd, "client-id", "FC_OUTBOX_CLIENT_ID", "FLOWCATALYST_CLIENT_ID")
	clientSecret := resolveEnvFlagMulti(cmd, "client-secret", "FC_OUTBOX_CLIENT_SECRET", "FLOWCATALYST_CLIENT_SECRET")
	tokenURL := resolveEnvFlag(cmd, "token-url", "FC_OUTBOX_TOKEN_URL")
	scope := resolveEnvFlag(cmd, "scope", "FC_OUTBOX_SCOPE")
	batchSize := resolveEnvFlagInt(cmd, "batch-size", "FC_OUTBOX_BATCH_SIZE")
	maxInFlight := resolveEnvFlagInt(cmd, "max-in-flight", "FC_OUTBOX_MAX_IN_FLIGHT")
	pollInterval := resolveEnvFlagInt(cmd, "poll-interval-ms", "FC_OUTBOX_POLL_INTERVAL_MS")
	maxRequestBytes := resolveEnvFlagInt(cmd, "max-request-bytes", "FC_OUTBOX_MAX_REQUEST_BYTES")
	initSchema, err := resolveEnvFlagBool(cmd, "init-schema", "FC_OUTBOX_INIT_SCHEMA")
	if err != nil {
		return err
	}
	leader, err := resolveEnvFlagBool(cmd, "leader", "FC_OUTBOX_LEADER")
	if err != nil {
		return err
	}
	requireToken, err := resolveEnvFlagBool(cmd, "health-require-token", "FC_OUTBOX_HEALTH_REQUIRE_TOKEN")
	if err != nil {
		return err
	}
	leaseMS := resolveEnvFlagInt(cmd, "leader-lease-ms", "FC_OUTBOX_LEADER_LEASE_MS")
	stallMS := resolveEnvFlagInt(cmd, "stall-timeout-ms", "FC_OUTBOX_STALL_TIMEOUT_MS")
	healthAddr := resolveEnvFlag(cmd, "health-addr", "FC_OUTBOX_HEALTH_ADDR")

	if sourceURL == "" {
		return errors.New("--source-db-url (or FC_OUTBOX_SOURCE_DB_URL) is required")
	}
	if leaseMS < 100 {
		return fmt.Errorf("--leader-lease-ms (FC_OUTBOX_LEADER_LEASE_MS) = %d: want at least 100", leaseMS)
	}
	lease := time.Duration(leaseMS) * time.Millisecond
	stall := time.Duration(stallMS) * time.Millisecond
	if stall < 2*lease {
		return fmt.Errorf("--stall-timeout-ms (FC_OUTBOX_STALL_TIMEOUT_MS) = %d: want at least twice the leader lease (%d)", stallMS, 2*leaseMS)
	}

	rootCtx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Stop on SIGINT/SIGTERM from here on, so a poller still waiting for its
	// table stops promptly too.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		select {
		case <-stop:
			slog.Info("shutdown signal received")
			cancel()
		case <-rootCtx.Done():
		}
	}()

	health := newOutboxHealth(stall, requireToken)
	if !strings.EqualFold(healthAddr, "off") && healthAddr != "" {
		if err := serveOutboxHealth(rootCtx, healthAddr, health); err != nil {
			return err
		}
	}

	pool, err := database.NewPool(rootCtx, database.Config{URL: sourceURL})
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer pool.Close()

	repo := outboxpg.New(pool)
	if initSchema {
		if err := repo.InitSchema(rootCtx); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	} else {
		wait := outboxTableWait
		wait.Checked = health.contacted
		health.setPhase(phaseWaitingTable)
		if err := repo.WaitForTable(rootCtx, wait); err != nil {
			if rootCtx.Err() != nil {
				slog.Info("fcdev outbox stopped before outbox_messages was created")
				return nil
			}
			return err
		}
	}
	lock, err := repo.Lock(rootCtx)
	if err != nil {
		if rootCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("resolve outbox_messages: %w", err)
	}

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = targetURL
	pcfg.AuthToken = authToken
	pcfg.DBTimeout = 10 * time.Second

	// When a service-account client_id/secret is available, mint and auto-refresh
	// the platform token via client_credentials (preferred — survives long runs).
	// Otherwise fall back to the static auth-token.
	authMode := "none"
	if authToken != "" {
		authMode = "static-token"
	}
	if clientID != "" && clientSecret != "" {
		if tokenURL == "" {
			tokenURL = strings.TrimRight(targetURL, "/") + "/oauth/token"
		}
		pcfg.TokenSource = newClientCredentialsTokenSource(tokenURL, clientID, clientSecret, scope)
		authMode = "client_credentials"
	}

	if batchSize > 0 {
		pcfg.BatchSize = batchSize
	}
	if maxInFlight > 0 {
		pcfg.MaxInFlight = int64(maxInFlight)
	}
	if pollInterval > 0 {
		pcfg.PollInterval = time.Duration(pollInterval) * time.Millisecond
	}
	if maxRequestBytes > 0 {
		pcfg.MaxRequestBytes = maxRequestBytes
	}
	if leader {
		pcfg.TakeoverRecoveryDelay = pcfg.HTTPTimeout + outboxTakeoverMargin
	}
	var procRepo outbox.Repository = repo
	if wrap := testHooks(rootCtx).wrapRepo; wrap != nil {
		procRepo = wrap(repo)
	}
	proc := outbox.NewProcessor(pcfg, procRepo)

	var election *outboxpg.Leadership
	var electionDone chan error // nil (never ready) with leadership off
	if leader {
		electionDone = make(chan error, 1)
		election = outboxpg.NewLeadership(pool.Config().ConnConfig, lock, outboxpg.LeaderOptions{
			Lease:      lease,
			Alive:      proc.LastIteration,
			StallAfter: stall,
			OnRole: func(r outboxpg.Role) {
				if r == outboxpg.RoleStandby {
					pool.Reset() // a standby keeps the lock connection only
				}
			},
		})
		proc.IsLeader, proc.Term = election.IsLeader, election.Term
	} else {
		slog.Warn("outbox leadership is off: this poller dispatches without an election; run exactly one per outbox",
			"outbox", lock.String())
	}
	health.running(lock.String(), election, proc)
	if src := pcfg.TokenSource; src != nil {
		go probeToken(rootCtx, health, src, func() bool { return proc.IsLeader == nil || proc.IsLeader() })
	}

	slog.Info("fcdev outbox started", "source", redactDBURL(sourceURL), "outbox", lock.String(), "target", targetURL,
		"auth", authMode, "leadership", leader, "lease", lease.String(), "health", healthAddr)
	if election != nil {
		go func() { electionDone <- election.Run(rootCtx) }()
	}
	procDone := make(chan struct{})
	go func() {
		defer close(procDone)
		proc.Run(rootCtx)
	}()

	var fatal error
	electionOver := election == nil
	select {
	case <-rootCtx.Done(): // shutdown
	case fatal = <-electionDone: // the election cannot go on (or saw the shutdown first)
		electionOver = true
		cancel()
	}
	// The election gives the lock up once the processor has stopped claiming (it checks the
	// leadership before every send); a hung poll loop is not waited for long.
	select {
	case <-procDone:
	case <-time.After(5 * time.Second):
		slog.Error("outbox poll loop did not stop within 5s; exiting without it")
	}
	if !electionOver {
		fatal = <-electionDone // it gives the lock up and returns
	}
	if fatal != nil {
		slog.Error("fcdev outbox cannot carry on", "outbox", lock.String(), "err", fatal)
		return fatal
	}
	slog.Info("fcdev outbox stopped")
	return nil
}
