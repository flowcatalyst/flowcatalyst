#!/usr/bin/env bash
# Publish the Go SDK (pkg/fcsdk) to its own repository, github.com/flowcatalyst/go-sdk,
# as ONE commit per release: the commit's tree is pkg/fcsdk exactly as it is at
# <ref>, its parent is the published repository's current main, its message is
# "Release vX.Y.Z", and it is pushed as main plus the plain tag vX.Y.Z (what the
# Go module proxy serves). Nothing of this repository's history goes with it
# (owner ruling 2026-10-30: the platform repository need not stay public).
#
# Usage: scripts/publish-go-sdk.sh <ref> <X.Y.Z> [repo-url]
#   <ref>       commit or tag in this repository (release.sh passes go-sdk/vX.Y.Z)
#   <X.Y.Z>     the version to tag on the published repository
#   [repo-url]  defaults to git@github.com:flowcatalyst/go-sdk.git (the smoke
#               test passes a local bare repository)
#
# Rerunnable: if vX.Y.Z is already published with the same tree it says so and
# exits 0; with a different tree it refuses (a published Go version is immutable:
# the proxy has cached it).
set -euo pipefail

ref="${1:-}"
version="${2:-}"
repo="${3:-git@github.com:flowcatalyst/go-sdk.git}"
[ -n "$ref" ] && [ -n "$version" ] || { echo "usage: scripts/publish-go-sdk.sh <ref> <X.Y.Z> [repo-url]" >&2; exit 2; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

tree="$(git rev-parse --verify -q "$ref^{commit}:pkg/fcsdk")" || { echo "✗ $ref has no pkg/fcsdk" >&2; exit 1; }
module="$(git show "$ref:pkg/fcsdk/go.mod" | awk '/^module[[:space:]]/ {print $2; exit}')"
if [ "$module" != "github.com/flowcatalyst/go-sdk" ]; then
	echo "✗ pkg/fcsdk/go.mod at $ref declares '$module'; the published repository serves github.com/flowcatalyst/go-sdk." >&2
	exit 1
fi

remote_refs="$(git ls-remote "$repo" refs/heads/main "refs/tags/v$version" "refs/tags/v$version^{}")" || {
	echo "✗ cannot reach $repo. Create the repository (empty) and check your access." >&2
	exit 1
}
if awk -v t="refs/tags/v$version" '$2 == t {f=1} END {exit !f}' <<< "$remote_refs"; then
	git fetch -q "$repo" "refs/tags/v$version"
	if [ "$(git rev-parse "FETCH_HEAD^{tree}")" = "$tree" ]; then
		echo "✓ v$version is already published from the same tree."
		exit 0
	fi
	echo "✗ v$version is already published with different content. Cut a new version." >&2
	exit 1
fi

if awk '$2 == "refs/heads/main" {f=1} END {exit !f}' <<< "$remote_refs"; then
	git fetch -q "$repo" refs/heads/main
	commit="$(git commit-tree "$tree" -p "$(git rev-parse FETCH_HEAD)" -m "Release v$version")"
else
	commit="$(git commit-tree "$tree" -m "Release v$version")"
fi

git push -q "$repo" "$commit:refs/heads/main" "$commit:refs/tags/v$version"
echo "✓ Published github.com/flowcatalyst/go-sdk v$version ($commit) to $repo"
