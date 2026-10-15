#!/usr/bin/env bash
#
# An isolated FlowCatalyst platform for end-to-end checks of applications built
# on it (first user: the Camel ingestor, ingest-runtime + ingest-control-plane).
# See docs/e2e-platform.md.
#
#   scripts/e2e-platform.sh up       build, start, seed; prints the env
#   scripts/e2e-platform.sh status   is it running, which ports, which database
#   scripts/e2e-platform.sh env      print the generated env file
#   scripts/e2e-platform.sh verify   health, OIDC login, token mint, dispatch round trip
#   scripts/e2e-platform.sh down     stop it (the database is kept)
#   scripts/e2e-platform.sh drop     stop it, drop its database and delete its state
#
# Isolation, and why it is a separate DATABASE rather than a schema:
#   - Its own database (default e2e_camel) in the developer's embedded Postgres
#     cluster when that is running on 15432, else in a Postgres cluster this
#     script starts itself (own data dir, own port). The cluster on 15432 is
#     never started, stopped or written outside that one database. A schema in
#     the shared `flowcatalyst` database does not work: the migration bootstrap
#     probes information_schema across every schema (internal/migrate), so it
#     would see the other instance's goose ledger and skip or misapply
#     migrations.
#   - Its own ports for every listener (API, metrics; MCP, function runner and
#     the outbox admin API are off), its own process, PID file, signing key,
#     encryption key and HOME, so nothing lands in the developer's
#     ~/Library/{Caches,Application Support} (fcdev writes MCP and fn-CLI
#     credentials there on first boot).
#   - Started with a clean environment (env -i), so FC_* variables in the
#     calling shell never reach it.
#
# Overridable (environment): E2E_STATE_DIR, E2E_API_PORT, E2E_METRICS_PORT,
# E2E_DB_NAME, E2E_DB_MODE (auto|shared|own), E2E_SHARED_PG_PORT,
# E2E_OWN_PG_PORT, E2E_PG_BIN, E2E_CONTROL_PLANE_URL, E2E_RECEIVER_PORT,
# E2E_SKIP_BUILD=1.
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STATE=${E2E_STATE_DIR:-$REPO/tmp/e2e-platform}
API_PORT=${E2E_API_PORT:-18080}
METRICS_PORT=${E2E_METRICS_PORT:-19090}
DB_NAME=${E2E_DB_NAME:-e2e_camel}
DB_MODE=${E2E_DB_MODE:-auto}
SHARED_PG_PORT=${E2E_SHARED_PG_PORT:-15432}
OWN_PG_PORT=${E2E_OWN_PG_PORT:-25432}
CP_BASE=${E2E_CONTROL_PLANE_URL:-http://localhost:8090}
RECEIVER_PORT=${E2E_RECEIVER_PORT:-18099}
FUNCTIONS=${E2E_FUNCTIONS:-}

BASE="http://localhost:$API_PORT"
BIN="$STATE/bin/fcdev"
PIDFILE="$STATE/fcdev.pid"
LOG="$STATE/logs/fcdev.log"
ENVFILE="$STATE/e2e.env"
SECRETS="$STATE/secrets.env" # generated once; never printed by `status`
JAR="$STATE/tmp/admin.jar"

ADMIN_EMAIL=admin@e2e-camel.test
OPERATOR_EMAIL=operator@acme.test
APP_CODE=ingestor
CLIENT_IDENT=acme
POOL_CODE=ingest-default

die() { echo "e2e-platform: $*" >&2; exit 1; }
say() { echo "e2e-platform: $*" >&2; }

# ── guards ──────────────────────────────────────────────────────────────────
[[ $DB_NAME =~ ^e2e_[a-z0-9_]+$ ]] || die "E2E_DB_NAME must match ^e2e_[a-z0-9_]+\$ (got $DB_NAME)"
case $API_PORT in 8080 | 9090 | 8095 | 8096 | 8090 | "$SHARED_PG_PORT") die "E2E_API_PORT $API_PORT is reserved for the developer's own fcdev" ;; esac
case $METRICS_PORT in 8080 | 9090 | 8095 | 8096 | 8090 | "$SHARED_PG_PORT") die "E2E_METRICS_PORT $METRICS_PORT is reserved for the developer's own fcdev" ;; esac
[ "$OWN_PG_PORT" != "$SHARED_PG_PORT" ] || die "E2E_OWN_PG_PORT must differ from the shared cluster's port"

need() { command -v "$1" >/dev/null 2>&1 || die "needs $1 on PATH"; }
port_busy() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

# ── Postgres ────────────────────────────────────────────────────────────────
# The embedded cluster's binaries, as fcdev caches them (only read here).
pg_bin() {
	if [ -n "${E2E_PG_BIN:-}" ]; then echo "$E2E_PG_BIN"; return; fi
	local cache
	case $(uname -s) in
	Darwin) cache="$HOME/Library/Caches" ;;
	*) cache="${XDG_CACHE_HOME:-$HOME/.cache}" ;;
	esac
	if [ -x "$cache/flowcatalyst/embedded-pg/bin/bin/pg_ctl" ]; then
		echo "$cache/flowcatalyst/embedded-pg/bin/bin"
	elif command -v pg_ctl >/dev/null 2>&1; then
		dirname "$(command -v pg_ctl)"
	else
		die "no Postgres binaries found for the own-cluster mode; set E2E_PG_BIN"
	fi
}

mode() { cat "$STATE/mode" 2>/dev/null || true; }

pg_port() { if [ "$(mode)" = own ]; then echo "$OWN_PG_PORT"; else echo "$SHARED_PG_PORT"; fi; }

# psql against the maintenance database `postgres` of the cluster in use. Only
# ever used for CREATE/DROP DATABASE of $DB_NAME and to read pg_database.
admin_psql() {
	if [ "$(mode)" = own ]; then
		psql -h 127.0.0.1 -p "$OWN_PG_PORT" -U postgres -d postgres -v ON_ERROR_STOP=1 -Atq "$@"
	else
		PGPASSWORD=postgres psql -h 127.0.0.1 -p "$SHARED_PG_PORT" -U postgres -d postgres -v ON_ERROR_STOP=1 -Atq "$@"
	fi
}

db_url() {
	if [ "$(mode)" = own ]; then
		echo "postgresql://postgres@127.0.0.1:$OWN_PG_PORT/$DB_NAME?sslmode=disable&pool_max_conns=8"
	else
		echo "postgresql://postgres:postgres@127.0.0.1:$SHARED_PG_PORT/$DB_NAME?sslmode=disable&pool_max_conns=8"
	fi
}

choose_mode() {
	local current
	current=$(mode)
	if [ -n "$current" ]; then
		[ "$DB_MODE" = auto ] || [ "$DB_MODE" = "$current" ] ||
			die "state was created in '$current' mode; run '$0 drop' before switching to '$DB_MODE'"
		if [ "$current" = shared ] && ! pg_isready -U postgres -d postgres -q -h 127.0.0.1 -p "$SHARED_PG_PORT"; then
			die "the shared embedded Postgres on $SHARED_PG_PORT is not running, and this script never starts it. Start your own fcdev, or '$0 drop' then 'E2E_DB_MODE=own $0 up'"
		fi
		return
	fi
	local chosen=$DB_MODE
	if [ "$chosen" = auto ]; then
		if pg_isready -U postgres -d postgres -q -h 127.0.0.1 -p "$SHARED_PG_PORT"; then chosen=shared; else chosen=own; fi
	fi
	case $chosen in
	shared) pg_isready -U postgres -d postgres -q -h 127.0.0.1 -p "$SHARED_PG_PORT" || die "no Postgres on $SHARED_PG_PORT (and it is never started from here)" ;;
	own) ;;
	*) die "E2E_DB_MODE must be auto, shared or own" ;;
	esac
	echo "$chosen" >"$STATE/mode"
	say "database mode: $chosen"
}

own_pg_up() {
	local bin data
	bin=$(pg_bin)
	data="$STATE/pg/data"
	if [ ! -f "$data/PG_VERSION" ]; then
		say "initialising own Postgres cluster in $data"
		mkdir -p "$STATE/pg"
		"$bin/initdb" -D "$data" -U postgres -A trust -E UTF8 >"$STATE/logs/initdb.log" 2>&1 ||
			die "initdb failed; see $STATE/logs/initdb.log"
	fi
	if pg_isready -U postgres -d postgres -q -h 127.0.0.1 -p "$OWN_PG_PORT"; then return; fi
	port_busy "$OWN_PG_PORT" && die "port $OWN_PG_PORT is taken by something else; set E2E_OWN_PG_PORT"
	# TCP on loopback only; no Unix socket (a long state path would overflow
	# the socket path limit, and nothing here connects through one).
	"$bin/pg_ctl" -D "$data" -l "$STATE/logs/pg.log" -w -t 60 \
		-o "-p $OWN_PG_PORT -k '' -c listen_addresses=127.0.0.1" start >/dev/null ||
		die "own Postgres did not start; see $STATE/logs/pg.log"
}

own_pg_down() {
	local data="$STATE/pg/data"
	[ -f "$data/postmaster.pid" ] || return 0
	"$(pg_bin)/pg_ctl" -D "$data" -m fast -w stop >/dev/null || true
}

ensure_database() {
	if [ "$(admin_psql -c "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")" != 1 ]; then
		say "creating database $DB_NAME on port $(pg_port)"
		admin_psql -c "CREATE DATABASE $DB_NAME"
	fi
}

# ── the platform process ────────────────────────────────────────────────────
pid() { cat "$PIDFILE" 2>/dev/null || true; }

# Our process only: the PID must be alive AND be this state dir's binary.
running() {
	local p
	p=$(pid)
	[ -n "$p" ] || return 1
	ps -p "$p" -o command= 2>/dev/null | grep -qF "$BIN"
}

build() {
	if [ "${E2E_SKIP_BUILD:-}" = 1 ] && [ -x "$BIN" ]; then return; fi
	say "building fcdev into $BIN"
	mkdir -p "$STATE/bin"
	(cd "$REPO" && go build -o "$BIN" ./cmd/fcdev)
}

secret() { LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$1"; }

ensure_secrets() {
	if [ ! -f "$SECRETS" ]; then
		umask 077
		{
			echo "ADMIN_PASSWORD='$(secret 20)Aa1!'"
			echo "OPERATOR_PASSWORD='$(secret 20)Aa1!'"
		} >"$SECRETS"
	fi
	# shellcheck disable=SC1090
	. "$SECRETS"
}

start_platform() {
	if running; then say "already running (pid $(pid))"; return; fi
	port_busy "$API_PORT" && die "port $API_PORT is in use; set E2E_API_PORT"
	port_busy "$METRICS_PORT" && die "port $METRICS_PORT is in use; set E2E_METRICS_PORT"
	mkdir -p "$STATE/home" "$STATE/tmp" "$STATE/logs" "$STATE/functions"
	say "starting the platform on $BASE (log $LOG)"
	(
		cd "$STATE"
		env -i \
			PATH="$PATH" HOME="$STATE/home" TMPDIR="$STATE/tmp" \
			XDG_DATA_HOME="$STATE/home/data" XDG_CACHE_HOME="$STATE/home/cache" XDG_CONFIG_HOME="$STATE/home/config" \
			FC_DATABASE_URL="$(db_url)" \
			FC_EMBEDDED_DB=false \
			FC_EMBEDDED_DB_PATH="$STATE/embedded-pg" \
			FC_API_PORT="$API_PORT" \
			FC_METRICS_PORT="$METRICS_PORT" \
			FC_MCP_ENABLED=false \
			FC_FUNCTIONS_ENABLED=$([ "$FUNCTIONS" = 1 ] && echo true || echo false) \
			FC_FUNCTIONS_PORT=$((API_PORT + 15)) \
			FC_FUNCTIONS_PUBLIC_PORT=0 \
			FC_FUNCTIONS_RUNNER_URL="http://127.0.0.1:$((API_PORT + 15))" \
			FC_FUNCTIONS_ARTIFACT_STORE="file://$STATE/functions" \
			FC_OUTBOX_ENABLED=false \
			FC_OUTBOX_ADMIN_PORT=0 \
			FC_SCHEDULER_DB_MAX_CONNECTIONS=4 \
			FC_DEV_PID_FILE="$STATE/fcdev-self.pid" \
			FC_FN_CLI_CREDENTIALS_PATH="$STATE/home/fng-cli.json" \
			FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL="$ADMIN_EMAIL" \
			FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
			FLOWCATALYST_BOOTSTRAP_ADMIN_NAME="E2E Admin" \
			nohup "$BIN" start >>"$LOG" 2>&1 </dev/null &
		echo $! >"$PIDFILE"
	)
	for _ in $(seq 1 120); do
		if curl -sf -o /dev/null "$BASE/health"; then say "healthy (pid $(pid))"; return; fi
		running || die "the platform exited during start-up; see $LOG"
		sleep 1
	done
	die "no healthy answer from $BASE/health after 120s; see $LOG"
}

stop_platform() {
	if ! running; then rm -f "$PIDFILE"; say "not running"; return; fi
	local p
	p=$(pid)
	say "stopping pid $p"
	kill -TERM "$p"
	for _ in $(seq 1 60); do
		ps -p "$p" >/dev/null 2>&1 || { rm -f "$PIDFILE"; say "stopped"; return; }
		sleep 1
	done
	die "pid $p did not stop within 60s (not killed; inspect it)"
}

# ── HTTP helpers (as the anchor admin's session) ────────────────────────────
login_admin() {
	local code
	code=$(curl -s -o /dev/null -w '%{http_code}' -c "$JAR" -X POST "$BASE/auth/login" \
		-H 'Content-Type: application/json' \
		-d "$(jq -nc --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')")
	[ "$code" = 200 ] || die "admin login answered $code"
}

# api METHOD PATH [BODY] → body on stdout; fails on a non-2xx (404 → exit 4).
api() {
	local method=$1 path=$2 body=${3:-} out code
	out=$(mktemp "$STATE/tmp/resp.XXXXXX")
	if [ -n "$body" ]; then
		code=$(curl -s -o "$out" -w '%{http_code}' -b "$JAR" -X "$method" "$BASE$path" \
			-H "Origin: $BASE" -H 'Content-Type: application/json' -d "$body")
	else
		code=$(curl -s -o "$out" -w '%{http_code}' -b "$JAR" -X "$method" "$BASE$path" -H "Origin: $BASE")
	fi
	cat "$out"
	rm -f "$out"
	case $code in
	2??) return 0 ;;
	404) return 4 ;;
	*) echo >&2; say "$method $path answered $code"; return 1 ;;
	esac
}

# ── seeding (idempotent) ────────────────────────────────────────────────────
seed() {
	login_admin
	local acme app pool sa sa_id sa_principal sa_secret login admin_id op_id resp

	# The CLIENT-tier client `acme`.
	if resp=$(api GET "/api/clients/by-identifier/$CLIENT_IDENT" 2>/dev/null); then
		acme=$(jq -r .id <<<"$resp")
	else
		acme=$(api POST /api/clients "$(jq -nc --arg i "$CLIENT_IDENT" '{name:"Acme",identifier:$i}')" | jq -r .id)
	fi

	# The application and its role (permissions travel inside roles).
	if resp=$(api GET "/api/applications/by-code/$APP_CODE" 2>/dev/null); then
		app=$(jq -r .id <<<"$resp")
	else
		app=$(api POST /api/applications "$(jq -nc --arg c "$APP_CODE" '{code:$c,name:"Ingestor",description:"Camel ingest runtime + control plane (e2e)"}')" | jq -r .id)
	fi
	api POST "/api/applications/$APP_CODE/roles/sync" \
		'{"roles":[{"name":"operator","displayName":"Ingestion operator","description":"Manages ingestions in the ingest control plane","permissions":["ingestor:admin:ingestion:manage"]}]}' >/dev/null
	api PUT "/api/clients/$acme/applications" "$(jq -nc --arg a "$app" '{enabledApplicationIds:[$a]}')" >/dev/null

	# A platform-wide dispatch pool for the jobs the runtime creates.
	pool=$(api GET /api/dispatch-pools | jq -r --arg c "$POOL_CODE" '.pools[] | select(.code==$c) | .id' | head -1)
	if [ -z "$pool" ]; then
		pool=$(api POST /api/dispatch-pools "$(jq -nc --arg c "$POOL_CODE" '{code:$c,name:"Ingest default",description:"Dispatch jobs created by the ingest runtime (e2e)",concurrency:10}')" | jq -r .id)
	fi

	# The runtime's credentials are the application's own service account,
	# provisioned the normal way (platform:application-service, which carries
	# dispatch-jobs-write; its webhook secret signs ingestor:* deliveries), then
	# widened to every application: the runtime creates jobs for whichever
	# application an ingestion targets.
	sa_secret=${RUNTIME_CLIENT_SECRET:-}
	sa_principal=$(api GET "/api/applications/by-code/$APP_CODE" | jq -r '.serviceAccountId // empty')
	if [ -z "$sa_principal" ]; then
		resp=$(api POST "/api/applications/$app/provision-service-account")
		sa_principal=$(jq -r .serviceAccount.principalId <<<"$resp")
		sa=$(jq -r .serviceAccount.oauthClient.clientId <<<"$resp")
		sa_secret=$(jq -r .serviceAccount.oauthClient.clientSecret <<<"$resp")
	else
		sa=$(api GET /api/oauth-clients | jq -r --arg p "$sa_principal" '.clients[] | select(.serviceAccountPrincipalId==$p) | .clientId' | head -1)
		if [ -z "$sa_secret" ] || [ "${RUNTIME_CLIENT_ID:-}" != "$sa" ]; then
			# The secret is shown once; with it lost, mint a new one.
			local row
			row=$(api GET "/api/oauth-clients/by-client-id/$sa" | jq -r .id)
			sa_secret=$(api POST "/api/oauth-clients/$row/regenerate-secret" '{"graceSeconds":0}' | jq -r .clientSecret)
		fi
	fi
	api PUT "/api/principals/$sa_principal/application-access" '{"allApplications":true,"applicationIds":[]}' >/dev/null
	sa_id=$(api GET "/api/principals/$sa_principal" | jq -r '.serviceAccountId // empty')

	# The control plane's login client: PUBLIC + PKCE, redirect to /auth/callback.
	login=$(api GET /api/oauth-clients | jq -r --arg a "$app" \
		'.clients[] | select((.applicationIds // []) | index($a)) | select(.grantTypes | index("authorization_code")) | .clientId' | head -1)
	if [ -z "$login" ]; then
		local cp127=${CP_BASE/localhost/127.0.0.1}
		resp=$(api POST "/api/applications/$app/provision-login-client" \
			"$(jq -nc --arg a "$CP_BASE/auth/callback" --arg b "$cp127/auth/callback" '{clientType:"PUBLIC",redirectUris:([$a,$b]|unique)}')")
		login=$(jq -r .loginClient.oauthClient.clientId <<<"$resp")
	fi

	# Users: the anchor admin also operates ingestions; one CLIENT-tier
	# operator of acme.
	admin_id=$(api GET "/api/principals?q=$ADMIN_EMAIL&type=USER" | jq -r --arg e "$ADMIN_EMAIL" '.principals[] | select(.email==$e) | .id' | head -1)
	[ -n "$admin_id" ] || die "bootstrap admin not found"
	api POST "/api/principals/$admin_id/roles" '{"role":"ingestor:operator"}' >/dev/null
	op_id=$(api GET "/api/principals?q=$OPERATOR_EMAIL&type=USER" | jq -r --arg e "$OPERATOR_EMAIL" '.principals[] | select(.email==$e) | .id' | head -1)
	if [ -z "$op_id" ]; then
		op_id=$(api POST /api/principals/users "$(jq -nc --arg e "$OPERATOR_EMAIL" --arg p "$OPERATOR_PASSWORD" --arg c "$acme" \
			'{email:$e,name:"Acme Operator",password:$p,scope:"CLIENT",clientId:$c,sendInvitation:false}')" | jq -r .id)
	fi
	api POST "/api/principals/$op_id/roles" '{"role":"ingestor:operator"}' >/dev/null

	# The router read its configuration at start-up, before acme existed; a
	# client's queues are only consumed once the router knows the client (it
	# re-reads every 300s on its own). Reload now.
	api POST /router/config/reload >/dev/null

	umask 077
	cat >"$ENVFILE" <<EOF
# FlowCatalyst e2e platform: generated by scripts/e2e-platform.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)
# State: $STATE   Database: $DB_NAME on 127.0.0.1:$(pg_port) ($(mode) mode)
FC_E2E_BASE_URL=$BASE
FC_E2E_DATABASE=$DB_NAME
FC_E2E_ADMIN_EMAIL=$ADMIN_EMAIL
FC_E2E_ADMIN_PASSWORD=$ADMIN_PASSWORD
FC_E2E_OPERATOR_EMAIL=$OPERATOR_EMAIL
FC_E2E_OPERATOR_PASSWORD=$OPERATOR_PASSWORD
FC_E2E_APPLICATION_ID=$app
FC_E2E_ACME_CLIENT_ID=$acme
FC_E2E_ACME_IDENTIFIER=$CLIENT_IDENT
FC_E2E_DISPATCH_POOL_ID=$pool
FC_E2E_DISPATCH_POOL_CODE=$POOL_CODE
FC_E2E_RUNTIME_SERVICE_ACCOUNT_ID=$sa_id
FC_E2E_RUNTIME_PRINCIPAL_ID=$sa_principal

# ingest-control-plane (framework AuthModule)
APP_ENV=local
AUTH_ISSUER=$BASE
AUTH_CLIENT_ID=$login
AUTH_PUBLIC_BASE_URL=$CP_BASE

# ingest-runtime (framework FlowCatalystModule)
FLOWCATALYST_BASE_URL=$BASE
FLOWCATALYST_APPLICATION=$APP_CODE
FLOWCATALYST_CLIENT_ID=$sa
FLOWCATALYST_CLIENT_SECRET=$sa_secret
EOF
	{
		echo "ADMIN_PASSWORD='$ADMIN_PASSWORD'"
		echo "OPERATOR_PASSWORD='$OPERATOR_PASSWORD'"
		echo "RUNTIME_CLIENT_ID='$sa'"
		echo "RUNTIME_CLIENT_SECRET='$sa_secret'"
	} >"$SECRETS"
	say "seeded; env written to $ENVFILE"
}

# ── verification ────────────────────────────────────────────────────────────
PASS=0
FAIL=0
check() { # desc, actual, expected
	if [ "$2" = "$3" ]; then
		printf 'PASS  %-58s [%s]\n' "$1" "$2"; PASS=$((PASS + 1))
	else
		printf 'FAIL  %-58s [got %s, want %s]\n' "$1" "$2" "$3"; FAIL=$((FAIL + 1))
	fi
}

jwt_claims() { python3 -c 'import sys,json,base64; p=sys.argv[1].split(".")[1]; p+="="*(-len(p)%4); print(json.dumps(json.loads(base64.urlsafe_b64decode(p))))' "$1"; }

# auth_code_login EMAIL PASSWORD → prints the id_token claims (JSON), or "".
auth_code_login() {
	local jar verifier challenge loc code tok
	jar=$(mktemp "$STATE/tmp/jar.XXXXXX")
	curl -s -o /dev/null -c "$jar" -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
		-d "$(jq -nc --arg e "$1" --arg p "$2" '{email:$e,password:$p}')"
	verifier=$(openssl rand -hex 32)
	challenge=$(printf %s "$verifier" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '=')
	loc=$(curl -s -o /dev/null -w '%{redirect_url}' -b "$jar" -G "$BASE/oauth/authorize" \
		--data-urlencode response_type=code --data-urlencode "client_id=$AUTH_CLIENT_ID" \
		--data-urlencode "redirect_uri=$AUTH_PUBLIC_BASE_URL/auth/callback" --data-urlencode 'scope=openid profile email' \
		--data-urlencode state=e2e-state --data-urlencode nonce=e2e-nonce \
		--data-urlencode "code_challenge=$challenge" --data-urlencode code_challenge_method=S256)
	rm -f "$jar"
	code=$(sed -n 's/.*[?&]code=\([^&]*\).*/\1/p' <<<"$loc")
	[ -n "$code" ] || { echo ""; return; }
	tok=$(curl -s -X POST "$BASE/oauth/token" --data-urlencode grant_type=authorization_code \
		--data-urlencode "code=$code" --data-urlencode "redirect_uri=$AUTH_PUBLIC_BASE_URL/auth/callback" \
		--data-urlencode "client_id=$AUTH_CLIENT_ID" --data-urlencode "code_verifier=$verifier" | jq -r '.id_token // empty')
	[ -n "$tok" ] && jwt_claims "$tok" || echo ""
}

verify() {
	need python3; need openssl
	[ -f "$ENVFILE" ] || die "not seeded yet; run '$0 up'"
	set -a
	# shellcheck disable=SC1090
	. "$ENVFILE"
	set +a

	check "GET /health" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/health")" 200
	check "discovery issuer" "$(curl -s "$BASE/.well-known/openid-configuration" | jq -r .issuer)" "$AUTH_ISSUER"
	check "admin password login" "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
		-d "$(jq -nc --arg e "$FC_E2E_ADMIN_EMAIL" --arg p "$FC_E2E_ADMIN_PASSWORD" '{email:$e,password:$p}')")" 200

	local claims
	claims=$(auth_code_login "$FC_E2E_ADMIN_EMAIL" "$FC_E2E_ADMIN_PASSWORD")
	check "admin: authorization code + PKCE → id_token" "$([ -n "$claims" ] && echo ok || echo none)" ok
	if [ -n "$claims" ]; then
		check "admin id_token iss" "$(jq -r .iss <<<"$claims")" "$AUTH_ISSUER"
		check "admin id_token aud" "$(jq -r 'if (.aud|type)=="array" then .aud[0] else .aud end' <<<"$claims")" "$AUTH_CLIENT_ID"
		check "admin id_token roles has ingestor:operator" "$(jq -r '(.roles // []) | index("ingestor:operator") != null' <<<"$claims")" true
		echo "      admin id_token: $(jq -c '{sub,tier:(.tier // .scope),clients,applications,roles}' <<<"$claims")"
	fi
	claims=$(auth_code_login "$FC_E2E_OPERATOR_EMAIL" "$FC_E2E_OPERATOR_PASSWORD")
	check "acme operator: authorization code + PKCE → id_token" "$([ -n "$claims" ] && echo ok || echo none)" ok
	if [ -n "$claims" ]; then
		check "acme operator id_token roles has ingestor:operator" "$(jq -r '(.roles // []) | index("ingestor:operator") != null' <<<"$claims")" true
		echo "      operator id_token: $(jq -c '{sub,tier:(.tier // .scope),clients,applications,roles}' <<<"$claims")"
	fi

	local token
	token=$(curl -s -u "$FLOWCATALYST_CLIENT_ID:$FLOWCATALYST_CLIENT_SECRET" -X POST "$BASE/oauth/token" \
		--data-urlencode grant_type=client_credentials | jq -r '.access_token // empty')
	check "runtime client_credentials token" "$([ -n "$token" ] && echo ok || echo none)" ok
	[ -n "$token" ] || { echo "---"; echo "PASS=$PASS FAIL=$FAIL"; return 1; }
	claims=$(jwt_claims "$token")
	check "runtime token_use" "$(jq -r .token_use <<<"$claims")" api
	check "runtime scope has dispatch-jobs-write" "$(jq -r '(.scope // "") | split(" ") | index("platform:messaging:batch:dispatch-jobs-write") != null' <<<"$claims")" true

	# Dispatch round trip: runtime SA creates a TASK job for acme in the pool;
	# the scheduler queues it, the router delivers it to a throwaway receiver.
	port_busy "$RECEIVER_PORT" && die "receiver port $RECEIVER_PORT is in use; set E2E_RECEIVER_PORT"
	local rlog="$STATE/tmp/receiver.log" rpid
	: >"$rlog"
	python3 -I -c '
import http.server, sys
log = open(sys.argv[2], "a")
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n).decode()
        signed = "yes" if self.headers.get("X-FlowCatalyst-Signature") else "no"
        log.write(self.path + " " + self.headers.get("X-Event-Type", "") + " signed=" + signed + " " + body + "\n"); log.flush()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers()
        self.wfile.write(b"{\"ack\":true}")
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
' "$RECEIVER_PORT" "$rlog" &
	rpid=$!
	sleep 1
	local marker body resp job status
	marker="e2e-$(date +%s)"
	body=$(jq -nc --arg u "http://127.0.0.1:$RECEIVER_PORT/hook" --arg c "$FC_E2E_ACME_CLIENT_ID" --arg p "$FC_E2E_DISPATCH_POOL_ID" --arg m "$marker" \
		'{items:[{kind:"TASK",code:"ingestor:e2e:check:ping",targetUrl:$u,payload:({marker:$m}|tojson),payloadContentType:"application/json",dataOnly:true,clientId:$c,dispatchPoolId:$p,messageGroup:"e2e-check",mode:"IMMEDIATE",correlationId:$m}]}')
	resp=$(curl -s -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -X POST "$BASE/api/dispatch-jobs/batch" -d "$body")
	check "POST /api/dispatch-jobs/batch as runtime" "$(jq -r '.results[0].status // .code // "?"' <<<"$resp")" SUCCESS
	job=$(jq -r '.results[0].id // empty' <<<"$resp")
	status=none
	if [ -n "$job" ]; then
		ADMIN_PASSWORD=$FC_E2E_ADMIN_PASSWORD
		login_admin
		for _ in $(seq 1 60); do
			status=$(api GET "/api/dispatch-jobs/$job" 2>/dev/null | jq -r '.status // "?"' || echo "?")
			[ "$status" = COMPLETED ] && break
			sleep 1
		done
	fi
	check "dispatch job $job reaches COMPLETED" "$status" COMPLETED
	check "receiver got the payload" "$(grep -c "$marker" "$rlog" || true)" 1
	check "delivery was signed" "$(grep "$marker" "$rlog" | grep -c ' signed=yes' || true)" 1
	kill "$rpid" 2>/dev/null || true
	wait "$rpid" 2>/dev/null || true

	echo "---"
	echo "PASS=$PASS FAIL=$FAIL"
	[ "$FAIL" = 0 ]
}

# ── commands ────────────────────────────────────────────────────────────────
cmd_up() {
	need curl; need jq; need psql; need pg_isready; need go; need lsof
	mkdir -p "$STATE/logs" "$STATE/tmp"
	choose_mode
	[ "$(mode)" = own ] && own_pg_up
	ensure_database
	ensure_secrets
	build
	start_platform
	seed
	cmd_env
}

cmd_status() {
	echo "state:     $STATE"
	echo "mode:      $(mode)"
	echo "database:  $DB_NAME on 127.0.0.1:$(pg_port)"
	if running; then
		echo "platform:  running, pid $(pid)"
		echo "api:       $BASE  (health $(curl -s -o /dev/null -w '%{http_code}' "$BASE/health"))"
		echo "metrics:   http://localhost:$METRICS_PORT"
		echo "listeners: $(lsof -nP -a -p "$(pid)" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR>1{print $9}' | sort -u | tr '\n' ' ')"
	else
		echo "platform:  not running"
	fi
	[ "$(mode)" = own ] && echo "own pg:    $(pg_isready -U postgres -d postgres -h 127.0.0.1 -p "$OWN_PG_PORT" 2>&1)"
	return 0
}

cmd_env() {
	[ -f "$ENVFILE" ] || die "not seeded yet; run '$0 up'"
	cat "$ENVFILE"
}

cmd_down() {
	stop_platform
	[ "$(mode)" = own ] && own_pg_down
	return 0
}

cmd_drop() {
	stop_platform
	local m
	m=$(mode)
	if [ "$m" = shared ]; then
		if pg_isready -U postgres -d postgres -q -h 127.0.0.1 -p "$SHARED_PG_PORT"; then
			say "dropping database $DB_NAME on port $SHARED_PG_PORT"
			admin_psql -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE)"
		else
			die "the shared Postgres on $SHARED_PG_PORT is down; database $DB_NAME left in place (state kept)"
		fi
	elif [ "$m" = own ]; then
		own_pg_down
	fi
	[ -n "$STATE" ] && [ "$STATE" != / ] && rm -rf "$STATE"
	say "dropped"
}

case ${1:-} in
up) cmd_up ;;
down) cmd_down ;;
status) cmd_status ;;
env) cmd_env ;;
verify) verify ;;
drop) cmd_drop ;;
*) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
