#!/usr/bin/env bash
# Builds the PostGIS add-on for zonky's linux PostgreSQL (amd64 or arm64, the
# machine's own architecture). Runs inside an almalinux:8 container as root;
# linux-docker.sh starts it, and the release workflow runs linux-docker.sh.
#
#   SRC=<dir with fetch.sh sources>  WORK=<scratch dir>  OUT=<output dir>  build-linux.sh
#
# Optional: ADDON_TAG (recorded in postgis-addon.json; default a -dev tag),
# JOBS, SOURCE_DATE_EPOCH (timestamps in the tarball), SKIP_DNF=1.
#
# Why AlmaLinux 8: glibc 2.28 and gcc-toolset-13 (C++17 for GEOS and PROJ).
# zonky's linux server needs glibc symbols up to GLIBC_2.27 only; the linkage
# check in verify-linux.sh fails the build if the add-on needs anything newer
# (GLIBC_MAX in versions.sh), so it runs wherever zonky's server runs.
#
# Every dependency is built from source as a static, position-independent
# archive with hidden visibility and linked into postgis-3.so, with libstdc++
# and libgcc, and every symbol from those archives is made local
# (-Wl,--exclude-libs,ALL). So the module needs only glibc, exports nothing of
# GEOS, PROJ, libxml2 and the rest, and cannot bind to the different libxml2
# the server has already loaded (zonky's lib/libxml2.so.2). No RPATH.
#
# The tarball is laid out to be unpacked over the root of zonky's tree (the
# directory holding bin/, lib/ and share/), which is what fcdev does:
#
#   postgis-addon.json                       what this is (written last by fcdev)
#   lib/postgresql/postgis-3.so              the module, with everything linked in
#   share/postgresql/extension/postgis.control, postgis--*.sql
#   lib/share/proj/proj.db, proj.ini         PROJ's data: PROJ looks in <dir of the
#                                            object holding its code>/../share/proj,
#                                            and its code is in lib/postgresql/postgis-3.so
#   postgis/share/licenses/<package>/        licence texts of everything bundled
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

: "${SRC:?set SRC to the directory fetch.sh filled}" "${WORK:?set WORK}" "${OUT:?set OUT}"
case "$(uname -m)" in
  x86_64) GOARCH=amd64 ;;
  aarch64) GOARCH=arm64 ;;
  *) echo "unsupported machine $(uname -m)" >&2; exit 1 ;;
esac
ADDON_TAG=${ADDON_TAG:-postgis-addon/v$POSTGIS_VERSION-pg$PG_MAJOR-dev}
JOBS=${JOBS:-$(nproc)}
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(date +%s)}

B=$WORK/build                   # unpacked sources
DEPS=$WORK/deps                 # static dependencies (only .a, so -l finds nothing shared)
PGPREFIX=$WORK/pg               # private PostgreSQL: headers, pg_config, PGXS, psql
STAGE=$WORK/stage               # PostGIS `make install DESTDIR=`
ADDON=$WORK/addon               # becomes the tarball
LOGS=$WORK/logs
TOOLS=$WORK/tools               # wrapper scripts
rm -rf "$DEPS" "$STAGE" "$ADDON"
mkdir -p "$B" "$DEPS/lib" "$DEPS/include" "$DEPS/bin" "$LOGS" "$TOOLS" "$OUT"

log() { printf '\n==> %s\n' "$*"; }
# step <log name> <command...>: output to $LOGS/<name>.log, its tail on failure
step() {
  local name=$1; shift
  if ! "$@" > "$LOGS/$name.log" 2>&1; then
    echo "FAILED: $name (log $LOGS/$name.log):" >&2
    tail -60 "$LOGS/$name.log" >&2
    exit 1
  fi
}
unpack() {  # unpack <name>: fresh source tree of a SOURCES entry, prints its dir
  local file dir
  file=$(source_field "$1" 4)
  dir=$B/$1
  rm -rf "$dir"; mkdir -p "$dir"
  tar -xf "$SRC/$file" -C "$dir" --strip-components=1
  echo "$dir"
}

# --- 0. toolchain ------------------------------------------------------------------
if [ "${SKIP_DNF:-0}" != 1 ]; then
  log "toolchain (dnf)"
  step dnf dnf -y install \
    gcc-toolset-13-gcc gcc-toolset-13-gcc-c++ gcc-toolset-13-libstdc++-devel gcc-toolset-13-binutils \
    make cmake perl bison flex \
    bzip2 xz tar gzip unzip diffutils file findutils which pkgconf-pkg-config \
    protobuf-c-compiler libicu-devel openssl-devel zlib-devel
fi
# shellcheck disable=SC1091
. /opt/rh/gcc-toolset-13/enable
gcc --version | head -1
cmake --version | head -1

# Build paths are mapped away so none is compiled into the add-on (__FILE__ in
# assertions, debug info); verify-linux.sh checks for $WORK in the module.
MAP="-ffile-prefix-map=$WORK/="
export CFLAGS="-O2 -fPIC -fvisibility=hidden $MAP"
export CXXFLAGS="-O2 -fPIC -fvisibility=hidden -fvisibility-inlines-hidden $MAP"
# pkg-config sees only the .pc files of the static dependencies, with their
# private libraries (a static archive needs its dependencies on the link line).
export PKG_CONFIG_LIBDIR=$DEPS/lib/pkgconfig
cat > "$TOOLS/pkg-config" <<EOF
#!/bin/sh
exec /usr/bin/pkg-config --static "\$@"
EOF
chmod +x "$TOOLS/pkg-config"
export PKG_CONFIG=$TOOLS/pkg-config
CMAKE_COMMON=(-DCMAKE_INSTALL_PREFIX="$DEPS" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_BUILD_TYPE=Release
  -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON -DBUILD_TESTING=OFF)

# --- 1. SQLite (for PROJ), from the amalgamation -------------------------------------
log "sqlite $(source_field sqlite 2)"
d=$(unpack sqlite)
(
  cd "$d"
  # shellcheck disable=SC2086
  # SQLITE_DISABLE_LFS: no _FILE_OFFSET_BITS=64 (off_t is 64-bit here anyway),
  # which on glibc 2.28 headers turns fcntl() into fcntl64@GLIBC_2.28.
  gcc $CFLAGS -DSQLITE_THREADSAFE=1 -DSQLITE_OMIT_LOAD_EXTENSION=1 -DSQLITE_DISABLE_LFS=1 -c sqlite3.c -o sqlite3.o
  ar rcs "$DEPS/lib/libsqlite3.a" sqlite3.o
  cp sqlite3.h sqlite3ext.h "$DEPS/include/"
  # the shell, which PROJ's build uses to make proj.db (never shipped)
  gcc -O2 -DSQLITE_THREADSAFE=1 shell.c sqlite3.c -o "$DEPS/bin/sqlite3" -lpthread -ldl -lm
) > "$LOGS/sqlite.log" 2>&1 || { tail -40 "$LOGS/sqlite.log"; exit 1; }
mkdir -p "$DEPS/lib/pkgconfig"
cat > "$DEPS/lib/pkgconfig/sqlite3.pc" <<EOF
prefix=$DEPS
libdir=\${prefix}/lib
includedir=\${prefix}/include
Name: SQLite
Description: SQL database engine
Version: $(source_field sqlite 2)
Libs: -L\${libdir} -lsqlite3
Libs.private: -lpthread -lm
Cflags: -I\${includedir}
EOF

# --- 2. json-c --------------------------------------------------------------------------
log "json-c $(source_field json-c 2)"
d=$(unpack json-c)
step json-c-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DBUILD_STATIC_LIBS=ON \
  -DBUILD_APPS=OFF -DDISABLE_EXTRA_LIBS=ON -DDISABLE_WERROR=ON
step json-c-build cmake --build "$d/_b" -j "$JOBS" --target install

# --- 3. libxml2 (no zlib, ICU, HTTP or catalogs: PostGIS needs the parser and XPath only;
# without catalogs no default catalog path of this build is compiled in) --------------
log "libxml2 $(source_field libxml2 2)"
d=$(unpack libxml2)
(cd "$d" && step libxml2-configure ./configure --prefix="$DEPS" --disable-shared --enable-static --with-pic \
  --without-python --without-zlib --without-icu --without-readline --without-history \
  --without-http --without-modules --without-debug --without-catalog)
step libxml2-make make -C "$d" -j "$JOBS" install

# --- 4. protobuf-c runtime (PostGIS's MVT and Geobuf output) ------------------------------
# The protoc-c compiler is the distribution's (protobuf-c-compiler): it only turns
# PostGIS's two .proto files into C at build time; nothing of it is shipped.
log "protobuf-c $(source_field protobuf-c 2)"
d=$(unpack protobuf-c)
(cd "$d" && step protobuf-c-configure ./configure --prefix="$DEPS" --disable-shared --enable-static --with-pic --disable-protoc)
step protobuf-c-make make -C "$d" -j "$JOBS" install

# --- 5. GEOS ----------------------------------------------------------------------------
log "geos $(source_field geos 2)"
d=$(unpack geos)
step geos-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DBUILD_DOCUMENTATION=OFF -DBUILD_GEOSOP=OFF \
  -DBUILD_BENCHMARKS=OFF
step geos-build cmake --build "$d/_b" -j "$JOBS" --target install
# PostGIS asks geos-config --clibs for its link line; a static GEOS needs
# --static-clibs (libgeos and libstdc++ after libgeos_c).
cat > "$TOOLS/geos-config" <<EOF
#!/bin/sh
case "\$1" in
  --clibs) exec "$DEPS/bin/geos-config" --static-clibs ;;
  *) exec "$DEPS/bin/geos-config" "\$@" ;;
esac
EOF
chmod +x "$TOOLS/geos-config"

# --- 6. PROJ (no TIFF grids, no network: no libtiff, curl, zstd, lzma, jpeg) --------------
# EMBED_PROJ_DATA_PATH=OFF: no fallback data path of this build machine is
# compiled in. EMBED_RESOURCE_FILES=OFF: a static PROJ would otherwise carry a
# copy of proj.db inside the module (9 MB). So proj.db is the add-on's
# lib/share/proj/proj.db or nothing (verify-linux.sh's negative control).
log "proj $(source_field proj 2)"
d=$(unpack proj)
step proj-cmake cmake -S "$d" -B "$d/_b" "${CMAKE_COMMON[@]}" -DENABLE_TIFF=OFF -DENABLE_CURL=OFF \
  -DBUILD_APPS=OFF -DEMBED_PROJ_DATA_PATH=OFF -DEMBED_RESOURCE_FILES=OFF \
  -DSQLite3_INCLUDE_DIR="$DEPS/include" -DSQLite3_LIBRARY="$DEPS/lib/libsqlite3.a" -DEXE_SQLITE3="$DEPS/bin/sqlite3"
step proj-build cmake --build "$d/_b" -j "$JOBS" --target install

# libstdc++ and libgcc, statically: gcc-toolset's full libstdc++.a in the deps
# directory, so every -lstdc++ (PostGIS's own C++ parts, GEOS, PROJ) finds the
# archive there before any shared libstdc++.
cp "$(g++ -print-file-name=libstdc++.a)" "$DEPS/lib/libstdc++.a"
if ls "$DEPS"/lib/*.so* >/dev/null 2>&1; then echo "a shared library is in $DEPS/lib:"; ls "$DEPS"/lib/*.so*; exit 1; fi

# --- 7. PostgreSQL, privately, for headers, pg_config, PGXS and psql ----------------------------
# zonky's archive has no include/ and no pg_config. The options that put
# something into pg_config.h follow zonky's build (its configure line is in
# its postgres binary: --with-icu --with-libxml --with-openssl, no readline);
# libxml2 is the one built above, so PGXS hands PostGIS those headers and not
# the system's. Nothing of this build is shipped.
log "postgresql $PG_VERSION (private)"
if [ ! -x "$PGPREFIX/bin/pg_config" ]; then
  d=$(unpack postgresql)
  (cd "$d" && step pg-configure ./configure --prefix="$PGPREFIX" --without-readline \
    --with-icu --with-libxml --with-openssl XML2_CONFIG="$DEPS/bin/xml2-config" \
    CFLAGS="-O2 $MAP" PKG_CONFIG=/usr/bin/pkg-config PKG_CONFIG_LIBDIR=/usr/lib64/pkgconfig:/usr/share/pkgconfig)
  step pg-make make -C "$d" -j "$JOBS"
  step pg-install make -C "$d" install
fi
PG_CONFIG=$PGPREFIX/bin/pg_config
"$PG_CONFIG" --version

# --- 8. PostGIS, core extension only ------------------------------------------------------
log "postgis $POSTGIS_VERSION"
d=$(unpack postgis)
(cd "$d" && step postgis-configure ./configure \
  --with-pgconfig="$PG_CONFIG" \
  --with-geosconfig="$TOOLS/geos-config" \
  --with-xml2config="$DEPS/bin/xml2-config" \
  --with-jsondir="$DEPS" \
  --with-protobufdir="$DEPS" \
  --without-raster --without-topology --without-sfcgal \
  --without-address-standardizer --without-tiger \
  --without-gui --with-gettext=no \
  LDFLAGS="-L$DEPS/lib")
sed -n '/PostGIS is now configured/,$p' "$LOGS/postgis-configure.log"
# The module's link: libgcc static, every symbol of a static archive made local.
# --as-needed keeps glibc libraries PostGIS names but does not use off NEEDED.
SL="-static-libgcc -Wl,--exclude-libs,ALL -Wl,--as-needed"
step postgis-make make -C "$d" -j "$JOBS" LDFLAGS_SL="$SL"
step postgis-install make -C "$d" install DESTDIR="$STAGE" LDFLAGS_SL="$SL"

# --- 9. assemble ------------------------------------------------------------------------------
log "assemble"
rm -rf "$ADDON"
EXTDIR=$ADDON/share/postgresql/extension
MODDIR=$ADDON/lib/postgresql
LIC=$ADDON/postgis/share/licenses
mkdir -p "$EXTDIR" "$MODDIR" "$ADDON/lib/share/proj" "$LIC"
SPKGLIB=$STAGE$("$PG_CONFIG" --pkglibdir)
SSHARE=$STAGE$("$PG_CONFIG" --sharedir)
cp "$SPKGLIB/postgis-3.so" "$MODDIR/"
strip --strip-unneeded "$MODDIR/postgis-3.so"
cp "$SSHARE/extension/postgis.control" "$EXTDIR/"
# The upgrade scripts are symlinks to one file in PostGIS's install; they are
# copied as files (fcdev's unpacker takes regular files only). They let
# ALTER EXTENSION postgis UPDATE run when a later add-on replaces this one. The
# --unpackaged scripts are dropped: PostgreSQL 13 removed FROM unpackaged.
for f in "$SSHARE"/extension/postgis--*.sql; do
  case "$f" in *unpackaged*) continue ;; esac
  cp -L "$f" "$EXTDIR/"
done
cp "$DEPS/share/proj/proj.db" "$DEPS/share/proj/proj.ini" "$ADDON/lib/share/proj/"

# Licence texts of everything in the module.
lic() {  # lic <package dir name> <source dir> <files...>
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
# libstdc++ and libgcc (GPL-3.0 with the GCC Runtime Library Exception)
gccv=$(gcc -dumpversion)
mkdir -p "$LIC/gcc-runtime-$gccv"
for f in $(rpm -qld gcc-toolset-13-gcc gcc-toolset-13-libstdc++-devel 2>/dev/null | grep -E 'COPYING(3|\.RUNTIME|\.LIB)?$' | sort -u); do
  cp "$f" "$LIC/gcc-runtime-$gccv/"
done
ls "$LIC/gcc-runtime-$gccv"/COPYING* >/dev/null 2>&1 || { echo "no GCC licence texts found"; exit 1; }
cat > "$LIC/README.txt" <<EOF
PostGIS add-on for fcdev's embedded PostgreSQL ($ADDON_TAG, linux/$GOARCH).

lib/postgresql/postgis-3.so is PostGIS $POSTGIS_VERSION (GPL-2.0-or-later) with these
linked in statically: GEOS $(source_field geos 2) (LGPL-2.1-or-later), PROJ $(source_field proj 2) (MIT),
SQLite $(source_field sqlite 2) (public domain), json-c $(source_field json-c 2) (MIT), libxml2 $(source_field libxml2 2) (MIT),
protobuf-c $(source_field protobuf-c 2) (BSD-2-Clause), and the GCC $gccv runtime (libstdc++, libgcc:
GPL-3.0 with the GCC Runtime Library Exception). lib/share/proj/proj.db is PROJ's
database (MIT; its EPSG content is under the EPSG terms of use, see PROJ's COPYING).

The exact source tarballs of all of these, and the scripts that built this, are
the release asset postgis-addon-$POSTGIS_VERSION-pg$PG_MAJOR-sources.tar.gz of the
same release (https://github.com/flowcatalyst/flowcatalyst/releases/tag/$ADDON_TAG).
EOF
write_manifest "$ADDON/postgis-addon.json" linux "$GOARCH" "$ADDON_TAG"

# --- 10. tarball ---------------------------------------------------------------------------
ASSET=$(addon_asset linux "$GOARCH")
log "package $OUT/$ASSET"
# Reproducible: sorted names, no owners, fixed times, gzip without a timestamp.
# postgis-addon.json goes last in the archive, as fcdev writes it last.
(cd "$ADDON" && { find lib share postgis -print0 | LC_ALL=C sort -z; printf 'postgis-addon.json\0'; } | \
  tar --null --no-recursion -T - --owner=0 --group=0 --numeric-owner --mtime="@$SOURCE_DATE_EPOCH" \
      --mode='u+rwX,go+rX,go-w' -cf - | gzip -n -9 > "$OUT/$ASSET")
ls -l "$OUT/$ASSET"
du -sh "$ADDON"
echo "built $ASSET ($ADDON_TAG)"
