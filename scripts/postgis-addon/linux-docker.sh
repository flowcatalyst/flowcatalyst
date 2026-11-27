#!/usr/bin/env bash
# Builds and verifies the linux PostGIS add-on for one architecture in Docker.
# The release workflow runs this on a runner of that architecture; it also runs
# on any machine with Docker (another architecture through emulation, slowly).
#
#   linux-docker.sh <amd64|arm64>
#
# WORK (default ${TMPDIR:-/tmp}/postgis-addon/linux-<arch>) holds the sources,
# the build and, in WORK/out, the tarball. ADDON_TAG, JOBS and SOURCE_DATE_EPOCH
# are passed on to build-linux.sh.
#
#   1. almalinux:8 (as root): fetch.sh, build-linux.sh, then verify-linux.sh's
#      linkage check (readelf).
#   2. a fresh almalinux:8, with no network, as user nobody, nothing
#      installed (so no PROJ, GEOS or libxml2 of the system's beyond what the
#      image has): verify-linux.sh's runtime proof on zonky's own archive.
#   3. the same runtime proof in ubuntu:18.04 (glibc 2.27, the oldest zonky's
#      server runs on: it is built there) and ubuntu:24.04 (a recent glibc).
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

ARCH=${1:?usage: linux-docker.sh <amd64|arm64>}
case "$ARCH" in amd64|arm64) ;; *) echo "unknown architecture $ARCH" >&2; exit 1 ;; esac
WORK=${WORK:-${TMPDIR:-/tmp}/postgis-addon/linux-$ARCH}
mkdir -p "$WORK"
WORK=$(cd "$WORK" && pwd -P)
ASSET=$(addon_asset linux "$ARCH")
IMG_BUILD=almalinux:8
C=/addon/work       # $WORK inside the containers; the build's paths start here
S=/addon/scripts

echo "== build in $IMG_BUILD (linux/$ARCH), work dir $WORK"
docker run --rm --platform "linux/$ARCH" \
  -v "$HERE:$S:ro" -v "$WORK:$C" \
  -e ADDON_TAG -e JOBS -e SOURCE_DATE_EPOCH \
  "$IMG_BUILD" bash -euo pipefail -c "
    dnf -y -q install unzip >/dev/null
    $S/fetch.sh sources $C/src
    $S/fetch.sh zonky $C/src linux $ARCH
    SRC=$C/src WORK=$C OUT=$C/out $S/build-linux.sh
    $S/verify-linux.sh linkage $C/out/$ASSET $C
  "

# zonky's .txz as a plain .tar too, for images without xz.
docker run --rm --platform "linux/$ARCH" -v "$WORK:$C" "$IMG_BUILD" \
  sh -c "xz -dc $C/src/zonky-linux-$ARCH.txz > $C/src/zonky-linux-$ARCH.tar"

for img in almalinux:8 ubuntu:18.04 ubuntu:24.04; do
  echo
  echo "== runtime proof in $img (linux/$ARCH): no network, user nobody, nothing installed"
  # Only the add-on, zonky's archive and the private build (for psql) are mounted.
  docker run --rm --platform "linux/$ARCH" --network none --user 65534:65534 \
    --tmpfs /scratch:rw,exec,size=1g -e HOME=/scratch \
    -v "$HERE:$S:ro" -v "$WORK/out:$C/out:ro" -v "$WORK/src:$C/src:ro" -v "$WORK/pg:$C/pg:ro" \
    "$img" bash "$S/verify-linux.sh" runtime "$C/out/$ASSET" "$C/src/zonky-linux-$ARCH.tar" "$C/pg" /scratch
done
echo
echo "ok: $WORK/out/$ASSET"
