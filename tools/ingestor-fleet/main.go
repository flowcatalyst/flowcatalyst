// ingestor-fleet is the helper of the ingestor fleet test
// (scripts/ingestor-fleet-test.sh, docs/ingestor-fleet-test.md). The script
// starts an isolated Postgres cluster and platform; this program seeds the
// platform, feeds the sources, runs three ingestor runtime processes, applies
// the faults in order, watches the platform, and writes the report.
//
// It is not meant to be run on its own: the script passes the addresses of
// what it started. Exit 0 when every assertion held, 1 when one failed, 2 when
// the test could not run to the end.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// config is everything the run needs: the addresses from the script (flags)
// and the test's knobs (FLEET_* environment variables).
type config struct {
	state         string
	apiURL        string
	metricsURL    string
	pgURL         string
	platformDB    string
	sourceDB      string
	platformPID   int
	java          string
	jar           string
	runtimePorts  []int
	feedPort      int
	receiverPort  int
	adminEmail    string
	adminPassword string

	lease        time.Duration // FC_INGESTION_LEASE_SECONDS the platform runs with
	poll         time.Duration // the runtimes' INGEST_POLL_INTERVAL
	sourceEvery  time.Duration // each source's own poll ("every")
	dbIngestions int
	apiPoll      bool
	rate         float64 // records a second fed to each source
	freeze       time.Duration
	faults       []string

	killMargin   time.Duration
	outageMargin time.Duration
	rollMargin   time.Duration
	freezeMargin time.Duration
	freshWithin  time.Duration // an ingestion is flowing when it made a job this recently
}

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ingestor-fleet:", err)
	}
	os.Exit(code)
}

func run() (int, error) {
	cfg, err := loadConfig()
	if err != nil {
		return 2, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	t, err := newTest(ctx, cfg)
	if err != nil {
		return 2, err
	}
	defer t.close()
	failed, err := t.run(ctx)
	if err != nil {
		return 2, err
	}
	if failed {
		return 1, nil
	}
	return 0, nil
}

func loadConfig() (config, error) {
	var cfg config
	var ports string
	flag.StringVar(&cfg.state, "state", "", "the state dir")
	flag.StringVar(&cfg.apiURL, "api", "", "the platform's base URL")
	flag.StringVar(&cfg.metricsURL, "metrics", "", "the platform's metrics URL")
	flag.StringVar(&cfg.pgURL, "pg", "", "postgresql://user@host:port of the test's cluster")
	flag.StringVar(&cfg.platformDB, "platform-db", "", "the platform's database")
	flag.StringVar(&cfg.sourceDB, "source-db", "", "the sources' database")
	flag.IntVar(&cfg.platformPID, "platform-pid", 0, "the platform's process id (frozen by SIGSTOP)")
	flag.StringVar(&cfg.java, "java", "", "the java binary")
	flag.StringVar(&cfg.jar, "jar", "", "the runtime jar")
	flag.StringVar(&ports, "runtime-ports", "", "three runtime HTTP ports, comma separated")
	flag.IntVar(&cfg.feedPort, "feed-port", 0, "the api-poll feed's port")
	flag.IntVar(&cfg.receiverPort, "receiver-port", 0, "the delivery receiver's port")
	flag.DurationVar(&cfg.lease, "lease", 120*time.Second, "the platform's ingestion lease")
	flag.Parse()

	for p := range strings.SplitSeq(ports, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return cfg, fmt.Errorf("-runtime-ports: %w", err)
		}
		cfg.runtimePorts = append(cfg.runtimePorts, n)
	}
	if len(cfg.runtimePorts) != 3 {
		return cfg, errors.New("-runtime-ports must name three ports")
	}
	if cfg.state == "" || cfg.apiURL == "" || cfg.pgURL == "" || cfg.platformPID <= 0 || cfg.jar == "" || cfg.java == "" {
		return cfg, errors.New("missing flags: run it through scripts/ingestor-fleet-test.sh")
	}
	cfg.state = filepath.Clean(cfg.state)
	cfg.adminEmail = os.Getenv("FLEET_ADMIN_EMAIL")
	cfg.adminPassword = os.Getenv("FLEET_ADMIN_PASSWORD")
	if cfg.adminEmail == "" || cfg.adminPassword == "" {
		return cfg, errors.New("FLEET_ADMIN_EMAIL and FLEET_ADMIN_PASSWORD must be set")
	}

	var err error
	seconds := func(key string, def int) time.Duration {
		if err != nil {
			return 0
		}
		v := os.Getenv(key)
		if v == "" {
			return time.Duration(def) * time.Second
		}
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			err = fmt.Errorf("%s must be a whole number of seconds, was %q", key, v)
		}
		return time.Duration(n) * time.Second
	}
	cfg.poll = seconds("FLEET_POLL_SECONDS", 10)
	cfg.sourceEvery = seconds("FLEET_SOURCE_EVERY_SECONDS", 1)
	cfg.freeze = seconds("FLEET_FREEZE_SECONDS", 60)
	cfg.killMargin = seconds("FLEET_KILL_MARGIN_SECONDS", 30)
	cfg.outageMargin = seconds("FLEET_OUTAGE_MARGIN_SECONDS", 60)
	cfg.rollMargin = seconds("FLEET_ROLL_MARGIN_SECONDS", 20)
	cfg.freezeMargin = seconds("FLEET_FREEZE_MARGIN_SECONDS", 30)
	cfg.freshWithin = seconds("FLEET_FRESH_SECONDS", 5)
	if err != nil {
		return cfg, err
	}
	if cfg.poll < time.Second || cfg.sourceEvery < time.Second {
		return cfg, errors.New("FLEET_POLL_SECONDS and FLEET_SOURCE_EVERY_SECONDS must be at least 1")
	}
	cfg.dbIngestions = 6
	if v := os.Getenv("FLEET_DB_INGESTIONS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 50 {
			return cfg, fmt.Errorf("FLEET_DB_INGESTIONS must be 1 to 50, was %q", v)
		}
		cfg.dbIngestions = n
	}
	cfg.apiPoll = os.Getenv("FLEET_API_POLL") != "0"
	cfg.rate = 2
	if v := os.Getenv("FLEET_RATE"); v != "" {
		f, e := strconv.ParseFloat(v, 64)
		if e != nil || f <= 0 || f > 100 {
			return cfg, fmt.Errorf("FLEET_RATE must be a number of records a second per source, above 0 and at most 100, was %q", v)
		}
		cfg.rate = f
	}
	cfg.faults = []string{"kill", "outage", "roll", "freeze"}
	if v := os.Getenv("FLEET_FAULTS"); v != "" {
		cfg.faults = nil
		for f := range strings.SplitSeq(v, ",") {
			f = strings.TrimSpace(f)
			switch f {
			case "kill", "outage", "roll", "freeze":
				cfg.faults = append(cfg.faults, f)
			case "", "none":
			default:
				return cfg, fmt.Errorf("FLEET_FAULTS: unknown fault %q (kill, outage, roll, freeze)", f)
			}
		}
	}
	return cfg, nil
}

// wallNow is the time without Go's monotonic reading: every time the test
// keeps is compared with the database's (job created_at, lease and runtime
// rows), which are wall-clock times, so its own must be too. (On macOS the
// monotonic clock stops while the machine sleeps; the wall clock does not.)
func wallNow() time.Time { return time.Now().Round(0) }
