#!/usr/bin/env bash
#
# The ingestor fleet test (docs/ingestor-fleet-test.md): one isolated platform,
# three ingestor runtime processes, steady input, and faults in order (kill -9
# one runtime, the whole fleet down, a rolling restart, the platform frozen).
# It checks that every fed record became exactly one job, how long each
# ingestion went without making jobs during each fault, the unheld gauge, and
# that every ingestion ends held by exactly one live runtime.
#
#   scripts/ingestor-fleet-test.sh          build, run, report; exit 1 on a failed assertion
#   make ingestor-fleet-test                the same
#
# Isolation (nothing of a developer's own platform is touched):
#   - Its own Postgres cluster in the state dir (initdb, loopback TCP only, no
#     Unix socket), on FLEET_PG_PORT (default 25532). Never the embedded cluster
#     on 15432; the binaries are only read from fcdev's cache (or PATH).
#   - Its own fcdev process, built from this checkout into the state dir, with
#     its own ports (API 28080, metrics 29090), HOME, TMPDIR, XDG dirs and PID
#     file, started with a clean environment (env -i). fcdev's own ingestor
#     child is off (FC_INGESTOR_ENABLED=false): the test starts its runtimes.
#   - The runtimes (ports 28091-28093), the source feed (28094) and the
#     delivery receiver (28095) belong to the helper (tools/ingestor-fleet),
#     which starts the runtimes with a clean environment and stops them by PID.
#   - Every process is stopped by the PID it was started with, on any exit
#     (trap), after checking the PID is still that process. No pattern kills.
#
# Overridable (environment): FLEET_STATE_DIR, FLEET_PG_PORT, FLEET_API_PORT,
# FLEET_METRICS_PORT, FLEET_RUNTIME_PORT_BASE (three ports from it),
# FLEET_FEED_PORT, FLEET_RECEIVER_PORT, FLEET_PG_BIN, FLEET_JAVA, FLEET_JAR,
# FLEET_SKIP_BUILD=1, FLEET_KEEP_RUNNING=1 (leave the platform and Postgres up
# after the run, for inspection), and the test's own knobs, read by the helper:
# FLEET_LEASE_SECONDS, FLEET_POLL_SECONDS, FLEET_DB_INGESTIONS, FLEET_API_POLL,
# FLEET_RATE, FLEET_FREEZE_SECONDS, FLEET_FAULTS, FLEET_*_MARGIN_SECONDS (see
# docs/ingestor-fleet-test.md).
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STATE=${FLEET_STATE_DIR:-$REPO/tmp/ingestor-fleet}
PG_PORT=${FLEET_PG_PORT:-25532}
API_PORT=${FLEET_API_PORT:-28080}
METRICS_PORT=${FLEET_METRICS_PORT:-29090}
RT_BASE=${FLEET_RUNTIME_PORT_BASE:-28091}
FEED_PORT=${FLEET_FEED_PORT:-28094}
RECEIVER_PORT=${FLEET_RECEIVER_PORT:-28095}
LEASE=${FLEET_LEASE_SECONDS:-120}

BIN="$STATE/bin/fcdev"
HELPER="$STATE/bin/ingestor-fleet"
LOGS="$STATE/logs"
PLATFORM_PIDFILE="$STATE/fcdev.pid"
PG_DATA="$STATE/pg/data"
PLATFORM_DB=fleet_platform
SOURCE_DB=fleet_source
ADMIN_EMAIL=admin@fleet.test

die() { echo "ingestor-fleet-test: $*" >&2; exit 2; }
say() { echo "ingestor-fleet-test: $*" >&2; }
need() { command -v "$1" >/dev/null 2>&1 || die "needs $1 on PATH"; }
port_busy() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

# ── guards ──────────────────────────────────────────────────────────────────
case $(basename "$STATE") in *fleet*) ;; *) die "FLEET_STATE_DIR must name a directory with 'fleet' in its name (it is wiped at start): $STATE" ;; esac
[ "$STATE" != / ] || die "FLEET_STATE_DIR may not be /"
PORTS="$PG_PORT $API_PORT $METRICS_PORT $RT_BASE $((RT_BASE + 1)) $((RT_BASE + 2)) $FEED_PORT $RECEIVER_PORT"
for p in $PORTS; do
	# The developer's own fcdev and its children, and the embedded cluster.
	case $p in 8080 | 8090 | 8091 | 8095 | 8096 | 9090 | 15432) die "port $p is reserved for the developer's own fcdev; pick another" ;; esac
done
[ "$(tr ' ' '\n' <<<"$PORTS" | sort | uniq -d)" = "" ] || die "the ports must all differ: $PORTS"
[ "$LEASE" -ge 120 ] 2>/dev/null || die "FLEET_LEASE_SECONDS must be 120 or more (the platform refuses less)"

need curl; need lsof; need openssl; need go; need psql; need pg_isready

pg_bin() {
	if [ -n "${FLEET_PG_BIN:-}" ]; then echo "$FLEET_PG_BIN"; return; fi
	local cache
	case $(uname -s) in
	Darwin) cache="$HOME/Library/Caches" ;;
	*) cache="${XDG_CACHE_HOME:-$HOME/.cache}" ;;
	esac
	# fcdev's cached binaries, read only (the cluster they run is ours).
	if [ -x "$cache/flowcatalyst/embedded-pg/bin/bin/pg_ctl" ]; then
		echo "$cache/flowcatalyst/embedded-pg/bin/bin"
	elif command -v pg_ctl >/dev/null 2>&1; then
		dirname "$(command -v pg_ctl)"
	else
		die "no Postgres binaries found; set FLEET_PG_BIN"
	fi
}
PGBIN=$(pg_bin)
JAVA=${FLEET_JAVA:-$(command -v java || true)}
[ -n "$JAVA" ] && [ -x "$JAVA" ] || die "needs java (25) on PATH, or FLEET_JAVA"

# ── cleanup: by PID, only our own processes ─────────────────────────────────
# ours PID NEEDLE: the process is alive and its command line contains NEEDLE.
ours() { [ -n "$1" ] && ps -p "$1" -o command= 2>/dev/null | grep -qF "$2"; }

stop_runtimes() {
	local f p
	for f in "$STATE"/pids/*.pid; do
		[ -f "$f" ] || continue
		p=$(cat "$f" 2>/dev/null || true)
		if ours "$p" "$STATE/hosts"; then
			say "stopping leftover runtime pid $p"
			kill -KILL "$p" 2>/dev/null || true
		fi
		rm -f "$f"
	done
}

stop_platform() {
	local p
	p=$(cat "$PLATFORM_PIDFILE" 2>/dev/null || true)
	ours "$p" "$BIN" || { rm -f "$PLATFORM_PIDFILE"; return 0; }
	# A run cut off during the freeze leaves it stopped: continue it first.
	kill -CONT "$p" 2>/dev/null || true
	kill -TERM "$p" 2>/dev/null || true
	for _ in $(seq 1 30); do
		ps -p "$p" >/dev/null 2>&1 || { rm -f "$PLATFORM_PIDFILE"; return 0; }
		sleep 1
	done
	say "platform pid $p did not stop within 30s; killing it"
	kill -KILL "$p" 2>/dev/null || true
	rm -f "$PLATFORM_PIDFILE"
}

stop_pg() {
	[ -f "$PG_DATA/postmaster.pid" ] || return 0
	"$PGBIN/pg_ctl" -D "$PG_DATA" -m fast -w -t 60 stop >/dev/null 2>&1 || say "could not stop the test's Postgres ($PG_DATA); stop it with: $PGBIN/pg_ctl -D $PG_DATA stop"
}

# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
	local code=$?
	trap - EXIT INT TERM
	stop_runtimes
	if [ "${FLEET_KEEP_RUNNING:-}" = 1 ] && [ "$code" -ne 2 ]; then
		say "left running (FLEET_KEEP_RUNNING=1): platform pid $(cat "$PLATFORM_PIDFILE" 2>/dev/null), Postgres $PG_DATA on $PG_PORT"
		say "stop them with: kill -TERM \$(cat $PLATFORM_PIDFILE); $PGBIN/pg_ctl -D $PG_DATA stop"
	else
		stop_platform
		stop_pg
	fi
	exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# ── a fresh state dir (the binaries are kept) ───────────────────────────────
mkdir -p "$STATE"
stop_runtimes
stop_platform
stop_pg
rm -rf "${STATE:?}/pg" "${STATE:?}/logs" "${STATE:?}/home" "${STATE:?}/tmp" "${STATE:?}/pids" "${STATE:?}"/rt-* "${STATE:?}/report.txt" "${STATE:?}/report.json" "${STATE:?}/hosts"
mkdir -p "$LOGS" "$STATE/home" "$STATE/tmp" "$STATE/pids" "$STATE/bin"

for p in $PORTS; do port_busy "$p" && die "port $p is in use; set the FLEET_*_PORT variables"; done

# ── build ───────────────────────────────────────────────────────────────────
if [ "${FLEET_SKIP_BUILD:-}" = 1 ] && [ -x "$BIN" ] && [ -x "$HELPER" ]; then
	say "FLEET_SKIP_BUILD=1: using $BIN and $HELPER as they are"
else
	overlay=()
	if [ ! -d "$REPO/frontend/dist" ]; then
		# The console is not used; a stub page lets fcdev build without
		# touching frontend/dist (a later `make go-build` must not embed it).
		mkdir -p "$STATE/build"
		echo '<!doctype html><title>ingestor fleet test</title>' >"$STATE/build/index.html"
		printf '{"Replace":{"%s/frontend/dist/index.html":"%s/build/index.html"}}\n' "$REPO" "$STATE" >"$STATE/build/overlay.json"
		overlay=(-overlay "$STATE/build/overlay.json")
		say "frontend/dist is missing: building fcdev with a stub console page"
	fi
	say "building fcdev and the test helper into $STATE/bin"
	go -C "$REPO" build ${overlay[@]+"${overlay[@]}"} -o "$BIN" ./cmd/fcdev
	go -C "$REPO" build -o "$HELPER" ./tools/ingestor-fleet
fi

JAR=${FLEET_JAR:-}
if [ -z "$JAR" ]; then
	# shellcheck disable=SC2012
	JAR=$(ls -t "$REPO"/ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar 2>/dev/null | head -1 || true)
	if [ -z "$JAR" ]; then
		say "no runtime jar yet: packaging it (ingestor/mvnw -DskipTests package)"
		"$REPO/ingestor/mvnw" -f "$REPO/ingestor/pom.xml" -q -B -DskipTests package
		# shellcheck disable=SC2012
		JAR=$(ls -t "$REPO"/ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar | head -1)
	fi
fi
[ -f "$JAR" ] || die "no runtime jar at $JAR"
say "runtime jar: $JAR"

# ── Postgres: our own cluster ───────────────────────────────────────────────
say "initialising the test's Postgres cluster in $PG_DATA (port $PG_PORT)"
mkdir -p "$STATE/pg"
"$PGBIN/initdb" -D "$PG_DATA" -U postgres -A trust -E UTF8 >"$LOGS/initdb.log" 2>&1 || die "initdb failed; see $LOGS/initdb.log"
# TCP on loopback only; no Unix socket (nothing connects through one, and a
# long state path would overflow the socket path limit).
"$PGBIN/pg_ctl" -D "$PG_DATA" -l "$LOGS/pg.log" -w -t 60 \
	-o "-p $PG_PORT -k '' -c listen_addresses=127.0.0.1 -c max_connections=200" start >/dev/null ||
	die "the test's Postgres did not start; see $LOGS/pg.log"
pg_isready -q -h 127.0.0.1 -p "$PG_PORT" -U postgres -d postgres || die "the test's Postgres does not answer on $PG_PORT"
for db in "$PLATFORM_DB" "$SOURCE_DB"; do
	psql -h 127.0.0.1 -p "$PG_PORT" -U postgres -d postgres -v ON_ERROR_STOP=1 -Atq -c "CREATE DATABASE $db" >/dev/null
done

# ── the platform ────────────────────────────────────────────────────────────
ADMIN_PASSWORD="$(openssl rand -hex 16)Aa1!"
say "starting the platform on http://localhost:$API_PORT (log $LOGS/fcdev.log)"
(
	cd "$STATE"
	env -i \
		PATH="$PATH" HOME="$STATE/home" TMPDIR="$STATE/tmp" \
		XDG_DATA_HOME="$STATE/home/data" XDG_CACHE_HOME="$STATE/home/cache" XDG_CONFIG_HOME="$STATE/home/config" \
		FC_DATABASE_URL="postgresql://postgres@127.0.0.1:$PG_PORT/$PLATFORM_DB?sslmode=disable&pool_max_conns=16" \
		FC_EMBEDDED_DB=false \
		FC_EMBEDDED_DB_PATH="$STATE/embedded-pg" \
		FC_API_PORT="$API_PORT" \
		FC_METRICS_PORT="$METRICS_PORT" \
		FC_MCP_ENABLED=false \
		FC_FUNCTIONS_ENABLED=false \
		FC_FUNCTIONS_PUBLIC_PORT=0 \
		FC_OUTBOX_ENABLED=false \
		FC_OUTBOX_ADMIN_PORT=0 \
		FC_INGESTION_ENABLED=true \
		FC_INGESTION_LEASE_SECONDS="$LEASE" \
		FC_INGESTOR_ENABLED=false \
		FC_SCHEDULER_DB_MAX_CONNECTIONS=4 \
		FC_DEV_PID_FILE="$STATE/fcdev-self.pid" \
		FC_FN_CLI_CREDENTIALS_PATH="$STATE/home/fng-cli.json" \
		FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL="$ADMIN_EMAIL" \
		FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
		FLOWCATALYST_BOOTSTRAP_ADMIN_NAME="Fleet Admin" \
		nohup "$BIN" start >>"$LOGS/fcdev.log" 2>&1 </dev/null &
	echo $! >"$PLATFORM_PIDFILE"
)
PLATFORM_PID=$(cat "$PLATFORM_PIDFILE")
for i in $(seq 1 120); do
	curl -sf -o /dev/null "http://127.0.0.1:$API_PORT/health" && break
	ours "$PLATFORM_PID" "$BIN" || die "the platform exited during start-up; see $LOGS/fcdev.log"
	[ "$i" = 120 ] && die "no healthy answer from the platform after 120s; see $LOGS/fcdev.log"
	sleep 1
done
# The freeze stops this one process: it must be the whole platform.
[ -z "$(pgrep -P "$PLATFORM_PID" 2>/dev/null || true)" ] || say "warning: the platform (pid $PLATFORM_PID) has child processes; the freeze stops only the parent"
say "platform healthy (pid $PLATFORM_PID); listening on: $(lsof -nP -a -p "$PLATFORM_PID" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR>1{print $9}' | sort -u | tr '\n' ' ')"

# ── the test ────────────────────────────────────────────────────────────────
helper_env=(
	PATH="$PATH" HOME="$STATE/home" TMPDIR="$STATE/tmp"
	FLEET_ADMIN_EMAIL="$ADMIN_EMAIL" FLEET_ADMIN_PASSWORD="$ADMIN_PASSWORD"
)
# The test's own knobs pass through; nothing else from the calling shell does.
while IFS='=' read -r k _; do
	case $k in FLEET_*) helper_env+=("$k=${!k}") ;; esac
done < <(env)

# A machine that sleeps mid-run stretches every gap past its bound: keep a Mac
# awake (idle sleep) while the helper runs.
awake=()
command -v caffeinate >/dev/null 2>&1 && awake=(caffeinate -i)

set +e
${awake[@]+"${awake[@]}"} env -i "${helper_env[@]}" "$HELPER" \
	-state "$STATE" \
	-api "http://localhost:$API_PORT" \
	-metrics "http://127.0.0.1:$METRICS_PORT/metrics" \
	-pg "postgresql://postgres@127.0.0.1:$PG_PORT" \
	-platform-db "$PLATFORM_DB" -source-db "$SOURCE_DB" \
	-platform-pid "$PLATFORM_PID" \
	-java "$JAVA" -jar "$JAR" \
	-runtime-ports "$RT_BASE,$((RT_BASE + 1)),$((RT_BASE + 2))" \
	-feed-port "$FEED_PORT" -receiver-port "$RECEIVER_PORT" \
	-lease "${LEASE}s"
code=$?
set -e
case $code in
0) say "PASS (report: $STATE/report.txt; logs: $LOGS)" ;;
1) say "FAIL: an assertion failed (report: $STATE/report.txt; logs: $LOGS)" ;;
*) say "the test could not run to the end (exit $code); logs: $LOGS" ;;
esac
exit "$code"
