# PostGIS add-on for fcdev's embedded PostgreSQL

fcdev's embedded PostgreSQL is zonky's vanilla `embedded-postgres-binaries`
(PostgreSQL 18.3; `embeddedPGVersion` in `cmd/fcdev/embedded.go`), which has no
PostGIS. These scripts build **PostGIS 3.6.4, core `postgis` extension only**,
from source for those binaries, for macOS and glibc Linux on amd64 and arm64,
and prove each build on zonky's own archive. The release workflow
(`.github/workflows/release-postgis-addon.yml`) runs them and publishes the
tarballs; `fcdev install postgis` installs the one for its platform
(`docs/embedded-postgres-postgis.md`).

| File | What it does |
|---|---|
| `versions.sh` | Every pinned input: versions, URLs and SHA-256 of the sources and of zonky's archives; the glibc and macOS floors. Sourced by everything else. |
| `fetch.sh` | Downloads the sources (`fetch.sh sources <dir>`) or one platform's zonky archive (`fetch.sh zonky <dir> <goos> <goarch>`) and checks each against its SHA-256. |
| `linux-docker.sh` | Linux, one architecture, in Docker: build, linkage check, then the runtime proof in three fresh containers. |
| `build-linux.sh` | The Linux build itself, inside `almalinux:8`. |
| `verify-linux.sh` | `linkage`: what the module needs and exports; `runtime`: the proof on zonky's archive. |
| `build-darwin.sh` | The macOS build, natively on the machine's architecture. |
| `verify-darwin.sh` | Linkage check, then the proof on zonky's archive, sandboxed away from Homebrew. |
| `proof.sql` | The SQL both verify scripts run; every check raises an error when it fails. |

## The tarball

`postgis-addon-<postgis>-pg<major>-<goos>-<goarch>.tar.gz` (Go's GOOS/GOARCH),
laid out to be unpacked over the root of zonky's tree (the directory holding
`bin/`, `lib/` and `share/`):

| Path | |
|---|---|
| `postgis-addon.json` | postgis version, pg major, goos, goarch, the release tag, component versions. The last entry; fcdev writes it last, as the mark of a complete install. |
| `lib/postgresql/postgis-3.so` / `.dylib` | the module |
| `share/postgresql/extension/postgis.control`, `postgis--*.sql` | the extension, with upgrade scripts as plain files (no links), so `ALTER EXTENSION postgis UPDATE` works after a later add-on; the `--unpackaged` scripts are dropped (PostgreSQL 13 removed `FROM unpackaged`) |
| Linux: `lib/share/proj/proj.db`, `proj.ini` | PROJ's data |
| macOS: `postgis/lib/*.dylib` | GEOS, PROJ (with SQLite inside), json-c, libxml2, protobuf-c |
| macOS: `postgis/share/proj/proj.db`, `proj.ini` | PROJ's data |
| `postgis/share/licenses/<package>/` | licence texts of everything bundled, and a README naming the sources asset |

PROJ finds `proj.db` with no environment variable, through its rule
"`<directory of the object holding PROJ's code>/../share/proj`" (dladdr). On
macOS that object is `postgis/lib/libproj.*.dylib`, so `postgis/share/proj`; on
Linux PROJ is linked into `lib/postgresql/postgis-3.so`, so `lib/share/proj`.
PROJ is built with `EMBED_PROJ_DATA_PATH=OFF` (no fallback path of the build
machine is compiled in) and `EMBED_RESOURCE_FILES=OFF` (a static PROJ would
otherwise carry its own 9 MB copy of `proj.db`), so the shipped file is the one
used; each verify script's negative control removes it and shows EPSG codes no
longer resolve.

## How each platform is built

**Linux** (`build-linux.sh`, in `almalinux:8` with gcc-toolset-13). Every
dependency (GEOS, PROJ, SQLite, json-c, libxml2, the protobuf-c runtime) is
built from source as a static, position-independent archive with hidden
visibility and linked into `postgis-3.so`, with libstdc++ and libgcc
(`-static-libgcc`, and gcc-toolset's own `libstdc++.a` found before any shared
one). `-Wl,--exclude-libs,ALL` makes every symbol from those archives local,
so the module exports nothing of them and cannot bind to the libxml2 the server
has already loaded (zonky's `lib/libxml2.so.2`). The module then needs glibc
only (`libc`, `libm`, `libpthread`, `libdl`, the loader), with no RPATH.

zonky's linux binaries are built on Ubuntu 18.04 and need glibc symbols up to
`GLIBC_2.27`. AlmaLinux 8 has glibc 2.28, so the linkage check fails the build
if the module needs any symbol newer than `GLIBC_2.27` (SQLite is built with
`SQLITE_DISABLE_LFS` for this: its `_FILE_OFFSET_BITS=64` turns `fcntl` into
`fcntl64@GLIBC_2.28`, and on 64-bit Linux `off_t` is 64-bit anyway). The
runtime proof runs in `ubuntu:18.04` (glibc 2.27) as well as `almalinux:8` and
`ubuntu:24.04`, each with no network, as user `nobody`, with nothing installed
(no PROJ, GEOS or libxml2 of the system's).

**macOS** (`build-darwin.sh`). Every dependency is built from source with
`MACOSX_DEPLOYMENT_TARGET=13.0` (zonky's server is built for macOS 13) as a
shared library, copied into `postgis/lib` (zonky's `lib/` already has
`libxml2.16.dylib` and `libzstd.1.dylib`, which must not be overwritten), with
install names rewritten to `@loader_path`, every `LC_RPATH` removed, and every
file re-signed ad hoc. That suffices because zonky's `postgres` carries the
`disable-library-validation` entitlement. PROJ is built without TIFF and curl
(so no libtiff, webp, zstd, lzma or jpeg).

**Private PostgreSQL.** zonky's archives ship neither headers nor `pg_config`,
so both builds compile PostgreSQL 18.3 into a private prefix for PGXS (and for
the `psql` the verify scripts use; zonky ships none). Its configure options
that reach `pg_config.h` follow zonky's own build (its configure line is in its
`postgres` binary); nothing of it is shipped.

**Build tools that are not bundled**: on Linux the distribution's
`protobuf-c-compiler` (`protoc-c` turns PostGIS's two `.proto` files into C),
ICU, OpenSSL and zlib headers for the private PostgreSQL, bison and flex; on
macOS Homebrew's `protobuf-c` (for `protoc-c`) and `icu4c`, `openssl@3`, `lz4`
and `zstd` for the private PostgreSQL. The linkage checks fail if the add-on
refers to any of them.

### Intel Macs

The workflow builds `darwin/amd64` natively on GitHub's `macos-15-intel`
runner (GitHub also lists `macos-26-intel`; macOS 14 images were retired on
2 November 2026), so the proof runs on a real x86_64 server.
zonky's darwin `postgres` is a universal binary (the same file in its
darwin-amd64 and darwin-arm64v8 archives), so either add-on matches it on its
own architecture. When GitHub has no Intel runner left, build amd64 on an arm64 runner
by cross-compiling (`-arch x86_64`, `CMAKE_OSX_ARCHITECTURES=x86_64`,
`--host=x86_64-apple-darwin`); that fallback is not written or tested, and its
runtime proof would need an x86_64 machine.

## Building locally

**Linux** needs Docker; the work directory defaults to
`${TMPDIR:-/tmp}/postgis-addon/linux-<arch>`:

```bash
scripts/postgis-addon/linux-docker.sh arm64     # native on an arm64 machine (Apple silicon's Docker is linux/arm64)
scripts/postgis-addon/linux-docker.sh amd64     # native on x86_64; elsewhere emulated, and several times slower
WORK=$HOME/addon-build scripts/postgis-addon/linux-docker.sh arm64
```

The tarball is `$WORK/out/postgis-addon-3.6.4-pg18-linux-<arch>.tar.gz`; the
logs of each step are in `$WORK/logs`.

**macOS** needs the Xcode command line tools, cmake, pkg-config and
`brew install protobuf-c icu4c openssl@3 lz4 zstd`:

```bash
W=$HOME/addon-build
scripts/postgis-addon/fetch.sh sources "$W/src"
scripts/postgis-addon/fetch.sh zonky "$W/src" darwin arm64      # or amd64 on an Intel Mac
SRC="$W/src" WORK="$W" OUT="$W/out" scripts/postgis-addon/build-darwin.sh
scripts/postgis-addon/verify-darwin.sh "$W/out/postgis-addon-3.6.4-pg18-darwin-arm64.tar.gz" \
  "$W/src/zonky-darwin-arm64.txz" "$W/pg" "$W"
```

A local build records the tag `postgis-addon/v<postgis>-pg<major>-dev`
(`ADDON_TAG` sets another). To try it in fcdev on a machine of that platform:
`fcdev install postgis --archive <tarball>`.

## Releasing

1. Change versions in `versions.sh` if needed (`fetch.sh` prints the SHA-256 it
   got on a mismatch; check it against upstream's own checksum where there is
   one, as the comments there say).
2. Dry run: run the workflow by hand (Actions → Release PostGIS add-on → Run
   workflow). It builds and verifies all four platforms and uploads the
   tarballs as artifacts; it releases nothing.
3. Tag and push `postgis-addon/v<postgis>-pg<major>-<build>`, e.g.
   `postgis-addon/v3.6.4-pg18-1` (a new build number for a rebuild of the same
   versions). The workflow builds, verifies, and publishes the four tarballs,
   `SHA256SUMS`, the sources bundle `postgis-addon-<postgis>-pg<major>-sources.tar.gz`
   (the exact upstream tarballs of everything bundled, and these scripts:
   PostGIS is GPL-2.0-or-later, GEOS LGPL-2.1-or-later), and a cosign
   signature and certificate for each.
4. Pin it: copy the tag and the four checksums from `SHA256SUMS` into
   `cmd/fcdev/postgis_addon_pins.go`, and release fcdev. Until then no fcdev
   installs it.
