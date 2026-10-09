#!/usr/bin/env bash
# Cut a FlowCatalyst release from the flowcatalyst-go repo.
#
# Usage: scripts/release.sh <kind> <bump>
#   <kind>  release line — currently: dev (the fcdev developer binary)
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
	echo "usage: scripts/release.sh <dev|ts|laravel|java|fn-ts|fn-rust|fn-go> <patch|minor|major|X.Y.Z[-suffix]>" >&2
	exit 2
}
[ -n "$kind" ] && [ -n "$bump" ] || usage

# manifest (optional) is a package file whose "version" must track the tag:
# TS package.json only. Laravel/composer.json and fcdev have no such field
# (Packagist reads the tag; fcdev embeds cmd/fcdev/VERSION). manifest_pom is
# the Java SDK's pom.xml, whose <version> element must track the tag.
#
# The function SDKs add: manifest_toml (Cargo.toml, first top-level
# `version = "…"`), lock_npm (package-lock.json root version, two places) and
# lock_cargo (Cargo.lock entry for $lock_cargo_crate). fn-go has none: a Go
# module is versioned purely by its tag (clients/fn-go/vX.Y.Z).
#
# RELEASE_DRY_RUN=1 runs every check and shows the bump diff, then reverts and
# exits without committing, tagging or pushing.
manifest=""
manifest_pom=""
manifest_toml=""
lock_npm=""
lock_cargo=""
lock_cargo_crate=""
workflow=""
case "$kind" in
	dev)     prefix="fcdev";         version_file="cmd/fcdev/VERSION";             label="fcdev" ;;
	ts)      prefix="typescript-sdk"; version_file="clients/typescript-sdk/VERSION"; label="typescript-sdk"; manifest="clients/typescript-sdk/package.json" ;;
	laravel) prefix="laravel-sdk";    version_file="clients/laravel-sdk/VERSION";    label="laravel-sdk" ;;
	java)    prefix="java-sdk";       version_file="clients/java-sdk/VERSION";       label="java-sdk"; manifest_pom="clients/java-sdk/pom.xml" ;;
	fn-ts)   prefix="fn-ts-sdk";      version_file="clients/fn-ts/VERSION";         label="fn-ts-sdk"; manifest="clients/fn-ts/package.json"; lock_npm="clients/fn-ts/package-lock.json"; workflow="split-fn-ts-sdk.yml" ;;
	fn-rust) prefix="fn-rust-sdk";    version_file="clients/fn-rust/VERSION";       label="fn-rust-sdk"; manifest_toml="clients/fn-rust/Cargo.toml"; lock_cargo="clients/fn-rust/Cargo.lock"; lock_cargo_crate="flowcatalyst-fn"; workflow="split-fn-rust-sdk.yml" ;;
	fn-go)   prefix="clients/fn-go";  version_file="clients/fn-go/VERSION";         label="fn-go-sdk" ;;
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

# Keep the Maven pom's <version> in lockstep (Java only). Only the first
# <version> element — the project's own — is rewritten.
if [ -n "$manifest_pom" ]; then
	awk -v new="$new" '
		/<version>[^<]+<\/version>/ && !done {
			sub(/<version>[^<]+<\/version>/, "<version>" new "</version>")
			done=1
		}
		{print}
	' "$manifest_pom" > "$manifest_pom.tmp" && mv "$manifest_pom.tmp" "$manifest_pom"
	if ! grep -q "<version>$new</version>" "$manifest_pom"; then
		echo "✗ Failed to update version in $manifest_pom. Reverting." >&2
		git checkout -- "$version_file" "$manifest_pom"
		exit 1
	fi
fi

# Cargo.toml: the first top-level `version = "…"` is the [package] one.
if [ -n "$manifest_toml" ]; then
	awk -v new="$new" '
		/^version[[:space:]]*=[[:space:]]*"[^"]+"/ && !done {
			sub(/"[^"]+"/, "\"" new "\"")
			done=1
		}
		{print}
	' "$manifest_toml" > "$manifest_toml.tmp" && mv "$manifest_toml.tmp" "$manifest_toml"
	if ! grep -q "^version = \"$new\"" "$manifest_toml"; then
		echo "✗ Failed to update version in $manifest_toml. Reverting." >&2
		git checkout -- "$version_file" "$manifest_toml"
		exit 1
	fi
fi

# package-lock.json: the root version appears twice (top level, and under
# packages[""]); dependency entries sit deeper so the indent tells them apart.
if [ -n "$lock_npm" ]; then
	awk -v new="$new" '
		/^  "version": "[^"]+",?[[:space:]]*$/ && !a { sub(/"version": "[^"]+"/, "\"version\": \"" new "\""); a=1 }
		/^      "version": "[^"]+",?[[:space:]]*$/ && a && !b { sub(/"version": "[^"]+"/, "\"version\": \"" new "\""); b=1 }
		{print}
	' "$lock_npm" > "$lock_npm.tmp" && mv "$lock_npm.tmp" "$lock_npm"
	if [ "$(grep -c "\"version\": \"$new\"" "$lock_npm")" -lt 2 ]; then
		echo "✗ Failed to update version in $lock_npm. Reverting." >&2
		git checkout -- "$version_file" "$manifest" "$lock_npm"
		exit 1
	fi
fi

# Cargo.lock: the crate's own [[package]] entry (name line, then version).
if [ -n "$lock_cargo" ]; then
	awk -v new="$new" -v crate="$lock_cargo_crate" '
		$0 == "name = \"" crate "\"" { hit=1; print; next }
		hit && /^version = "/ { print "version = \"" new "\""; hit=0; next }
		{print}
	' "$lock_cargo" > "$lock_cargo.tmp" && mv "$lock_cargo.tmp" "$lock_cargo"
	if ! grep -A1 "^name = \"$lock_cargo_crate\"" "$lock_cargo" | grep -q "^version = \"$new\""; then
		echo "✗ Failed to update version in $lock_cargo. Reverting." >&2
		git checkout -- "$version_file" "$manifest_toml" "$lock_cargo"
		exit 1
	fi
fi

touched="$manifest $manifest_pom $manifest_toml $lock_npm $lock_cargo"

git --no-pager diff --stat -- "$version_file" $touched
echo ""
if [ "${RELEASE_DRY_RUN:-}" = "1" ]; then
	git --no-pager diff -U0 -- "$version_file" $touched
	git checkout -- "$version_file" $touched
	echo "Dry run: would commit '$label v$new', tag $prefix/v$new, and push. Reverted."
	exit 0
fi
read -r -p "Commit '$label v$new', tag $prefix/v$new, and push? [y/N] " confirm || confirm="n"
case "$confirm" in
	y|Y|yes|YES) ;;
	*) echo "Aborted. Reverting."; git checkout -- "$version_file" $touched; exit 1 ;;
esac

git add -- "$version_file" $touched
git commit -m "$label v$new"
git tag "$prefix/v$new"
git push origin HEAD "$prefix/v$new"

echo ""
echo "✓ Released $label v$new"
if [ "$kind" = "fn-go" ]; then
	echo "  Consume:  go get github.com/flowcatalyst/flowcatalyst-go/clients/fn-go@v$new (module proxy; no workflow)"
else
	echo "  Workflow: https://github.com/flowcatalyst/flowcatalyst-go/actions/workflows/${workflow:-release-$prefix.yml}"
fi
echo "  Release:  https://github.com/flowcatalyst/flowcatalyst-go/releases/tag/$prefix/v$new"
