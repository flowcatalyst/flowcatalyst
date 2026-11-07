#!/usr/bin/env bash
# Publish a JS package (clients/field-js, clients/field-react-native) to its own git
# repository, as ONE commit per release: the commit's tree is exactly what `npm pack`
# would publish from <dir> at <ref> (built output included), its parent is the published
# repository's current main, its message is "Release vX.Y.Z", and it is pushed as main
# plus the plain tag vX.Y.Z. Consumers install it from git, with no registry:
#
#   "@flowcatalyst/field-react-native": "github:flowcatalyst/field-react-native#v0.1.0"
#
# The package is exported from <ref> (not the working tree), installed with its
# committed lockfile, checked (typecheck/lint/test when the package defines them) and
# packed. The published package.json has no scripts and no devDependencies, so a git
# install never builds anything on the consumer's machine. Nothing of this repository's
# history goes with it.
#
# Usage: scripts/publish-git-package.sh <dir> <ref> <X.Y.Z> <repo-url>
#   <dir>       the package directory, e.g. clients/field-react-native
#   <ref>       commit or tag in this repository (release.sh passes <prefix>/vX.Y.Z)
#   <X.Y.Z>     the version to tag on the published repository
#   <repo-url>  e.g. git@github.com:flowcatalyst/field-react-native.git (a test may pass
#               a local bare repository)
#
# PUBLISH_CHECK_ONLY=1 stops after packing (release.sh runs it on HEAD before it commits
# the version bump, so a failing check never leaves a tag behind); the version is not
# compared and the repository is not contacted.
#
# Rerunnable: if vX.Y.Z is already published with the same tree it says so and exits 0;
# with a different tree it refuses (cut a new version instead of moving a tag consumers
# may have locked).
set -euo pipefail

dir="${1:-}"
ref="${2:-}"
version="${3:-}"
repo="${4:-}"
[ -n "$dir" ] && [ -n "$ref" ] && [ -n "$version" ] && [ -n "$repo" ] || {
	echo "usage: scripts/publish-git-package.sh <dir> <ref> <X.Y.Z> <repo-url>" >&2
	exit 2
}

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

git rev-parse --verify -q "$ref^{commit}:$dir" >/dev/null || { echo "✗ $ref has no $dir" >&2; exit 1; }
check_only="${PUBLISH_CHECK_ONLY:-}"
pkg_version="$(git show "$ref:$dir/package.json" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).version))')"
if [ -z "$check_only" ] && [ "$pkg_version" != "$version" ]; then
	echo "✗ $dir/package.json at $ref says $pkg_version, not $version." >&2
	exit 1
fi

remote_refs=""
if [ -z "$check_only" ]; then
	remote_refs="$(git ls-remote "$repo" refs/heads/main "refs/tags/v$version")" || {
		echo "✗ cannot reach $repo. Create the repository (empty) and check your access." >&2
		exit 1
	}
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Export the package as tagged, then install, check and pack it there.
mkdir -p "$work/src"
git archive "$ref" "$dir" | tar -x -C "$work/src"
pkg="$work/src/$dir"
# pnpm runs with --dir from this repository so the repository's tool versions (mise.toml)
# pick the pnpm; npm pack runs the package's prepack (the build).
export CI=true
pnpm --dir "$pkg" install --frozen-lockfile --silent
for script in typecheck lint test; do
	if node -e "process.exit(require(process.argv[1]).scripts?.[process.argv[2]] ? 0 : 1)" "$pkg/package.json" "$script"; then
		echo "  pnpm run $script"
		pnpm --dir "$pkg" run --silent "$script"
	fi
done
(cd "$pkg" && npm pack --silent --pack-destination "$work" >/dev/null)
tarball="$(ls "$work"/*.tgz)"
mkdir -p "$work/out"
tar -xzf "$tarball" -C "$work/out"
out="$work/out/package"
node -e '
	const fs = require("fs"), f = process.argv[1];
	const p = JSON.parse(fs.readFileSync(f, "utf8"));
	delete p.scripts; delete p.devDependencies;
	fs.writeFileSync(f, JSON.stringify(p, null, "\t") + "\n");
' "$out/package.json"
if [ -n "$check_only" ]; then
	echo "✓ $dir at $ref passes its checks and packs ($(cd "$out" && find . -type f | wc -l | tr -d ' ') files)."
	exit 0
fi

# One commit whose tree is the packed package, built in a scratch repository.
git -C "$out" init -q
git -C "$out" add -A
tree="$(git -C "$out" write-tree)"

if awk -v t="refs/tags/v$version" '$2 == t {f=1} END {exit !f}' <<< "$remote_refs"; then
	git -C "$out" fetch -q "$repo" "refs/tags/v$version"
	if [ "$(git -C "$out" rev-parse "FETCH_HEAD^{tree}")" = "$tree" ]; then
		echo "✓ v$version is already published from the same tree."
		exit 0
	fi
	echo "✗ v$version is already published with different content. Cut a new version." >&2
	exit 1
fi

source_commit="$(git rev-parse --short "$ref^{commit}")"
message="Release v$version

Built from $dir at $source_commit."
if awk '$2 == "refs/heads/main" {f=1} END {exit !f}' <<< "$remote_refs"; then
	git -C "$out" fetch -q "$repo" refs/heads/main
	commit="$(git -C "$out" commit-tree "$tree" -p "$(git -C "$out" rev-parse FETCH_HEAD)" -m "$message")"
else
	commit="$(git -C "$out" commit-tree "$tree" -m "$message")"
fi

git -C "$out" push -q "$repo" "$commit:refs/heads/main" "$commit:refs/tags/v$version"
echo "✓ Published $dir v$version ($commit) to $repo"
