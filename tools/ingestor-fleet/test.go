package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// unheldThreshold is how long past its lease's end an ingestion goes unheld
// before the platform counts it (ingestion.UnheldGrace), and unheldInterval how
// often the gauge is recounted (ingestion.UnheldInterval).
const (
	unheldGrace    = 2 * time.Minute
	unheldInterval = time.Minute
)

// window is one fault, from when it was applied until the fleet was steady
// again, and what the jobs made in it are held to.
type window struct {
	Name           string        `json:"name"`
	Title          string        `json:"title"`
	Start          time.Time     `json:"start"`
	End            time.Time     `json:"end"`
	Bound          time.Duration `json:"boundNanos"`
	BoundWhy       string        `json:"boundWhy"`
	RecoveredAfter time.Duration `json:"recoveredAfterNanos"`
	Notes          []string      `json:"notes"`
	GaugeExpect    string        `json:"gaugeExpect"` // "zero" or "rise"
	GaugeMax       float64       `json:"gaugeMax"`
	GaugeRose      *time.Time    `json:"gaugeRose,omitempty"`
	GaugeCleared   *time.Time    `json:"gaugeCleared,omitempty"`
	GaugeSamples   int           `json:"gaugeSamples"`
	GaugeMissed    int           `json:"gaugeMissed"`
	Failures       []string      `json:"failures"`
}

func (w *window) note(format string, a ...any) {
	s := fmt.Sprintf(format, a...)
	w.Notes = append(w.Notes, s)
	log.Printf("  %s", s)
}

func (w *window) fail(format string, a ...any) {
	s := fmt.Sprintf(format, a...)
	w.Failures = append(w.Failures, s)
	log.Printf("  FAIL %s", s)
}

type test struct {
	cfg      config
	plat     *pgxpool.Pool
	src      *pgxpool.Pool
	api      *platform
	specs    []ingestionSpec
	fleet    *fleet
	obs      *observer
	feed     *feeder
	windows  []*window
	started  time.Time
	feedStop time.Time
	frozen   bool
	cancel   context.CancelFunc
	final    finalCheck
	feedErrs map[string]int
}

func newTest(ctx context.Context, cfg config) (*test, error) {
	plat, err := pgxpool.New(ctx, cfg.pgURL+"/"+cfg.platformDB+"?sslmode=disable&pool_max_conns=4")
	if err != nil {
		return nil, err
	}
	src, err := pgxpool.New(ctx, cfg.pgURL+"/"+cfg.sourceDB+"?sslmode=disable&pool_max_conns=16")
	if err != nil {
		plat.Close()
		return nil, err
	}
	api, err := newPlatform(cfg.apiURL)
	if err != nil {
		return nil, err
	}
	return &test{cfg: cfg, plat: plat, src: src, api: api}, nil
}

// close stops everything this program started: the platform is continued if
// it was left frozen, then the runtimes are stopped by PID.
func (t *test) close() {
	if t.frozen {
		_ = syscall.Kill(t.cfg.platformPID, syscall.SIGCONT)
	}
	if t.fleet != nil {
		t.fleet.stopAll()
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.plat.Close()
	t.src.Close()
}

func (t *test) ids() []string {
	out := make([]string, len(t.specs))
	for i, s := range t.specs {
		out[i] = s.id
	}
	return out
}

func (t *test) run(parent context.Context) (bool, error) {
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	cfg := t.cfg
	t.started = wallNow()
	log.SetFlags(log.Ltime)
	log.Printf("lease %s, runtime poll %s, source every %s, %.1f records/s per source, faults %v",
		cfg.lease, cfg.poll, cfg.sourceEvery, cfg.rate, cfg.faults)

	var specs []ingestionSpec
	for i := 1; i <= cfg.dbIngestions; i++ {
		specs = append(specs, ingestionSpec{code: fmt.Sprintf("db-%d", i), table: fmt.Sprintf("src_%d", i), kind: "database-poll"})
	}
	if cfg.apiPoll {
		specs = append(specs, ingestionSpec{code: "api-1", table: "api_feed", kind: "api-poll"})
	}
	if err := createSourceTables(ctx, t.src, specs); err != nil {
		return false, err
	}
	if err := writeHostsFile(cfg.state); err != nil {
		return false, err
	}
	if _, err := serve(ctx, cfg.feedPort, feedHandler(t.src, "api_feed")); err != nil {
		return false, fmt.Errorf("the api-poll feed: %w", err)
	}
	if _, err := serve(ctx, cfg.receiverPort, receiverHandler()); err != nil {
		return false, fmt.Errorf("the receiver: %w", err)
	}

	if err := waitFor(ctx, 30*time.Second, "the admin login", func() (bool, string, error) {
		err := t.api.login(ctx, cfg.adminEmail, cfg.adminPassword)
		return err == nil, fmt.Sprint(err), nil
	}); err != nil {
		return false, err
	}
	creds, seeded, err := t.api.seed(ctx, cfg, specs)
	if err != nil {
		return false, fmt.Errorf("seeding the platform: %w", err)
	}
	t.specs = seeded
	log.Printf("seeded %d ingestions (%d database-poll, api-poll %v)", len(seeded), cfg.dbIngestions, cfg.apiPoll)

	if t.fleet, err = newFleet(cfg, creds); err != nil {
		return false, err
	}
	t.obs = startObserver(ctx, cfg.metricsURL)
	t.feed = startFeeder(ctx, t.src, t.specs, cfg.rate)

	log.Printf("starting %d runtimes", len(t.fleet.procs))
	for _, r := range t.fleet.procs {
		if err := t.fleet.start(r.n); err != nil {
			return false, err
		}
	}
	if err := waitFor(ctx, 3*time.Minute, "the fleet to start and every ingestion to make jobs", t.steady); err != nil {
		return false, err
	}
	log.Printf("steady after %s; baseline for 20s", time.Since(t.started).Round(time.Second))
	if err := sleep(ctx, 20*time.Second); err != nil {
		return false, err
	}

	for _, f := range cfg.faults {
		var w *window
		var err error
		switch f {
		case "kill":
			w, err = t.faultKill(ctx)
		case "outage":
			w, err = t.faultOutage(ctx)
		case "roll":
			w, err = t.faultRoll(ctx)
		case "freeze":
			w, err = t.faultFreeze(ctx)
		}
		if w != nil {
			t.windows = append(t.windows, w)
		}
		if err != nil {
			return false, err
		}
		if len(w.Failures) > 0 && !t.recovered(ctx) {
			log.Printf("the fleet did not recover from %s; skipping the remaining faults", w.Name)
			break
		}
	}

	log.Printf("settling for 30s, then stopping the feed")
	settleFrom := wallNow()
	if err := sleep(ctx, 30*time.Second); err != nil {
		return false, err
	}
	t.final = t.checkHoldings(ctx, settleFrom)
	t.feedStop = wallNow()
	t.feedErrs = t.feed.stop()
	if err := waitFor(ctx, 3*time.Minute, "every fed record to have its job", func() (bool, string, error) {
		d, err := t.collect(ctx)
		if err != nil {
			return false, "", err
		}
		missing := 0
		for _, s := range t.specs {
			missing += d.stats(s.code).missing
		}
		return missing == 0, fmt.Sprintf("%d record(s) without a job", missing), nil
	}); err != nil {
		log.Printf("drain: %v", err)
	}
	// A record read again would make its duplicate now, if it were going to.
	if err := sleep(ctx, 2*cfg.poll); err != nil {
		return false, err
	}
	d, err := t.collect(ctx)
	if err != nil {
		return false, err
	}
	return t.report(d)
}

// steady holds when every running runtime has polled since it started, the
// leases are spread over the running runtimes within the fair share, and every
// ingestion made a job within freshWithin.
func (t *test) steady() (bool, string, error) {
	ctx := context.Background()
	running := t.fleet.running()
	if len(running) == 0 {
		return false, "no runtime running", nil
	}
	for _, r := range running {
		ok, why, err := t.polledSince(ctx, r)
		if err != nil || !ok {
			return false, why, err
		}
	}
	ok, why, err := t.balanced(ctx, running)
	if err != nil || !ok {
		return false, why, err
	}
	return t.fresh(ctx)
}

func (t *test) polledSince(ctx context.Context, r *runtimeProc) (bool, string, error) {
	row, found, err := runtimeState(ctx, t.plat, r.id)
	if err != nil {
		return false, "", err
	}
	if !found || !row.lastSeen.After(r.start) {
		return false, r.id + " has not polled since it started", nil
	}
	return true, "", nil
}

func (t *test) balanced(ctx context.Context, running []*runtimeProc) (bool, string, error) {
	held, err := leases(ctx, t.plat, t.ids())
	if err != nil {
		return false, "", err
	}
	up := map[string]bool{}
	for _, r := range running {
		up[r.id] = true
	}
	counts := map[string]int{}
	for _, s := range t.specs {
		rt := held[s.id]
		switch {
		case rt == "":
			return false, s.code + " is not held", nil
		case !up[rt]:
			return false, s.code + " is held by " + rt + ", which is not running", nil
		}
		counts[rt]++
	}
	n, k := len(t.specs), len(running)
	share := (n + k - 1) / k
	for _, r := range running {
		c := counts[r.id]
		if c > share {
			return false, fmt.Sprintf("%s holds %d, over the share %d", r.id, c, share), nil
		}
		// Every runtime must hold one when the others' shares cannot cover
		// them all.
		if c == 0 && n > (k-1)*share {
			return false, r.id + " holds none yet", nil
		}
	}
	return true, "", nil
}

func (t *test) fresh(ctx context.Context) (bool, string, error) {
	ages, err := latestJobs(ctx, t.plat)
	if err != nil {
		return false, "", err
	}
	for _, s := range t.specs {
		age, ok := ages[s.code]
		if !ok {
			return false, s.code + " has made no job", nil
		}
		if age > t.cfg.freshWithin {
			return false, fmt.Sprintf("%s made its last job %s ago", s.code, age.Round(time.Second)), nil
		}
	}
	return true, "", nil
}

// recovered reports whether the fleet is steady again within a minute and a
// half; used after a fault that failed, to decide whether to go on.
func (t *test) recovered(ctx context.Context) bool {
	return waitFor(ctx, 90*time.Second, "recovery", t.steady) == nil
}

func (t *test) holdings(ctx context.Context) (map[string]int, error) {
	held, err := leases(ctx, t.plat, t.ids())
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, rt := range held {
		if rt != "" {
			counts[rt]++
		}
	}
	return counts, nil
}

// ── the faults ──────────────────────────────────────────────────────────────

// faultKill kills the runtime holding the most leases with SIGKILL: no
// goodbye, so its leases run out and the survivors take them at their next
// poll after that. Then it starts it again and waits for the fleet to even out.
func (t *test) faultKill(ctx context.Context) (*window, error) {
	cfg := t.cfg
	counts, err := t.holdings(ctx)
	if err != nil {
		return nil, err
	}
	victim := t.fleet.procs[0]
	for _, r := range t.fleet.procs {
		if counts[r.id] > counts[victim.id] {
			victim = r
		}
	}
	w := &window{
		Name: "kill", GaugeExpect: "zero",
		Title:    fmt.Sprintf("kill -9 %s (held %d of %d)", victim.id, counts[victim.id], len(t.specs)),
		Bound:    cfg.lease + cfg.poll + cfg.killMargin,
		BoundWhy: fmt.Sprintf("lease %s + poll %s + margin %s", cfg.lease, cfg.poll, cfg.killMargin),
	}
	log.Printf("fault: %s", w.Title)
	w.Start = wallNow()
	if err := t.fleet.kill(victim.n); err != nil {
		return w, err
	}
	if err := sleep(ctx, 2*cfg.freshWithin); err != nil {
		return w, err
	}
	if err := waitFor(ctx, cfg.lease+cfg.poll+cfg.killMargin+time.Minute, "every ingestion making jobs again", func() (bool, string, error) {
		return t.fresh(ctx)
	}); err != nil {
		w.fail("%v", err)
	}
	w.RecoveredAfter = time.Since(w.Start)
	w.note("every ingestion made jobs again %s after the kill", w.RecoveredAfter.Round(time.Second))

	restart := wallNow()
	if err := t.fleet.start(victim.n); err != nil {
		return w, err
	}
	if err := waitFor(ctx, cfg.lease+time.Minute, victim.id+" to poll again", func() (bool, string, error) {
		return t.polledSince(ctx, victim)
	}); err != nil {
		w.fail("%v", err)
	}
	w.note("%s started again; its first poll was accepted %s later", victim.id, time.Since(restart).Round(time.Second))
	if err := waitFor(ctx, 3*cfg.poll+90*time.Second, "the fleet to even out", t.steady); err != nil {
		w.fail("%v", err)
	}
	w.End = wallNow()
	w.note("the fleet was even again %s after the restart", w.End.Sub(restart).Round(time.Second))
	return w, nil
}

// faultOutage kills every runtime with SIGKILL and keeps them down until the
// platform's unheld gauge counts the ingestions, then starts them all again
// and waits for the gauge to return to 0. A single kill never raises the gauge
// (the survivors take the leases one poll after they end, long before the
// unheld threshold of a lease and two minutes); this is the fault that does.
func (t *test) faultOutage(ctx context.Context) (*window, error) {
	cfg := t.cfg
	n := float64(len(t.specs))
	w := &window{
		Name: "outage", GaugeExpect: "rise",
		Title: fmt.Sprintf("the whole fleet down (kill -9 all %d) until fc_ingestion_unheld counts the ingestions", len(t.fleet.procs)),
	}
	log.Printf("fault: %s", w.Title)
	w.Start = wallNow()
	for _, r := range t.fleet.running() {
		if err := t.fleet.kill(r.n); err != nil {
			return w, err
		}
	}
	// The last lease ends at most one lease after the kill; the ingestion is
	// unheld once it has had no lease for another lease and unheldGrace, and
	// counted at the next recount (a minute at most): up to two leases, two
	// minutes and a minute after the kill.
	riseWithin := 2*cfg.lease + unheldGrace + unheldInterval + time.Minute
	if err := waitFor(ctx, riseWithin, "fc_ingestion_unheld to count every ingestion", func() (bool, string, error) {
		s, ok := t.obs.latest()
		if !ok {
			return false, "no gauge sample", nil
		}
		if s.value > 0 && w.GaugeRose == nil {
			at := s.at
			w.GaugeRose = &at
			w.note("fc_ingestion_unheld rose to %.0f %s after the kill", s.value, at.Sub(w.Start).Round(time.Second))
		}
		return s.value >= n, fmt.Sprintf("gauge %.0f", s.value), nil
	}); err != nil {
		w.fail("%v", err)
	}
	restart := wallNow()
	w.note("restarting the fleet %s after the kill", restart.Sub(w.Start).Round(time.Second))
	w.Bound = restart.Sub(w.Start) + cfg.poll + cfg.outageMargin
	w.BoundWhy = fmt.Sprintf("outage %s + poll %s + margin %s", restart.Sub(w.Start).Round(time.Second), cfg.poll, cfg.outageMargin)
	for _, r := range t.fleet.procs {
		if err := t.fleet.start(r.n); err != nil {
			return w, err
		}
	}
	if err := waitFor(ctx, 3*cfg.poll+2*time.Minute, "the fleet to start and even out", t.steady); err != nil {
		w.fail("%v", err)
	}
	w.RecoveredAfter = time.Since(restart)
	w.note("every ingestion made jobs again %s after the restart", w.RecoveredAfter.Round(time.Second))
	if err := waitFor(ctx, 2*unheldInterval+30*time.Second, "fc_ingestion_unheld to return to 0", func() (bool, string, error) {
		s, ok := t.obs.latest()
		if !ok || !s.at.After(restart) {
			return false, "no gauge sample since the restart", nil
		}
		if s.value == 0 {
			at := s.at
			w.GaugeCleared = &at
			return true, "", nil
		}
		return false, fmt.Sprintf("gauge %.0f", s.value), nil
	}); err != nil {
		w.fail("%v", err)
	} else {
		w.note("fc_ingestion_unheld was 0 again %s after the restart", w.GaugeCleared.Sub(restart).Round(time.Second))
	}
	w.End = wallNow()
	return w, nil
}

// faultRoll stops each runtime in turn with SIGTERM (it stops its routes,
// gives their leases back and says goodbye), starts it again, waits for its
// poll and for the fleet to even out, then goes on to the next.
func (t *test) faultRoll(ctx context.Context) (*window, error) {
	cfg := t.cfg
	w := &window{
		Name: "roll", GaugeExpect: "zero",
		Title:    fmt.Sprintf("rolling restart (SIGTERM, start, wait for its poll) of all %d", len(t.fleet.procs)),
		Bound:    cfg.poll + cfg.rollMargin,
		BoundWhy: fmt.Sprintf("poll %s + margin %s", cfg.poll, cfg.rollMargin),
	}
	log.Printf("fault: %s", w.Title)
	w.Start = wallNow()
	for _, r := range t.fleet.procs {
		if !r.running() {
			if err := t.fleet.start(r.n); err != nil {
				return w, err
			}
			continue
		}
		term := wallNow()
		took, byItself, err := t.fleet.terminate(r.n, 45*time.Second)
		if err != nil {
			return w, err
		}
		row, _, err := runtimeState(ctx, t.plat, r.id)
		if err != nil {
			return w, err
		}
		goodbye := row.leftAt != nil && row.leftAt.After(term.Add(-time.Second))
		how := "it ended by itself"
		if !byItself {
			how = "it did not end within 45s and was killed"
		}
		said := "goodbye recorded (left_at set)"
		if !goodbye {
			said = "NO goodbye recorded"
		}
		w.note("%s: SIGTERM, %s after %s; %s", r.id, how, took.Round(100*time.Millisecond), said)
		start := wallNow()
		if err := t.fleet.start(r.n); err != nil {
			return w, err
		}
		if err := waitFor(ctx, cfg.lease+30*time.Second, r.id+" to poll again", func() (bool, string, error) {
			return t.polledSince(ctx, r)
		}); err != nil {
			w.fail("%v", err)
		}
		polled := time.Since(start)
		if err := waitFor(ctx, 3*cfg.poll+time.Minute, "the fleet to even out", t.steady); err != nil {
			w.fail("%v", err)
		}
		w.note("%s: first poll accepted %s after its start; fleet even %s after it", r.id, polled.Round(time.Second), time.Since(start).Round(time.Second))
	}
	w.End = wallNow()
	w.RecoveredAfter = w.End.Sub(w.Start)
	return w, nil
}

// faultFreeze stops the platform process (SIGSTOP) for the freeze length and
// continues it (SIGCONT). The runtimes cannot poll, report or make jobs
// meanwhile; their leases outlast the freeze.
func (t *test) faultFreeze(ctx context.Context) (*window, error) {
	cfg := t.cfg
	w := &window{
		Name: "freeze", GaugeExpect: "zero",
		Title:    fmt.Sprintf("the platform frozen (SIGSTOP pid %d) for %s, then SIGCONT", cfg.platformPID, cfg.freeze),
		Bound:    cfg.freeze + cfg.poll + cfg.freezeMargin,
		BoundWhy: fmt.Sprintf("freeze %s + poll %s + margin %s", cfg.freeze, cfg.poll, cfg.freezeMargin),
	}
	log.Printf("fault: %s", w.Title)
	w.Start = wallNow()
	if err := syscall.Kill(cfg.platformPID, syscall.SIGSTOP); err != nil {
		return w, fmt.Errorf("SIGSTOP the platform: %w", err)
	}
	t.frozen = true
	slept := sleep(ctx, cfg.freeze)
	if err := syscall.Kill(cfg.platformPID, syscall.SIGCONT); err != nil {
		return w, fmt.Errorf("SIGCONT the platform: %w", err)
	}
	t.frozen = false
	if slept != nil {
		return w, slept
	}
	cont := wallNow()
	health := &http.Client{Timeout: 3 * time.Second}
	if err := waitFor(ctx, time.Minute, "the platform to answer /health", func() (bool, string, error) {
		resp, err := health.Get(cfg.apiURL + "/health")
		if err != nil {
			return false, err.Error(), nil
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK, resp.Status, nil
	}); err != nil {
		w.fail("%v", err)
	}
	if err := sleep(ctx, 2*cfg.freshWithin); err != nil {
		return w, err
	}
	if err := waitFor(ctx, cfg.poll+cfg.freezeMargin+2*time.Minute, "every ingestion making jobs again", t.steady); err != nil {
		w.fail("%v", err)
	}
	w.End = wallNow()
	w.RecoveredAfter = w.End.Sub(cont)
	w.note("steady again %s after SIGCONT", w.RecoveredAfter.Round(time.Second))
	return w, nil
}

// ── at the end ──────────────────────────────────────────────────────────────

// finalCheck is who holds what once the faults are over.
type finalCheck struct {
	Holders  map[string]string   `json:"holders"`  // ingestion code → runtime id
	JobsBy   map[string][]string `json:"jobsBy"`   // ingestion code → runtimes that made its jobs while settling
	Failures []string            `json:"failures"` //
}

// checkHoldings checks every ingestion is held by exactly one runtime that is
// running (a process this test started and has not stopped) and that the
// platform counts as live, and that only that runtime made its jobs since
// from.
func (t *test) checkHoldings(ctx context.Context, from time.Time) finalCheck {
	fc := finalCheck{Holders: map[string]string{}, JobsBy: map[string][]string{}}
	held, err := leases(ctx, t.plat, t.ids())
	if err != nil {
		fc.Failures = append(fc.Failures, "reading the leases: "+err.Error())
		return fc
	}
	live, err := liveRuntimes(ctx, t.plat, t.cfg.lease)
	if err != nil {
		fc.Failures = append(fc.Failures, "reading the runtimes: "+err.Error())
		return fc
	}
	up := map[string]bool{}
	for _, r := range t.fleet.running() {
		up[r.id] = true
	}
	by, err := jobRuntimesSince(ctx, t.plat, from)
	if err != nil {
		fc.Failures = append(fc.Failures, "reading the jobs: "+err.Error())
		return fc
	}
	for _, s := range t.specs {
		rt := held[s.id]
		fc.Holders[s.code] = rt
		fc.JobsBy[s.code] = by[s.code]
		switch {
		case rt == "":
			fc.Failures = append(fc.Failures, s.code+": no runtime holds an unexpired lease")
		case !up[rt]:
			fc.Failures = append(fc.Failures, s.code+": held by "+rt+", whose process is not running")
		case !live[rt]:
			fc.Failures = append(fc.Failures, s.code+": held by "+rt+", which the platform does not count as live")
		}
		if rt != "" && !slices.Equal(by[s.code], []string{rt}) {
			fc.Failures = append(fc.Failures, fmt.Sprintf("%s: jobs while settling were made by %v, not only by its holder %s", s.code, by[s.code], rt))
		}
	}
	return fc
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
