#!/usr/bin/env bash
# Builds the PostGIS add-on for zonky's darwin PostgreSQL, for the machine's
# own architecture (arm64 or x86_64). The release workflow runs it natively on
# an arm64 runner and on an Intel runner.
#
#   SRC=<dir with fetch.sh sources>  WORK=<scratch dir>  OUT=<output dir>  build-darwin.sh
#
# Optional: ADDON_TAG (recorded in postgis-addon.json; default a -dev tag),
# JOBS, SOURCE_DATE_EPOCH, BREW (Homebrew prefix; default `brew --prefix`).
#
# Needs: Xcode command line tools, cmake, pkg-config, and from Homebrew
# protobuf-c (only its protoc-c, which turns PostGIS's two .proto files into C)
# and icu4c, openssl@3, lz4 and zstd (only for the private PostgreSQL build,
# see step 7). Nothing from Homebrew is bundled: the linkage check in
# verify-darwin.sh fails the build if the add-on refers to anything outside
# itself, /usr/lib and /System, and verify-darwin.sh runs the server with
# Homebrew's directories unreadable.
#
# Every bundled library is built from source with MACOSX_DEPLOYMENT_TARGET=13.0
# (zonky's server is built for macOS 13). zonky's lib/ already holds
# libxml2.16.dylib and libzstd.1.dylib, which must not be overwritten, so the
# add-on's libraries go in their own prefix, postgis/lib, refer to each other
# by @loader_path, and are re-signed ad hoc after their install names are
# rewritten (zonky's postgres carries the disable-library-validation
# entitlement, which is why an ad hoc signature is enough). The tarball:
#
#   postgis-addon.json                     what this is (written last by fcdev)
#   lib/postgresql/postgis-3.dylib         the module
#   share/postgresql/extension/postgis.control, postgis--*.sql
#   postgis/lib/*.dylib                    GEOS, PROJ (with SQLite inside), json-c,
#                                          libxml2, protobuf-c
#   postgis/share/proj/proj.db, proj.ini   PROJ finds it relative to libproj
#                                          (<dir of libproj>/../share/proj)
#   postgis/share/licenses/<package>/      licence texts of everything bundled
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

: "${SRC:?set SRC to the directory fetch.sh filled}" "${WORK:?set WORK}" "${OUT:?set OUT}"
case "$(uname -m)" in
  arm64) GOARCH=arm64 ;;
  x86_64) GOARCH=amd64 ;;
  *) echo "unsupported machine $(uname -m)" >&2; exit 1 ;;
esac
ADDON_TAG=${ADDON_TAG:-postgis-addon/v$POSTGIS_VERSION-pg$PG_MAJOR-dev}
JOBS=${JOBS:-$(sysctl -n hw.ncpu)}
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(date +%s)}
BREW=${BREW:-$(brew --prefix)}
export MACOSX_DEPLOYMENT_TARGET=$MACOS_MIN

mkdir -p "$WORK" "$OUT"
WORK=$(cd "$WORK" && pwd -P)
B=$WORK/build
DEPS=$WORK/deps
PGPREFIX=$WORK/pg
STAGE=$WORK/stage
ADDON=$WORK/addon
LOGS=$WORK/logs
rm -rf "$DEPS" "$STAGE" "$ADDON"
mkdir -p "$B" "$DEPS/lib" "$DEPS/include" "$DEPS/bin" "$LOGS"

log() { printf '\n==> %s\n' "$*"; }
step() {
  local name=$1; shift
  if ! "$@" > "$LOGS/$name.log" 2>&1; then
    echo "FAILED: $name (log $LOGS/$name.log):" >&2
    tail -60 "$LOGS/$name.log" >&2
    exit 1
  fi
}
unpack() {
  local file dir
  file=$(source_field "$1" 4)
  dir=$B/$1
  rm -rf "$dir"; mkdir -p "$dir"
  tar -xf "$SRC/$file" -C "$dir" --strip-components=1
  echo "$dir"
}

MAP="-ffile-prefix-map=$WORK/="
export CFLAGS="-O2 $MAP"
export CXXFLAGS="-O2 $MAP"
export LDFLAGS="-Wl,-headerpad_max_install_names"   # room for install_name_tool
# Only the dependencies built here, never Homebrew's .pc files.
export PKG_CONFIG_LIBDIR=$DEPS/lib/pkgconfig
CMAKE_COMMON=(-DCMAKE_INSTALL_PREFIX="$DEPS" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_BUILD_TYPE=Release
  -DBUILD_SHARED_LIBS=ON -DBUILD_TESTING=OFF -DCMAKE_OSX_DEPLOYMENT_TARGET="$MACOS_MIN"
  -DCMAKE_FIND_FRAMEWORK=NEVER -DCMAKE_IGNORE_PREFIX_PATH="$BREW;/opt/homebrew;/usr/local")
clang --version | head -1
cmake --version | head -1

# --- 1. SQLite, static and hidden, linked into libproj only ----------------------------
log "sqlite $(source_field sqlite 2)"
d=$(unpack sqlite)
(
  cd "$d"
  # shellcheck disable=SC2086
  clang $CFLAGS -fPIC -fvisibility=hidden -DSQLITE_THREADSAFE=1 -DSQLITE_OMIT_LOAD_EXTENSION=1 -c sqlite3.c -o sqlite3.o
  ar rcs "$DEPS/lib/libsqlite3.a" sqlite3.o
  cp sqlite3.h sqlite3ext.h "$DEPS/include/"
  clang -O2 -DSQLITE_THREADSAFE=1 shell.c sqlite3.c -o "$DEPS/bin/sqlite3"   # builds proj.db; not shipped
) > "$LOGS/sqlite.log" 2>&1 || { tail -40 "$LOGS/sqlite.log"; exit 1; }

# --- 2. json-c ---------------------------------------------------------------------------
log "json-c $(source_field json-c 2)"
d=$(unpack json-c)
step json-c-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DBUILD_STATIC_LIBS=OFF \
  -DBUILD_APPS=OFF -DDISABLE_EXTRA_LIBS=ON -DDISABLE_WERROR=ON
step json-c-build cmake --build "$d/_b" -j "$JOBS" --target install

# --- 3. libxml2 (macOS's own libiconv; no zlib, ICU, HTTP or catalogs) ---------------------
log "libxml2 $(source_field libxml2 2)"
d=$(unpack libxml2)
(cd "$d" && step libxml2-configure ./configure --prefix="$DEPS" --enable-shared --disable-static \
  --without-python --without-zlib --without-icu --without-readline --without-history \
  --without-http --without-modules --without-debug --without-catalog)
step libxml2-make make -C "$d" -j "$JOBS" install

# --- 4. protobuf-c runtime ---------------------------------------------------------------
log "protobuf-c $(source_field protobuf-c 2)"
d=$(unpack protobuf-c)
(cd "$d" && step protobuf-c-configure ./configure --prefix="$DEPS" --enable-shared --disable-static --disable-protoc)
step protobuf-c-make make -C "$d" -j "$JOBS" install

# --- 5. GEOS -------------------------------------------------------------------------------
log "geos $(source_field geos 2)"
d=$(unpack geos)
step geos-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DBUILD_DOCUMENTATION=OFF -DBUILD_GEOSOP=OFF \
  -DBUILD_BENCHMARKS=OFF
step geos-build cmake --build "$d/_b" -j "$JOBS" --target install

# --- 6. PROJ (no TIFF, no network; proj.db as a file, no build path compiled in) -----------
log "proj $(source_field proj 2)"
d=$(unpack proj)
step proj-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DENABLE_TIFF=OFF -DENABLE_CURL=OFF \
  -DBUILD_APPS=OFF -DEMBED_PROJ_DATA_PATH=OFF -DEMBED_RESOURCE_FILES=OFF \
  -DSQLite3_INCLUDE_DIR="$DEPS/include" -DSQLite3_LIBRARY="$DEPS/lib/libsqlite3.a" -DEXE_SQLITE3="$DEPS/bin/sqlite3"
step proj-build cmake --build "$d/_b" -j "$JOBS" --target install

# --- 7. PostgreSQL, privately, for headers, pg_config, PGXS and psql -------------------------
# zonky's darwin archive (EDB's build) has no include/ and no pg_config. The
# options that put something into pg_config.h follow EDB's configure line
# (in zonky's postgres binary) where the library is at hand: ICU, OpenSSL,
# LZ4 and Zstandard from Homebrew, libxml2 the one built above (so PGXS hands
# PostGIS those headers). Omitted, as none changes what an extension sees
# (PG_MODULE_MAGIC checks the major, FUNC_MAX_ARGS, INDEX_MAX_KEYS, NAMEDATALEN,
# FLOAT8PASSBYVAL, all defaults in both builds): perl, python, tcl, libxslt,
# libcurl, uuid, LDAP, PAM, Bonjour, GSSAPI. Nothing of this build is shipped.
log "postgresql $PG_VERSION (private)"
if [ ! -x "$PGPREFIX/bin/pg_config" ]; then
  d=$(unpack postgresql)
  ICU=$(brew --prefix icu4c) SSL=$(brew --prefix openssl@3) LZ4=$(brew --prefix lz4) ZSTD=$(brew --prefix zstd)
  (cd "$d" && step pg-configure ./configure --prefix="$PGPREFIX" --without-readline \
    --with-icu --with-openssl --with-libxml --with-lz4 --with-zstd \
    --with-includes="$SSL/include" --with-libraries="$SSL/lib" \
    XML2_CONFIG="$DEPS/bin/xml2-config" CFLAGS="-O2 $MAP" \
    PKG_CONFIG_LIBDIR="$ICU/lib/pkgconfig:$LZ4/lib/pkgconfig:$ZSTD/lib/pkgconfig:$SSL/lib/pkgconfig")
  step pg-make make -C "$d" -j "$JOBS"
  step pg-install make -C "$d" install
fi
PG_CONFIG=$PGPREFIX/bin/pg_config
"$PG_CONFIG" --version

# --- 8. PostGIS, core extension only ----------------------------------------------------------
log "postgis $POSTGIS_VERSION"
d=$(unpack postgis)
PROTOC_DIR=$(dirname "$(command -v protoc-c || echo "$BREW/bin/protoc-c")")
(cd "$d" && PATH="$PATH:$PROTOC_DIR" step postgis-configure ./configure \
  --with-pgconfig="$PG_CONFIG" \
  --with-geosconfig="$DEPS/bin/geos-config" \
  --with-xml2config="$DEPS/bin/xml2-config" \
  --with-jsondir="$DEPS" \
  --with-protobufdir="$DEPS" \
  --without-raster --without-topology --without-sfcgal \
  --without-address-standardizer --without-tiger \
  --without-gui --with-gettext=no \
  LDFLAGS="-L$DEPS/lib $LDFLAGS")
sed -n '/PostGIS is now configured/,$p' "$LOGS/postgis-configure.log"
step postgis-make make -C "$d" -j "$JOBS"
# PostGIS's extension Makefile sets top_srcdir=../.., which breaks PostgreSQL's
# MKDIR_P on macOS ($(top_srcdir)/config/install-sh); pass one.
step postgis-install make -C "$d" install DESTDIR="$STAGE" MKDIR_P='/bin/mkdir -p'

# --- 9. assemble -------------------------------------------------------------------------------
log "assemble"
EXTDIR=$ADDON/share/postgresql/extension
MODDIR=$ADDON/lib/postgresql
DEPDIR=$ADDON/postgis/lib
LIC=$ADDON/postgis/share/licenses
mkdir -p "$EXTDIR" "$MODDIR" "$DEPDIR" "$ADDON/postgis/share/proj" "$LIC"
SPKGLIB=$STAGE$("$PG_CONFIG" --pkglibdir)
SSHARE=$STAGE$("$PG_CONFIG" --sharedir)
cp "$SPKGLIB/postgis-3.dylib" "$MODDIR/"
cp "$SSHARE/extension/postgis.control" "$EXTDIR/"
for f in "$SSHARE"/extension/postgis--*.sql; do
  case "$f" in *unpackaged*) continue ;; esac
  cp -L "$f" "$EXTDIR/"
done
cp "$DEPS/share/proj/proj.db" "$DEPS/share/proj/proj.ini" "$ADDON/postgis/share/proj/"

is_system() { case "$1" in /usr/lib/*|/System/*) return 0 ;; *) return 1 ;; esac; }
# resolve <install name> <real path of the file that refers to it>
resolve() {
  local ref=$1 from=$2 dir rp cand
  dir=$(dirname "$from")
  case "$ref" in
    @loader_path/*) cand="$dir/${ref#@loader_path/}" ;;
    @rpath/*)
      cand=""
      while read -r rp; do
        rp=${rp/@loader_path/$dir}
        if [ -e "$rp/${ref#@rpath/}" ]; then cand="$rp/${ref#@rpath/}"; break; fi
      done < <(otool -l "$from" | awk '/cmd LC_RPATH/{getline; getline; print $2}')
      [ -n "$cand" ] || cand="$dir/${ref#@rpath/}" ;;
    *) cand=$ref ;;
  esac
  python3 -I -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$cand"
}
# Walk the module's dependency graph, copying every non-system library.
# (bash 3.2: no associative arrays; MAP lines are "<name> <real path>".)
MAPF=$WORK/bundled.map; : > "$MAPF"
QUEUE=("$MODDIR/postgis-3.dylib|$SPKGLIB/postgis-3.dylib")
while [ ${#QUEUE[@]} -gt 0 ]; do
  item=${QUEUE[0]}; QUEUE=("${QUEUE[@]:1}")
  f=${item%%|*}; src=${item#*|}
  self=$(otool -D "$f" | tail -n +2)
  for ref in $(otool -L "$f" | tail -n +2 | awk '{print $1}'); do
    [ "$ref" = "$self" ] && continue
    is_system "$ref" && continue
    real=$(resolve "$ref" "$src")
    [ -f "$real" ] || { echo "cannot resolve $ref from $src"; exit 1; }
    case "$real" in "$DEPS"/*) ;; *) echo "$f refers to $ref = $real, which was not built here"; exit 1 ;; esac
    name=$(basename "$ref")
    if [ ! -e "$DEPDIR/$name" ]; then
      echo "$name $real" >> "$MAPF"
      cp "$real" "$DEPDIR/$name"; chmod u+w "$DEPDIR/$name"
      QUEUE+=("$DEPDIR/$name|$real")
    fi
  done
done
echo "bundled:"; cat "$MAPF"

# Install names: the libraries refer to each other as @loader_path/<name>, the
# module to them as @loader_path/../../postgis/lib/<name>; no LC_RPATH is left.
fix_refs() {  # fix_refs <file> <prefix>
  local f=$1 prefix=$2 ref rp self
  self=$(otool -D "$f" | tail -n +2)
  while read -r ref; do
    is_system "$ref" && continue
    [ "$ref" = "$self" ] && continue
    install_name_tool -change "$ref" "$prefix/$(basename "$ref")" "$f"
  done < <(otool -L "$f" | tail -n +2 | awk '{print $1}')
  while read -r rp; do
    install_name_tool -delete_rpath "$rp" "$f"
  done < <(otool -l "$f" | awk '/cmd LC_RPATH/{getline; getline; print $2}')
}
while read -r name _; do
  install_name_tool -id "@loader_path/$name" "$DEPDIR/$name"
  fix_refs "$DEPDIR/$name" "@loader_path"
done < "$MAPF"
fix_refs "$MODDIR/postgis-3.dylib" "@loader_path/../../postgis/lib"
strip -x "$MODDIR/postgis-3.dylib" "$DEPDIR"/*.dylib
# install_name_tool and strip invalidate the signature, and arm64 refuses to
# map unsigned code: sign again, ad hoc.
for f in "$MODDIR/postgis-3.dylib" "$DEPDIR"/*.dylib; do codesign -s - -f "$f"; done

# Licence texts
lic() {
  local pkg=$1 dir=$2 f; shift 2
  mkdir -p "$LIC/$pkg"
  for f in "$@"; do cp "$dir/$f" "$LIC/$pkg/"; done
}
lic "postgis-$POSTGIS_VERSION" "$B/postgis" COPYING LICENSE.TXT
for f in "$B"/postgis/deps/*/LICENSE*; do
  [ -f "$f" ] || continue
  sub=$(basename "$(dirname "$f")")
  mkdir -p "$LIC/postgis-$POSTGIS_VERSION/deps-$sub"; cp "$f" "$LIC/postgis-$POSTGIS_VERSION/deps-$sub/"
done
lic "geos-$(source_field geos 2)" "$B/geos" COPYING
lic "proj-$(source_field proj 2)" "$B/proj" COPYING
lic "json-c-$(source_field json-c 2)" "$B/json-c" COPYING
lic "libxml2-$(source_field libxml2 2)" "$B/libxml2" Copyright
lic "protobuf-c-$(source_field protobuf-c 2)" "$B/protobuf-c" LICENSE
mkdir -p "$LIC/sqlite-$(source_field sqlite 2)"
cat > "$LIC/sqlite-$(source_field sqlite 2)/LICENSE" <<'EOF'
SQLite is in the public domain: https://sqlite.org/copyright.html
The author disclaims copyright to this source code. In place of a legal notice,
here is a blessing:

    May you do good and not evil.
    May you find forgiveness for yourself and forgive others.
    May you share freely, never taking more than you give.
EOF
cat > "$LIC/README.txt" <<EOF
PostGIS add-on for fcdev's embedded PostgreSQL ($ADDON_TAG, darwin/$GOARCH).

lib/postgresql/postgis-3.dylib is PostGIS $POSTGIS_VERSION (GPL-2.0-or-later). postgis/lib holds
GEOS $(source_field geos 2) (LGPL-2.1-or-later), PROJ $(source_field proj 2) (MIT) with SQLite $(source_field sqlite 2) (public
domain) linked in, json-c $(source_field json-c 2) (MIT), libxml2 $(source_field libxml2 2) (MIT) and protobuf-c $(source_field protobuf-c 2)
(BSD-2-Clause), as shared libraries: GEOS can be replaced by another build of the
same version. postgis/share/proj/proj.db is PROJ's database (MIT; its EPSG content
is under the EPSG terms of use, see PROJ's COPYING).

The exact source tarballs of all of these, and the scripts that built this, are
the release asset postgis-addon-$POSTGIS_VERSION-pg$PG_MAJOR-sources.tar.gz of the
same release (https://github.com/flowcatalyst/flowcatalyst/releases/tag/$ADDON_TAG).
EOF
write_manifest "$ADDON/postgis-addon.json" darwin "$GOARCH" "$ADDON_TAG"

# --- 10. tarball ----------------------------------------------------------------------------------
ASSET=$(addon_asset darwin "$GOARCH")
log "package $OUT/$ASSET"
stamp=$(date -u -r "$SOURCE_DATE_EPOCH" +%Y%m%d%H%M.%S)
find "$ADDON" -exec touch -h -t "$stamp" {} +
chmod -R u+rwX,go+rX,go-w "$ADDON"
LIST=$WORK/tar.list
(cd "$ADDON" && { find lib share postgis | LC_ALL=C sort; echo postgis-addon.json; } > "$LIST")
# bsdtar: -n takes the list as is (no recursion), so postgis-addon.json is last.
(cd "$ADDON" && COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata --uid 0 --gid 0 --uname '' --gname '' \
  -n -c -f - -T "$LIST" | gzip -n -9 > "$OUT/$ASSET")
ls -l "$OUT/$ASSET"
du -sh "$ADDON"
echo "built $ASSET ($ADDON_TAG)"
