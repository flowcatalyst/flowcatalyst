#!/usr/bin/env bash
# Verifies a linux PostGIS add-on. Two parts, run separately by linux-docker.sh:
#
#   verify-linux.sh linkage <addon.tar.gz> [<build root>]
#     Needs readelf. The archive holds regular files and directories only, under
#     the paths fcdev accepts; postgis-3.so needs only glibc (no RPATH/RUNPATH,
#     no libstdc++, libgcc_s or any library of the build), no glibc symbol newer
#     than GLIBC_MAX, exports and imports nothing of its bundled libraries, and
#     holds no path of the build root.
#
#   verify-linux.sh runtime <addon.tar.gz> <zonky .txz or .tar> <pg prefix> <scratch dir>
#     Unpacks zonky's vanilla archive and the add-on over it, as fcdev does, and
#     runs zonky's initdb and pg_ctl with an empty environment on a free port
#     (never 15432 or 8080), then proof.sql with the private build's psql (the
#     <pg prefix>; zonky ships none), then a negative control without proj.db.
#     Must not run as root (PostgreSQL refuses).
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

fail() { echo "VERIFY FAILED: $*" >&2; exit 1; }

linkage() {
  local tgz=$1 root=${2:-} t mod bad=0 f
  command -v readelf >/dev/null || fail "readelf is needed"
  t=$(mktemp -d); trap 'rm -rf "$t"' RETURN

  echo "== archive entries"
  # Every entry a regular file or directory, relative, no "..", under the paths
  # fcdev installs (the same rules as its unpacker).
  tar -tvzf "$tgz" | awk '{print substr($1,1,1)}' | sort | uniq -c
  if tar -tvzf "$tgz" | awk '{t=substr($1,1,1)} t != "-" && t != "d" {bad=1; print} END {exit bad}'; then :; else fail "an entry is neither a file nor a directory"; fi
  while read -r f; do
    case "$f" in
      /*|*../*|..*) fail "unsafe path $f" ;;
      postgis-addon.json|lib/|lib/postgresql/*|lib/share/|lib/share/proj/*|share/|share/postgresql/|share/postgresql/extension/*|postgis/*) ;;
      *) fail "path outside what fcdev installs: $f" ;;
    esac
  done < <(tar -tzf "$tgz")
  [ "$(tar -tzf "$tgz" | tail -1)" = postgis-addon.json ] || fail "postgis-addon.json is not the last entry"
  tar -xzf "$tgz" -C "$t"
  echo "== postgis-addon.json"; cat "$t/postgis-addon.json"
  grep -q "\"postgis\": \"$POSTGIS_VERSION\"" "$t/postgis-addon.json" || fail "manifest postgis version"
  grep -q "\"pgMajor\": \"$PG_MAJOR\"" "$t/postgis-addon.json" || fail "manifest pg major"
  grep -q '"goos": "linux"' "$t/postgis-addon.json" || fail "manifest goos"

  mod=$t/lib/postgresql/postgis-3.so
  [ -f "$mod" ] || fail "no lib/postgresql/postgis-3.so"
  echo "== $(basename "$mod"): $(stat -c %s "$mod") bytes"
  readelf -h "$mod" | grep -E 'Class|Machine|Type'
  echo "== dynamic section (readelf -d)"
  readelf -d "$mod" | grep -E 'NEEDED|RPATH|RUNPATH|SONAME' || true
  if readelf -d "$mod" | grep -qE 'RPATH|RUNPATH'; then echo "has an RPATH/RUNPATH"; bad=1; fi
  while read -r lib; do
    case "$lib" in
      libc.so.6|libm.so.6|libpthread.so.0|libdl.so.2|librt.so.1|ld-linux-x86-64.so.2|ld-linux-aarch64.so.1) ;;
      *) echo "needs $lib, which is not glibc"; bad=1 ;;
    esac
  done < <(readelf -d "$mod" | sed -n 's/.*(NEEDED).*\[\(.*\)\]/\1/p')

  echo "== glibc symbol versions needed"
  readelf -V "$mod" | grep -o 'GLIBC_[0-9.]*' | sort -u -V | tr '\n' ' '; echo
  local newest
  newest=$(readelf -V "$mod" | grep -o 'GLIBC_[0-9.]*' | sed 's/GLIBC_//' | sort -u -V | tail -1)
  if [ "$(printf '%s\n%s\n' "$newest" "$GLIBC_MAX" | sort -V | tail -1)" != "$GLIBC_MAX" ]; then
    echo "needs GLIBC_$newest, newer than zonky's GLIBC_$GLIBC_MAX"; bad=1
    readelf -W --dyn-syms "$mod" | grep "GLIBC_$newest" | head
  else
    echo "newest GLIBC_$newest <= GLIBC_$GLIBC_MAX (zonky's linux server)"
  fi

  echo "== symbols (readelf --dyn-syms)"
  # The bundled libraries' names: none may be exported (another module could bind
  # to them) or imported (it would bind to the server's libxml2, or fail).
  local libre='^(_?xml|html|xsd|xpath|sqlite3|json_|lh_|array_list|printbuf|protobuf_c|GEOS|initGEOS|finishGEOS|proj_|pj_|geod_|_ZN|_ZS|_ZT|_Unwind|__gxx|__cxa_(allocate|throw|begin|end|rethrow|free|get|pure|guard|demangle|bad))'
  local exported imported
  # PostGIS's own SQL function GEOSnoop (postgis_geos_noop) matches the pattern.
  exported=$(readelf -W --dyn-syms "$mod" | awk '$5 != "LOCAL" && $7 != "UND" && NF >= 8 {print $8}' | sed 's/@.*//' | grep -v -E '^(pg_finfo_)?GEOSnoop$' | sort -u)
  # A weak undefined symbol may stay unresolved (it is then null): libstdc++'s
  # vtables name __cxa_pure_virtual weakly, which a static link does not pull in.
  imported=$(readelf -W --dyn-syms "$mod" | awk '$7 == "UND" && $5 != "WEAK" && NF >= 8 {print $8}' | sort -u)
  echo "weak and undefined: $(readelf -W --dyn-syms "$mod" | awk '$7 == "UND" && $5 == "WEAK" && NF >= 8 {print $8}' | sort -u | tr '\n' ' ')"
  echo "exported: $(echo "$exported" | grep -c .) symbols; imported: $(echo "$imported" | grep -c .) (glibc: $(echo "$imported" | grep -c '@GLIBC'), the rest from the server)"
  if echo "$exported" | grep -E "$libre" | head -5 | grep .; then echo "exports symbols of a bundled library"; bad=1; fi
  if echo "$imported" | grep -v '@GLIBC' | grep -E "$libre" | head -5 | grep .; then echo "imports symbols of a bundled library"; bad=1; fi
  echo "exported (sample): $(echo "$exported" | grep -E '^(Pg_magic_func|_PG_init|LWGEOM_in|postgis_full_version|geography_in)$' | tr '\n' ' ')"

  if [ -n "$root" ]; then
    echo "== build root $root in the module"
    local n
    n=$(grep -c -a -F "$root" "$mod" || true)
    if [ "$n" != 0 ]; then echo "$n lines of the module hold $root:"; grep -a -o -F "${root}" "$mod" | sort -u | head; bad=1; else echo "none"; fi
  fi
  echo "== licences"; (cd "$t/postgis/share/licenses" && ls)
  [ "$bad" = 0 ] || fail "linkage check"
  echo "linkage check passed"
}

free_port() {
  local p
  while :; do
    p=$(( (RANDOM << 15 | RANDOM) % 40000 + 20000 ))
    [ "$p" != 15432 ] && [ "$p" != 8080 ] || continue
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then echo "$p"; return; fi
  done
}

runtime() {
  local tgz=$1 zonky=$2 pgp=$3 scratch=$4 T ROOT NP PORT PSQL
  [ "$(id -u)" != 0 ] || fail "run the runtime proof as an unprivileged user"
  T=$scratch/postgis-verify; rm -rf "$T"; mkdir -p "$T"
  ROOT=$T/pgroot; NP=$T/noproj
  PSQL=$pgp/bin/psql
  # shellcheck disable=SC1091
  echo "== system: $(. /etc/os-release && echo "$PRETTY_NAME"), $(ldd --version | head -1), $(uname -m)"
  echo "== PROJ, GEOS or libxml2 installed in the system:"
  ls /usr/lib*/libproj* /usr/lib*/libgeos* /usr/lib/*/libproj* /usr/lib/*/libgeos* /usr/lib*/libxml2* /usr/lib/*/libxml2* 2>/dev/null || echo "   none"
  echo "== proj.db anywhere outside the add-on:"
  find / \( -path /proc -o -path /sys -o -path "$T" \) -prune -o -name proj.db -print 2>/dev/null | grep . || echo "   none"

  # run <cmd...>: the server's empty environment
  run() { env -i PATH=/usr/bin:/bin LANG=C HOME=/nonexistent "$@"; }
  start() {  # start <root> <port>
    run "$1/bin/initdb" -D "$1/data" -U postgres -A trust -E UTF8 --locale=C >/dev/null
    run "$1/bin/pg_ctl" -D "$1/data" -l "$1/server.log" -w \
      -o "-p $2 -c listen_addresses=127.0.0.1 -c unix_socket_directories=''" start >/dev/null
  }
  stop() { run "$1/bin/pg_ctl" -D "$1/data" -m fast -w stop >/dev/null 2>&1 || true; }
  sql() {  # sql <port> <psql args...>: the client gets zonky's lib for libssl 1.1
    LD_LIBRARY_PATH="$pgp/lib:$ROOT/lib" "$PSQL" -h 127.0.0.1 -p "$1" -U postgres -d postgres -X "${@:2}"
  }

  mkdir -p "$ROOT"
  tar -xf "$zonky" -C "$ROOT"
  tar -xzf "$tgz" -C "$ROOT"
  # shellcheck disable=SC2012
  echo "== tree: $(cd "$ROOT" && ls | tr '\n' ' ')"
  PORT=$(free_port)
  start "$ROOT" "$PORT"
  trap 'stop "$ROOT"; stop "$NP"' EXIT
  local pm; pm=$(head -1 "$ROOT/data/postmaster.pid")
  echo "== server on 127.0.0.1:$PORT (no unix socket), postmaster pid $pm; its environment:"
  tr '\0' '\n' < "/proc/$pm/environ" | sed 's/^/   /'

  cat > "$T/inspect.sh" <<'EOF'
echo "   mapped shared objects:"
awk '$6 ~ /\.so/ {print $6}' "/proc/$BACKEND/maps" | sort -u | sed 's/^/     /'
echo "   open files:"
for fd in /proc/$BACKEND/fd/*; do readlink "$fd"; done 2>/dev/null | grep '^/' | grep -v -e '/data/' -e '^/dev/' | sort -u | sed 's/^/     /'
echo "   anything of the system's PROJ/GEOS/libxml2: $(grep -c -E '/usr/.*(proj|geos|xml2)' "/proc/$BACKEND/maps")"
EOF
  INSPECT=$T/inspect.sh sql "$PORT" -v ON_ERROR_STOP=1 -e \
    -v postgis="$POSTGIS_VERSION" -v geos="$(source_field geos 2)" -v proj="$(source_field proj 2)" \
    -v projdb="$ROOT/lib/share/proj/proj.db" -f "$HERE/proof.sql"
  stop "$ROOT"
  echo "== server log"; cat "$ROOT/server.log"

  echo
  echo "== negative control: the same tree without lib/share/proj; EPSG codes must not resolve"
  mkdir -p "$NP"
  tar -xf "$zonky" -C "$NP"; tar -xzf "$tgz" -C "$NP"; rm -rf "$NP/lib/share/proj"
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
  stop "$NP"; trap - EXIT
  echo "runtime proof passed"
}

case "${1:-}" in
  linkage) shift; linkage "$@" ;;
  runtime) shift; [ $# -eq 4 ] || fail "usage: runtime <addon.tar.gz> <zonky archive> <pg prefix> <scratch dir>"; runtime "$@" ;;
  *) fail "usage: verify-linux.sh linkage <addon.tar.gz> [<build root>] | runtime <addon.tar.gz> <zonky archive> <pg prefix> <scratch dir>" ;;
esac
