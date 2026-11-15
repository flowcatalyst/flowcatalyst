#!/usr/bin/env bash
# Publish one directory of this repository to its own repository as ONE commit per
# release: the commit's tree is <dir> exactly as it is at <ref>, its parent is the
# published repository's current main, its message is "Release vX.Y.Z", and it is
# pushed as main plus the plain tag vX.Y.Z. Nothing of this repository's history
# goes with it.
#
# Used for the Swift package clients/field-ios (github.com/flowcatalyst/field-ios):
# Swift Package Manager reads Package.swift at a repository root and resolves
# versions from plain vX.Y.Z tags, so the package cannot be consumed from a
# subdirectory of this repository.
#
# Usage: scripts/publish-git-subtree.sh <dir> <ref> <X.Y.Z> <repo-url>
#   <dir>       directory in this repository (clients/field-ios)
#   <ref>       commit or tag in this repository (release.sh passes <prefix>/vX.Y.Z)
#   <X.Y.Z>     the version to tag on the published repository
#   <repo-url>  the published repository (a test passes a local bare repository)
#
# PUBLISH_CHECK_ONLY=1 checks that <dir> exists at <ref> and, for a Swift package,
# that it builds on this machine (when a Swift toolchain is installed); it
# publishes nothing.
#
# Rerunnable: if vX.Y.Z is already published with the same tree it says so and
# exits 0; with a different tree it refuses (an app may have locked the tag).
set -euo pipefail

dir="${1:-}"
ref="${2:-}"
version="${3:-}"
repo="${4:-}"
[ -n "$dir" ] && [ -n "$ref" ] && [ -n "$version" ] && [ -n "$repo" ] || {
	echo "usage: scripts/publish-git-subtree.sh <dir> <ref> <X.Y.Z> <repo-url>" >&2
	exit 2
}

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

tree="$(git rev-parse --verify -q "$ref^{commit}:$dir")" || { echo "✗ $ref has no $dir" >&2; exit 1; }

if [ "${PUBLISH_CHECK_ONLY:-}" = "1" ]; then
	if git cat-file -e "$tree:Package.swift" 2>/dev/null; then
		if command -v swift >/dev/null 2>&1; then
			work="$(mktemp -d)"
			trap 'rm -rf "$work"' EXIT
			git archive "$tree" | tar -x -C "$work"
			echo "  checking $dir at $ref: swift build"
			swift build --package-path "$work" >"$work/.build.log" 2>&1 || {
				tail -40 "$work/.build.log" >&2
				echo "✗ $dir at $ref does not build." >&2
				exit 1
			}
		else
			echo "  no Swift toolchain here: $dir is not built before the release (its CI workflow is the check)."
		fi
	fi
	echo "✓ $dir at $ref is ready to publish."
	exit 0
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
echo "✓ Published $dir v$version ($commit) to $repo"
