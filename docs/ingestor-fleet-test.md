# The ingestor fleet test

`make ingestor-fleet-test` (or `scripts/ingestor-fleet-test.sh`) runs one isolated platform and three ingestor
runtime processes on this machine, feeds them steadily, and applies faults one after another. It proves what the
unit tests cannot: that a fleet of runtimes hands ingestions over when one dies, when the whole fleet is down, when
each is restarted in turn, and when the platform stops answering, without losing a record or making one twice.
Item D of `docs/ingestor-gap-fixes.md`; it also guards A1/A2 (shutdown and route stops), which show up here as the
rolling-restart gap.

No AWS, no Docker, nothing of a developer's own platform. About **13 minutes** end to end (a minute of build when
the binaries are not there yet). `make` itself exits 2 on any failure; the script's own codes are 0 (pass), 1 (an
assertion failed) and 2 (it could not run to the end).

## Running it

```sh
make ingestor-fleet-test                  # build, run, report
FLEET_FAULTS=roll,freeze make ingestor-fleet-test   # only some faults (kill, outage, roll, freeze; none)
FLEET_SKIP_BUILD=1 make ingestor-fleet-test         # reuse the binaries in the state dir
```

It needs Go, a JDK 25 (`java` on `PATH`, or `FLEET_JAVA`), `psql`/`pg_isready`, `curl`, `lsof`, `openssl`, and
Postgres server binaries (fcdev's cached ones under `~/Library/Caches/flowcatalyst/embedded-pg`, else `pg_ctl` on
`PATH`, or `FLEET_PG_BIN`). It uses the newest `ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar`
(packaging it first if there is none; `FLEET_JAR` names another), so **rebuild the jar** (`make ingestor-build`) to
test runtime changes. fcdev is built from this checkout into the state dir; when `frontend/dist` is missing it is
built with a stub console page through a Go build overlay, leaving `frontend/dist` alone.

**Keep the machine awake.** Every gap is measured on the wall clock (the jobs' `created_at`), so a laptop that
sleeps during a fault stretches that fault's gaps past its bound (a run where macOS idle-slept for 18 minutes
during the outage failed exactly that way). On macOS the script runs the helper under `caffeinate -i`, which
prevents idle sleep; closing the lid still sleeps.

The report is printed and written to `tmp/ingestor-fleet/report.txt` (and `report.json`); the logs of the
platform, Postgres and each runtime (`fleet-rt-N.log`, one file across restarts) are in `tmp/ingestor-fleet/logs/`.
`FLEET_KEEP_RUNNING=1` leaves the platform and Postgres up afterwards for inspection (the script prints how to stop
them); the next run stops and wipes them anyway.

## What it runs

| | |
|---|---|
| Postgres | its own cluster in `tmp/ingestor-fleet/pg` on `127.0.0.1:25532` (loopback TCP only, no Unix socket): database `fleet_platform` for the platform, `fleet_source` for the sources. Never the embedded cluster on 15432 |
| Platform | `fcdev start` on 28080 (metrics 29090), own HOME/TMPDIR/XDG dirs and PID file, clean environment (`env -i`), `FC_INGESTION_LEASE_SECONDS=120` (the shortest legal lease), MCP, functions, outbox and fcdev's own ingestor child off |
| Runtimes | `fleet-rt-1..3` on 28091-28093 (`127.0.0.1`), `INGEST_POLL_INTERVAL=PT10S`, functions off, `APP_ENV=local`, one service account (`platform:ingestion-runtime`, anchor tier, every application), clean environment, `INGEST_PARENT_PID` = the helper (a runtime ends itself if the helper dies) |
| Ingestions | application `fleet` (INTEGRATION), client `fleet`, pool `fleet-pool`: six `database-poll` (`db-1..6`, watermark on `id`, `every` 1 s) and one `api-poll` (`api-1`, cursor `after`, `every`/`idleDelay` 1 s); targets deliver to a local receiver on 28095 that answers 200 |
| Feeder | one row every 0.5 s into each source table (`src_1..6`, `api_feed`); the api-poll feed on 28094 serves `api_feed` past a cursor |

The sources live in the test's own Postgres, but the platform refuses a loopback address in a saved definition.
The definitions name `fleet-pg.test` and `fleet-feed.test` instead, and the runtimes' JVMs resolve those (and
`localhost`) to `127.0.0.1` through `-Djdk.net.hosts.file`; the runtime allows loopback under `APP_ENV=local`. The
database password is an `env:` secret the runtimes carry (Postgres trusts loopback).

Isolation: every port is checked free before start and refused if it is one of the developer fcdev's (8080, 8090,
8091, 8095, 8096, 9090, 15432). Every process is stopped by the PID it was started with, on any exit (an `EXIT`
trap; the platform is `SIGCONT`ed first in case a run was cut off during the freeze), after checking the PID is
still that process. The helper records each runtime's PID in `tmp/ingestor-fleet/pids/` so the script can stop a
leftover. Secrets (the admin password, the runtime's client secret) are generated per run, passed only through
environments, and never printed.

## The faults

After the fleet is steady (every runtime has polled, the leases are spread within the fair share, every ingestion
made a job in the last 5 s) and a 20 s baseline:

| # | Fault | What happens | Gap bound (defaults) |
|---|---|---|---|
| 1 | **kill**: `kill -9` the runtime holding the most leases | No goodbye; its leases run out and the survivors take them at their next poll. Then it is started again (same runtime id) and the fleet evens out | lease + poll + 30 s = 160 s |
| 2 | **outage**: `kill -9` all three, kept down until `fc_ingestion_unheld` counts every ingestion, then all started again | The only fault that can raise the unheld gauge (below) | outage length + poll + 60 s |
| 3 | **roll**: `SIGTERM` each runtime in turn, start it again, wait for its first accepted poll and for the fleet to even out | Shutdown stops the routes, gives the leases back and says goodbye, so the others take them at their next poll and the restarted one polls at once | poll + 20 s = 30 s |
| 4 | **freeze**: `SIGSTOP` the platform for 60 s, then `SIGCONT` | Runtimes cannot poll or make jobs; the leases outlast the freeze | freeze + poll + 30 s = 100 s |

Then 30 s of settling, the feed stops, and the test waits for every fed record to have its job.

**The unheld gauge.** `fc_ingestion_unheld` counts an ingestion only after it has had no unexpired lease for a lease
**and two minutes**, recounted every minute. A single `kill -9` cannot raise it: the survivors take the dead
runtime's leases one poll after they end. So the kill fault asserts it stays **0** (no false alarm during a normal
failover), and the outage fault is the one that keeps every runtime down long enough for it to count every
ingestion and asserts it then returns to 0 once the fleet is back. That takes a while: the last lease ends up to a
lease after the kill, the ingestion counts once it has had no lease for another lease and two minutes, and the
gauge is recounted once a minute, so 6 to 7 minutes of outage with the 120 s lease. Turn the outage off with
`FLEET_FAULTS=kill,roll,freeze` for a shorter run (about 7 minutes).

## The assertions

- **Exactly once:** every fed record became exactly one job: the jobs' payload ids (each job's payload is its
  row, or its feed item) against the source tables: no duplicates, none missing, none unknown.
- **Gap per fault:** for each ingestion, the longest time without a new job from its last job before the fault
  until the fleet was steady again stays within the fault's bound.
- **Unheld gauge:** 0 throughout the kill, the roll and the freeze; during the outage it rises to the number of
  ingestions and returns to 0 after the restart.
- **At the end:** every ingestion is held (unexpired, unreleased lease) by a runtime whose process is running and
  that the platform counts as live, and its jobs while settling were made by that runtime alone (each job carries a
  `runtimeId` metadata entry).
- A fault whose wait timed out (the fleet never became steady, a restarted runtime never polled) is a failure too.

## Reading the report

```
1. kill -9 fleet-rt-2 (held 3 of 7)
   from +42s for 2m11s; bound on the gap 160.0s (lease 2m0s + poll 10s + margin 30s)
   - every ingestion made jobs again 2m0s after the kill
   - fleet-rt-2 started again; its first poll was accepted 1s later
   - the fleet was even again 11s after the restart
   ingest     max gap  max latency   fed  result jobs made by, in turn
   db-1        120.6s       119.7s   263  ok     rt-1
   db-3          1.0s         1.0s   263  ok     rt-3
   ...
   fc_ingestion_unheld: max 0 over 65 samples (0 unanswered); expected 0
```

- **max gap**: the longest time the ingestion made no job in the window. An ingestion the fault did not touch shows
  about the source interval; a moved one shows how long the hand-over took.
- **max latency**: the longest a record fed during the window waited for its job (fed_at to the job's
  created_at). It tracks the gap; a latency well above the gap means a backlog.
- **jobs made by, in turn**: the runtimes that made the ingestion's jobs during the window, in order; `rt-1 > rt-2`
  is a hand-over. The platform fences jobs by lease, so two runtimes never make jobs for one ingestion at once.
- The notes say how long recovery took, when a restarted runtime's first poll was accepted, how long each
  `SIGTERM` stop took and whether the goodbye was recorded (`left_at` set): a missing goodbye makes the restarted
  runtime wait a lease for its id (`409 RUNTIME_ID_IN_USE`), and the roll gap shows it.
- **Records**: fed, jobs, duplicates, missing, unknown per ingestion over the whole run.
- **At the end**: who holds each ingestion and which runtimes made its jobs while settling.
- `RESULT: PASS`, or `RESULT: FAIL` with every failed assertion listed.

## Knobs

| Variable | Default | |
|---|---|---|
| `FLEET_LEASE_SECONDS` | 120 | the platform's lease (at least 120) |
| `FLEET_POLL_SECONDS` | 10 | the runtimes' poll interval |
| `FLEET_SOURCE_EVERY_SECONDS` | 1 | each source's own poll |
| `FLEET_DB_INGESTIONS` | 6 | database-poll ingestions |
| `FLEET_API_POLL` | 1 | `0` leaves out the api-poll ingestion |
| `FLEET_RATE` | 2 | records a second fed to each source |
| `FLEET_FREEZE_SECONDS` | 60 | the platform freeze |
| `FLEET_FAULTS` | `kill,outage,roll,freeze` | which faults, in this order; `none` for a smoke run |
| `FLEET_KILL_MARGIN_SECONDS`, `FLEET_OUTAGE_MARGIN_SECONDS`, `FLEET_ROLL_MARGIN_SECONDS`, `FLEET_FREEZE_MARGIN_SECONDS` | 30, 60, 20, 30 | the margins in the gap bounds |
| `FLEET_FRESH_SECONDS` | 5 | "making jobs": a job within this long |
| `FLEET_STATE_DIR`, `FLEET_PG_PORT`, `FLEET_API_PORT`, `FLEET_METRICS_PORT`, `FLEET_RUNTIME_PORT_BASE`, `FLEET_FEED_PORT`, `FLEET_RECEIVER_PORT` | `tmp/ingestor-fleet`, 25532, 28080, 29090, 28091, 28094, 28095 | where it runs |
| `FLEET_JAVA`, `FLEET_JAR`, `FLEET_PG_BIN`, `FLEET_SKIP_BUILD`, `FLEET_KEEP_RUNNING` | | what it runs with |

## Files

- `scripts/ingestor-fleet-test.sh`: guards, build, the Postgres cluster, the platform, cleanup by PID.
- `tools/ingestor-fleet/`: the helper (seeding, feeder, api-poll feed, receiver, runtime processes, faults,
  gauge sampling, report).
