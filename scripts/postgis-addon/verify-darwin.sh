#!/usr/bin/env bash
# Verifies a darwin PostGIS add-on on the machine's own architecture.
#
#   verify-darwin.sh <addon.tar.gz> <zonky .txz> <pg prefix> [<build root>]
#
# 1. Linkage: the archive holds regular files and directories only, under the
#    paths fcdev accepts; the module and every bundled library refer only to
#    each other (@loader_path), /usr/lib and /System; no LC_RPATH; signatures
#    valid; built for macOS <= MACOS_MIN; no path of the build root inside.
# 2. Runtime: zonky's vanilla archive with the add-on unpacked over it, as fcdev
#    does; zonky's initdb and pg_ctl run with an empty environment (env -i) in
#    a sandbox that denies every read of Homebrew's directories (/opt/homebrew,
#    /usr/local), so a library or proj.db that leaked from Homebrew cannot be
#    opened; TCP on a free port (never 15432 or 8080) and no unix socket (the
#    path would pass the 103-byte limit); then proof.sql with the private
#    build's psql (<pg prefix>; zonky ships none).
# 3. Negative control: the same tree without postgis/share/proj, where EPSG
#    codes must not resolve, so proj.db is found through the add-on.
#
# bash 3.2 (macOS /bin/bash) compatible.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

fail() { echo "VERIFY FAILED: $*" >&2; exit 1; }
[ $# -ge 3 ] || fail "usage: verify-darwin.sh <addon.tar.gz> <zonky .txz> <pg prefix> [<build root>]"
TGZ=$1 ZONKY=$2 PGP=$3 ROOTPATH=${4:-}
PSQL=$PGP/bin/psql
T=$(mktemp -d "${TMPDIR:-/tmp}/postgis-verify.XXXXXX")
T=$(cd "$T" && pwd -P)
ROOT=$T/pgroot NP=$T/noproj
case "$(uname -m)" in arm64) ARCH=arm64 ;; x86_64) ARCH=x86_64 ;; *) fail "machine $(uname -m)" ;; esac

# ── 1. linkage ──────────────────────────────────────────────────────────────
echo "== archive entries"
tar -tvzf "$TGZ" | awk '{print substr($1,1,1)}' | sort | uniq -c
tar -tvzf "$TGZ" | awk '{t=substr($1,1,1)} t != "-" && t != "d" {bad=1; print} END {exit bad}' \
  || fail "an entry is neither a file nor a directory"
while read -r f; do
  case "$f" in
    /*|*../*|..*) fail "unsafe path $f" ;;
    postgis-addon.json|lib/|lib/postgresql/|lib/postgresql/*|share/|share/postgresql/|share/postgresql/extension/*|postgis/*) ;;
    lib|lib/postgresql|share|share/postgresql|share/postgresql/extension|postgis) ;;
    *) fail "path outside what fcdev installs: $f" ;;
  esac
done < <(tar -tzf "$TGZ")
[ "$(tar -tzf "$TGZ" | tail -1)" = postgis-addon.json ] || fail "postgis-addon.json is not the last entry"
X=$T/addon; mkdir -p "$X"; tar -xzf "$TGZ" -C "$X"
echo "== postgis-addon.json"; cat "$X/postgis-addon.json"
grep -q "\"postgis\": \"$POSTGIS_VERSION\"" "$X/postgis-addon.json" || fail "manifest postgis version"
grep -q "\"pgMajor\": \"$PG_MAJOR\"" "$X/postgis-addon.json" || fail "manifest pg major"
grep -q '"goos": "darwin"' "$X/postgis-addon.json" || fail "manifest goos"

MOD=$X/lib/postgresql/postgis-3.dylib
[ -f "$MOD" ] || fail "no lib/postgresql/postgis-3.dylib"
bad=0
newer() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)" = "$1" ] && [ "$1" != "$2" ]; }
for f in "$MOD" "$X"/postgis/lib/*.dylib; do
  echo "--- ${f#"$X"/} ($(stat -f %z "$f") bytes, $(lipo -archs "$f"))"
  otool -L "$f" | tail -n +2
  if otool -L "$f" | tail -n +2 | awk '{print $1}' | grep -Ev '^(@loader_path/|/usr/lib/|/System/)'; then echo "  ^ refers outside the add-on and the system"; bad=1; fi
  if otool -l "$f" | grep -A2 LC_RPATH | grep -q path; then echo "  has an LC_RPATH"; bad=1; fi
  case "$(lipo -archs "$f")" in *"$ARCH"*) ;; *) echo "  not built for $ARCH"; bad=1 ;; esac
  minos=$(otool -l "$f" | awk '/LC_BUILD_VERSION/{b=1} b && /minos/{print $2; exit}')
  [ -n "$minos" ] || minos=$(otool -l "$f" | awk '/LC_VERSION_MIN_MACOSX/{b=1} b && /version/{print $2; exit}')
  echo "  minos $minos"
  if [ -z "$minos" ] || newer "$minos" "$MACOS_MIN"; then echo "  needs macOS $minos, newer than $MACOS_MIN"; bad=1; fi
  codesign -v "$f" || { echo "  signature invalid"; bad=1; }
  if [ -n "$ROOTPATH" ] && grep -q -a -F "$ROOTPATH" "$f"; then
    echo "  holds the build root $ROOTPATH:"; grep -a -o -F "$ROOTPATH" "$f" | head -3; bad=1
  fi
done
echo "== licences"; (cd "$X/postgis/share/licenses" && ls)
[ "$bad" = 0 ] || fail "linkage check"
echo "linkage check passed: only @loader_path, /usr/lib and /System"

# ── 2. runtime ──────────────────────────────────────────────────────────────
SANDBOX='(version 1)(allow default)(deny file-read* (subpath "/opt/homebrew") (subpath "/usr/local"))'
run() { env -i HOME="$HOME" PATH=/usr/bin:/bin LANG=C sandbox-exec -p "$SANDBOX" "$@"; }
free_port() { python3 -I -c 'import socket
while True:
    s = socket.socket(); s.bind(("127.0.0.1", 0)); p = s.getsockname()[1]; s.close()
    if p not in (15432, 8080): print(p); break'; }
start() {  # start <root> <port>
  run "$1/bin/initdb" -D "$1/data" -U postgres -A trust -E UTF8 --locale=C >/dev/null
  run "$1/bin/pg_ctl" -D "$1/data" -l "$1/server.log" -w \
    -o "-p $2 -c listen_addresses=127.0.0.1 -c unix_socket_directories=''" start >/dev/null
}
stop() { run "$1/bin/pg_ctl" -D "$1/data" -m fast -w stop >/dev/null 2>&1 || true; }
sql() { "$PSQL" -h 127.0.0.1 -p "$1" -U postgres -d postgres -X "${@:2}"; }

echo
echo "== the sandbox denies Homebrew:"
run /bin/ls /opt/homebrew /usr/local 2>&1 | head -2 || true
echo "== environment the server gets:"; run /usr/bin/env | sed 's/^/   /'

mkdir -p "$ROOT"
tar -xJf "$ZONKY" -C "$ROOT"
tar -xzf "$TGZ" -C "$ROOT"
# shellcheck disable=SC2012
echo "== tree: $(cd "$ROOT" && ls | tr '\n' ' '); postgres: $(lipo -archs "$ROOT/bin/postgres")"
PORT=$(free_port)
start "$ROOT" "$PORT"
trap 'stop "$ROOT"; stop "$NP"; rm -rf "$T"' EXIT
echo "== server on 127.0.0.1:$PORT, postmaster pid $(head -1 "$ROOT/data/postmaster.pid")"

cat > "$T/inspect.sh" <<'EOF'
echo "   libraries and proj.db the backend has mapped or open (lsof):"
lsof -p "$BACKEND" -Fn | sed -n 's/^n//p' | grep -E '\.dylib$|proj\.db' | sort -u | sed 's/^/     /'
echo "   paths under /opt/homebrew or /usr/local open or mapped: $(lsof -p "$BACKEND" -Fn | grep -c -E '^n/(opt/homebrew|usr/local)/')"
EOF
INSPECT=$T/inspect.sh sql "$PORT" -v ON_ERROR_STOP=1 -e \
  -v postgis="$POSTGIS_VERSION" -v geos="$(source_field geos 2)" -v proj="$(source_field proj 2)" \
  -v projdb="$ROOT/postgis/share/proj/proj.db" -f "$HERE/proof.sql"
stop "$ROOT"
echo "== server log"; cat "$ROOT/server.log"

# ── 3. negative control ─────────────────────────────────────────────────────
echo
echo "== negative control: the same tree without postgis/share/proj; EPSG codes must not resolve"
mkdir -p "$NP"
tar -xJf "$ZONKY" -C "$NP"; tar -xzf "$TGZ" -C "$NP"; rm -rf "$NP/postgis/share/proj"
PORT=$(free_port); start "$NP" "$PORT"
sql "$PORT" -v ON_ERROR_STOP=1 -q <<'SQL'
CREATE EXTENSION postgis;
SELECT 'by SRID still works: ' || ST_AsText(ST_Transform(ST_SetSRID(ST_MakePoint(18.42, -33.92), 4326), 32734)) AS srid;
DO $$
BEGIN
  PERFORM ST_Transform(ST_MakePoint(18.42, -33.92), 'EPSG:4326', 'EPSG:32734');
  RAISE EXCEPTION 'negative control failed: EPSG codes resolved without the add-on''s proj.db';
EXCEPTION WHEN others THEN
  IF SQLERRM LIKE 'negative control failed%' THEN RAISE; END IF;
  RAISE NOTICE 'as expected without proj.db: %', SQLERRM;
END $$;
SQL
stop "$NP"
echo "verify passed"
