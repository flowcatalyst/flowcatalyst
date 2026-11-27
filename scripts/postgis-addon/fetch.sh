#!/usr/bin/env bash
# Downloads the pinned inputs of the PostGIS add-on and checks each against its
# SHA-256 in versions.sh. A file already present is checked, not downloaded
# again; a file that fails the check is removed and the script fails.
#
#   fetch.sh sources <dir> [bundled]      every source tarball (only the bundled ones with "bundled")
#   fetch.sh zonky <dir> <goos> <goarch>  zonky's PostgreSQL archive for one platform:
#                                         <dir>/zonky-<goos>-<goarch>.txz
#
# Needs curl, and unzip for zonky's .jar.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=versions.sh
. "$HERE/versions.sh"

die() { echo "fetch.sh: $*" >&2; exit 1; }

# get <url> <dest> <sha256>
get() {
  local url=$1 dest=$2 want=$3 got
  if [ ! -f "$dest" ]; then
    echo "fetch $url"
    curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$dest.part" "$url"
    mv "$dest.part" "$dest"
  fi
  got=$(sha256_of "$dest")
  if [ "$got" != "$want" ]; then
    rm -f "$dest"
    die "$(basename "$dest"): sha256 $got, expected $want"
  fi
  echo "ok    $(basename "$dest")  $got"
}

[ $# -ge 2 ] || die "usage: fetch.sh sources <dir> [bundled] | fetch.sh zonky <dir> <goos> <goarch>"
what=$1 dir=$2
mkdir -p "$dir"
case "$what" in
  sources)
    only=${3:-}
    printf '%s\n' "$SOURCES" | while read -r name _ role file url sha; do
      [ -n "$name" ] || continue
      if [ "$only" = bundled ] && [ "$role" != bundled ]; then continue; fi
      get "$url" "$dir/$file" "$sha"
    done
    ;;
  zonky)
    [ $# -eq 4 ] || die "usage: fetch.sh zonky <dir> <goos> <goarch>"
    line=$(printf '%s\n' "$ZONKY_JARS" | awk -v o="$3" -v a="$4" '$1 == o && $2 == a')
    [ -n "$line" ] || die "no zonky archive pinned for $3/$4"
    # shellcheck disable=SC2086
    set -- $line
    plat=$3 sha=$4
    jar=embedded-postgres-binaries-$plat-$ZONKY_VERSION.jar
    get "$ZONKY_MAVEN/embedded-postgres-binaries-$plat/$ZONKY_VERSION/$jar" "$dir/$jar" "$sha"
    txz=$dir/zonky-$1-$2.txz
    # The jar holds exactly one .txz, the archive embedded-postgres unpacks.
    unzip -p "$dir/$jar" '*.txz' > "$txz.part"
    [ -s "$txz.part" ] || die "$jar holds no .txz"
    mv "$txz.part" "$txz"
    echo "ok    $(basename "$txz")  (from $jar)"
    ;;
  *) die "unknown target $what" ;;
esac
