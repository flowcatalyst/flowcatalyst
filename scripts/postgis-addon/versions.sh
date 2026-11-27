# shellcheck shell=bash disable=SC2034
# Pinned inputs of the PostGIS add-on for fcdev's embedded PostgreSQL. Sourced
# by every script in this directory (bash 3.2 compatible: macOS /bin/bash).
#
# Every download is checked against the SHA-256 here before it is used. Where
# upstream publishes a checksum of its own, the value here was checked against
# it when it was pinned (noted per line); the others were taken from the
# HTTPS download on the day they were pinned.
#
# Raising a version: change it here, run ./fetch.sh to get the new SHA-256 (it
# prints what it got on a mismatch), check it against upstream where upstream
# publishes one, then cut a new add-on tag (README.md, "Releasing").

# What the add-on is. The tag postgis-addon/v<POSTGIS_VERSION>-pg<PG_MAJOR>-<n>
# must agree with these two (the release workflow refuses one that does not).
POSTGIS_VERSION=3.6.4
PG_MAJOR=18

# zonky's embedded-postgres-binaries release fcdev runs (embeddedPGVersion in
# cmd/fcdev/embedded.go = embeddedpostgres.V18 = 18.3.0). The add-on is built
# against the headers of the same PostgreSQL release and verified on zonky's
# own archive.
ZONKY_VERSION=18.3.0
PG_VERSION=18.3

# glibc symbol versions the Linux add-on may need: no newer than zonky's own
# linux binaries need (their highest is GLIBC_2.27; they are built on Ubuntu
# 18.04), so the add-on runs wherever zonky's server runs.
GLIBC_MAX=2.27

# macOS: zonky's darwin server is built with -mmacosx-version-min=13.
MACOS_MIN=13.0

# Source tarballs, one per line:
#   <name> <version> <role> <file> <url> <sha256>
# role: bundled = compiled into the add-on (its source goes into the release's
# sources bundle); build = needed to build it, nothing of it is shipped.
#
#   postgresql  sha256 as published by postgresql.org (.sha256 beside the file)
#   postgis     md5 c2f5d43c7e5df0ec1af37ef60d95e0d2 as published by osgeo.org
#   geos        no upstream checksum
#   proj        md5 50ed234d73d68f7bf4378e9a020a40a7 as published by osgeo.org
#   sqlite      SHA3-256 b6ced585c750916b1052667239a12c25a7ff34e7f06e1e6e01ecb0427657d9c4 as on sqlite.org/download.html
#   json-c      no upstream checksum (the json-c project's release bucket)
#   libxml2     sha256 as published by download.gnome.org (.sha256sum)
#   protobuf-c  no upstream checksum (GitHub release asset)
SOURCES='
postgresql 18.3 build postgresql-18.3.tar.bz2 https://ftp.postgresql.org/pub/source/v18.3/postgresql-18.3.tar.bz2 d95663fbbf3a80f81a9d98d895266bdcb74ba274bcc04ef6d76630a72dee016f
postgis 3.6.4 bundled postgis-3.6.4.tar.gz https://download.osgeo.org/postgis/source/postgis-3.6.4.tar.gz ed8dc6679f1e06f7b113592b04cde2a7e00f1b1e681294c8ca2204058990cec6
geos 3.15.1 bundled geos-3.15.1.tar.bz2 https://download.osgeo.org/geos/geos-3.15.1.tar.bz2 8eb6dc2ea78b00c2f5109c6b0dba9e028c60bad42d7b1b0cc9952b12dc71f571
proj 9.9.0 bundled proj-9.9.0.tar.gz https://download.osgeo.org/proj/proj-9.9.0.tar.gz 791a0610547eeabb17006cfd49cdbd2034f3240f47ed5e88a1031811f4e2bcf3
sqlite 3.54.1 bundled sqlite-autoconf-3540100.tar.gz https://www.sqlite.org/2026/sqlite-autoconf-3540100.tar.gz 926ef8d3eab93fbf6e06011901273eec076b25594a4e1cdadd50c987f4f6d64e
json-c 0.19 bundled json-c-0.19.tar.gz https://s3.amazonaws.com/json-c_releases/releases/json-c-0.19.tar.gz 37ad0249902e301bd9052bf712e511fcc6acff4ecaad4b5900aad9ce564e26de
libxml2 2.15.4 bundled libxml2-2.15.4.tar.xz https://download.gnome.org/sources/libxml2/2.15/libxml2-2.15.4.tar.xz 98087fd181d9070724f3fbc65c7377db03038eb92bd882374daff44940138821
protobuf-c 1.5.2 bundled protobuf-c-1.5.2.tar.gz https://github.com/protobuf-c/protobuf-c/releases/download/v1.5.2/protobuf-c-1.5.2.tar.gz e2c86271873a79c92b58fef7ebf8de1aa0df4738347a8bd5d4e65a80a16d0d24
'

# zonky's archives from Maven Central, one per line:
#   <goos> <goarch> <zonky platform> <sha256 of the .jar>
# (each equals the .jar.sha256 Maven Central publishes beside it)
ZONKY_JARS='
linux amd64 linux-amd64 1974c3a59c3ae010e9818e81637e425aad94eaac26e4ef533b42e081fe7ca345
linux arm64 linux-arm64v8 1fb555a6602c13a113ab1f0cc9224d599948e85e51ac5f001393605505eaf78e
darwin amd64 darwin-amd64 cb68c57232945f5e74db946e6077d70cec56bfed35959b21acc750cc80ca0977
darwin arm64 darwin-arm64v8 a18ec9ec6a905e04f5a08a8f7870bddadd635feb54dee332a3daf80fada3cb1a
'
ZONKY_MAVEN=https://repo1.maven.org/maven2/io/zonky/test/postgres

# source_field <name> <field#>: one field of a SOURCES line (2 version, 3 role,
# 4 file, 5 url, 6 sha256).
source_field() {
  printf '%s\n' "$SOURCES" | awk -v n="$1" -v f="$2" '$1 == n { print $f; found = 1 } END { exit !found }'
}

# addon_asset <goos> <goarch>: the release asset name.
addon_asset() {
  printf 'postgis-addon-%s-pg%s-%s-%s.tar.gz\n' "$POSTGIS_VERSION" "$PG_MAJOR" "$1" "$2"
}

# sha256_of <file>
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# write_manifest <file> <goos> <goarch> <tag>: postgis-addon.json, the file
# fcdev writes last when it installs the add-on (its completeness marker).
write_manifest() {
  local comps="" name ver
  for name in geos proj sqlite json-c libxml2 protobuf-c; do
    ver=$(source_field "$name" 2)
    comps="$comps    \"$name\": \"$ver\",
"
  done
  comps=${comps%,
}
  cat > "$1" <<EOF
{
  "postgis": "$POSTGIS_VERSION",
  "pgMajor": "$PG_MAJOR",
  "goos": "$2",
  "goarch": "$3",
  "tag": "$4",
  "builtAgainst": "PostgreSQL $PG_VERSION, zonky embedded-postgres-binaries $ZONKY_VERSION",
  "components": {
$comps
  }
}
EOF
}
