#!/usr/bin/env bash
# Cut a FlowCatalyst release from the flowcatalyst-go repo.
#
# Usage: scripts/release.sh <kind> <bump>
#   <kind>  release line — currently: dev (the fc-dev developer binary)
#   <bump>  patch | minor | major | X.Y.Z[-suffix]
#
# The version source of truth is a per-line VERSION file, seeded from the Rust
# monorepo's current numbers so the Go repo continues the sequence (it does not
# restart at 0.0.1). The bump base is max(VERSION file, highest <prefix>/v* git
# tag) so a hand-cut tag can never collide with the next computed bump. On
# confirm this commits the bumped VERSION, tags <prefix>/vX.Y.Z, and pushes —
# the matching GitHub Actions workflow then builds and publishes the release.
set -euo pipefail

kind="${1:-}"
bump="${2:-}"

usage() {
	echo "usage: scripts/release.sh <dev|ts|laravel> <patch|minor|major|X.Y.Z[-suffix]>" >&2
	exit 2
}
[ -n "$kind" ] && [ -n "$bump" ] || usage

# manifest (optional) is a package file whose "version" must track the tag:
# TS package.json only. Laravel/composer.json and fc-dev have no such field
# (Packagist reads the tag; fc-dev embeds cmd/fc-dev/VERSION).
manifest=""
case "$kind" in
	dev)     prefix="fc-dev";         version_file="cmd/fc-dev/VERSION";             label="fc-dev" ;;
	ts)      prefix="typescript-sdk"; version_file="clients/typescript-sdk/VERSION"; label="typescript-sdk"; manifest="clients/typescript-sdk/package.json" ;;
	laravel) prefix="laravel-sdk";    version_file="clients/laravel-sdk/VERSION";    label="laravel-sdk" ;;
	*)       echo "✗ unknown release kind: $kind" >&2; usage ;;
esac

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

if [ -n "$(git status --porcelain)" ]; then
	echo "✗ Working tree is dirty. Commit or stash first." >&2
	git status --short
	exit 1
fi

[ -f "$version_file" ] || { echo "✗ missing version file: $version_file" >&2; exit 1; }

clean_re='^[0-9]+\.[0-9]+\.[0-9]+$'
file_version="$(tr -d '[:space:]' < "$version_file")"

# Highest existing <prefix>/vX.Y.Z tag (plain semver only; pre-release tags are
# ignored for the max so they can't outrank a clean release).
tag_version="$(git tag --list "$prefix/v*" \
	| sed "s|^$prefix/v||" \
	| awk -F. '/^[0-9]+\.[0-9]+\.[0-9]+$/ { printf "%010d%010d%010d %s\n", $1, $2, $3, $0 }' \
	| sort -r | awk 'NR==1 {print $2}')"

# Bump base = max(VERSION file, highest tag).
current="$file_version"
if [ -n "$tag_version" ] && [[ "$file_version" =~ $clean_re ]] && [[ "$tag_version" =~ $clean_re ]]; then
	higher="$(printf '%s\n%s\n' "$file_version" "$tag_version" \
		| awk -F. '{ printf "%010d%010d%010d %s\n", $1, $2, $3, $0 }' \
		| sort -r | awk 'NR==1 {print $2}')"
	current="$higher"
	if [ "$higher" = "$tag_version" ] && [ "$tag_version" != "$file_version" ]; then
		echo "  VERSION file: $file_version (behind)"
		echo "  Highest tag:  $tag_version"
		echo "  ↑ bumping from the tag, not the file."
	fi
fi

case "$bump" in
	patch|minor|major)
		if [[ ! "$current" =~ $clean_re ]]; then
			echo "✗ Cannot auto-bump '$current' (has a prerelease suffix). Pass an explicit X.Y.Z." >&2
			exit 1
		fi
		;;
esac

case "$bump" in
	patch) new="$(echo "$current" | awk -F. -v OFS=. '{$3++; print}')" ;;
	minor) new="$(echo "$current" | awk -F. -v OFS=. '{$2++; $3=0; print}')" ;;
	major) new="$(echo "$current" | awk -F. -v OFS=. '{$1++; $2=0; $3=0; print}')" ;;
	*)
		if [[ "$bump" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
			new="$bump"
		else
			echo "✗ '$bump' is not patch|minor|major|X.Y.Z[-suffix]" >&2
			exit 1
		fi
		;;
esac

if [ "$new" = "$current" ]; then
	echo "✗ Computed version $new equals current. Nothing to bump." >&2
	exit 1
fi
if git rev-parse -q --verify "refs/tags/$prefix/v$new" >/dev/null; then
	echo "✗ Tag $prefix/v$new already exists." >&2
	exit 1
fi

echo ""
echo "  $label: $current → $new"
echo ""

printf '%s\n' "$new" > "$version_file"

# Keep the package manifest's version in lockstep (TS only). POSIX
# [[:space:]] (not \s) so this works on macOS BSD awk as well as gawk.
if [ -n "$manifest" ]; then
	awk -v new="$new" '
		/^[[:space:]]*"version":[[:space:]]*"[^"]+",?[[:space:]]*$/ && !done {
			sub(/"version":[[:space:]]*"[^"]+"/, "\"version\": \"" new "\"")
			done=1
		}
		{print}
	' "$manifest" > "$manifest.tmp" && mv "$manifest.tmp" "$manifest"
	if ! grep -q "\"version\": \"$new\"" "$manifest"; then
		echo "✗ Failed to update version in $manifest. Reverting." >&2
		git checkout -- "$version_file" "$manifest"
		exit 1
	fi
fi

git --no-pager diff --stat -- "$version_file" $manifest
echo ""
read -r -p "Commit '$label v$new', tag $prefix/v$new, and push? [y/N] " confirm || confirm="n"
case "$confirm" in
	y|Y|yes|YES) ;;
	*) echo "Aborted. Reverting."; git checkout -- "$version_file" $manifest; exit 1 ;;
esac

git add -- "$version_file" $manifest
git commit -m "$label v$new"
git tag "$prefix/v$new"
git push origin HEAD "$prefix/v$new"

echo ""
echo "✓ Released $label v$new"
echo "  Workflow: https://github.com/flowcatalyst/flowcatalyst-go/actions/workflows/release-$prefix.yml"
echo "  Release:  https://github.com/flowcatalyst/flowcatalyst-go/releases/tag/$prefix/v$new"
