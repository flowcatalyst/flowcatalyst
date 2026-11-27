package main

// The PostGIS add-on release `fcdev install postgis` installs: the tag, and
// the SHA-256 of each platform's asset of that release. Nothing else is
// trusted: the archive is downloaded from this repository's GitHub release of
// the tag and installed only when its SHA-256 is the one here.
//
// The add-on is built and published by .github/workflows/release-postgis-addon.yml
// from scripts/postgis-addon (its README says how). To pin a release: copy the
// tag and the values of its SHA256SUMS asset here, in the commit that cuts the
// next fcdev release.
//
// The tag reads postgis-addon/v<PostGIS version>-pg<PostgreSQL major>-<build>;
// its major must be the one embeddedPGVersion pins (a test checks it). While
// the tag is empty, this fcdev has no add-on to install and says so.

// postgisAddonTag is the pinned release, e.g. "postgis-addon/v3.6.4-pg18-1".
const postgisAddonTag = "postgis-addon/v3.6.4-pg18-1"

// postgisAddonSHA256 holds the SHA-256 (hex) of each platform's asset,
// postgis-addon-<postgis>-pg<major>-<goos>-<goarch>.tar.gz, keyed goos/goarch.
// A platform that is missing, or empty, has no add-on in this fcdev.
var postgisAddonSHA256 = map[string]string{
	"darwin/amd64": "906c1ff9d47d51f7846f7b99eddae24d093831960cf01d0a9e978cc236aa7e51",
	"darwin/arm64": "1711c3506e39513df8f47f799154ebbe40a01c571b44bb05713fec0418a05dde",
	"linux/amd64":  "c6fc8cf3fb4854b38c6eb0f91794e6d4e8ba241f67454250065ad6ae46a7ba76",
	"linux/arm64":  "b799cfd27b9762ee8c78672b8d061fe97ac194644b75ace07904085ac98c4298",
}
