#!/usr/bin/env bash
# Release every SDK line that has changed since its last tag, one after another.
#
# Usage: scripts/release-all-sdks.sh [patch|minor|major]      (default: patch)
#
#   BUMP semantics: the bump applies to every line that is released.
#   ONLY="fn-ts fn-go"        restrict to these lines (names as in release.sh)
#   SKIP="java laravel"       leave these lines out
#   WITH_DEV=1                also release fcdev (cmd/fcdev), off by default
#   RELEASE_DRY_RUN=1         run every check per line, change and push nothing
#   RELEASE_ALL_PLAN=1        only print the plan (no checks that need a clean tree)
#   RELEASE_ALL_YES=1         do not ask for the single up-front confirmation
#
# Lines: ts laravel java go-sdk fn-ts fn-rust fn-go (+ dev). A line is released
# only if its directory changed since its highest <prefix>/v* tag (VERSION and
# manifests are ignored: they change in the release commit itself). A line that
# has never been released (VERSION 0.0.0, no tag) is skipped unless it is named in
# ONLY; it then gets 0.1.0, because a bare bump would produce 0.0.1.
#
# Each line is cut by scripts/release.sh (its own checks, commit, tag and push);
# this script only decides which lines, in what order, and asks once.
# NOTE: release.sh pushes HEAD, so every unpushed commit on the current branch
# goes up with the first release. Push or review those first.
set -euo pipefail

bump="${1:-patch}"
case "$bump" in patch|minor|major) ;; *) echo "usage: $0 [patch|minor|major]" >&2; exit 2 ;; esac

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

# kind | tag prefix | directory watched for changes | version file
lines=(
	"ts|typescript-sdk|clients/typescript-sdk|clients/typescript-sdk/VERSION"
	"laravel|laravel-sdk|clients/laravel-sdk|clients/laravel-sdk/VERSION"
	"java|java-sdk|clients/java-sdk|clients/java-sdk/VERSION"
	"go-sdk|go-sdk|pkg/fcsdk|pkg/fcsdk/VERSION"
	"fn-ts|fn-ts-sdk|clients/fn-ts|clients/fn-ts/VERSION"
	"fn-rust|fn-rust-sdk|clients/fn-rust|clients/fn-rust/VERSION"
	"fn-go|clients/fn-go|clients/fn-go|clients/fn-go/VERSION"
)
if [ "${WITH_DEV:-}" = "1" ]; then
	lines+=("dev|fcdev|cmd/fcdev|cmd/fcdev/VERSION")
fi

in_list() { local x="$1"; shift; for i in "$@"; do [ "$i" = "$x" ] && return 0; done; return 1; }
read -r -a only <<< "${ONLY:-}"
read -r -a skip <<< "${SKIP:-}"

plan=()   # "kind|bump-or-version|reason"
echo "Release plan (bump: $bump):"
for entry in "${lines[@]}"; do
	IFS='|' read -r kind prefix dir vfile <<< "$entry"
	if [ "${#only[@]}" -gt 0 ] && ! in_list "$kind" "${only[@]}"; then continue; fi
	if [ "${#skip[@]}" -gt 0 ] && in_list "$kind" "${skip[@]}"; then
		printf '  %-8s skipped (SKIP)\n' "$kind"; continue
	fi
	last="$(git tag --list "$prefix/v*" | sed "s|^$prefix/v||" \
		| awk -F. '/^[0-9]+\.[0-9]+\.[0-9]+$/ { printf "%010d%010d%010d %s\n", $1, $2, $3, $0 }' \
		| sort -r | awk 'NR==1 {print $2}' | sed "s|^\(.\)|$prefix/v\1|")" || true
	file_v="$(tr -d '[:space:]' < "$vfile")"
	if [ -z "$last" ]; then
		if [ "$file_v" = "0.0.0" ]; then
			if [ "${#only[@]}" -gt 0 ] && in_list "$kind" "${only[@]}"; then
				printf '  %-8s FIRST release -> 0.1.0\n' "$kind"
				plan+=("$kind|0.1.0")
			else
				printf '  %-8s never released: skipped (name it in ONLY to cut 0.1.0)\n' "$kind"
			fi
			continue
		fi
		printf '  %-8s no tag yet, VERSION %s: releasing %s\n' "$kind" "$file_v" "$bump"
		plan+=("$kind|$bump")
		continue
	fi
	if git diff --quiet "$last" HEAD -- "$dir" ":(exclude)$vfile"; then
		printf '  %-8s unchanged since %s: skipped\n' "$kind" "$last"
		continue
	fi
	n="$(git rev-list --count "$last"..HEAD -- "$dir")"
	printf '  %-8s %s commit(s) since %s -> %s\n' "$kind" "$n" "$last" "$bump"
	plan+=("$kind|$bump")
done

if [ "${#plan[@]}" -eq 0 ]; then
	echo "Nothing to release."
	exit 0
fi
[ "${RELEASE_ALL_PLAN:-}" = "1" ] && exit 0

if [ -n "$(git status --porcelain)" ]; then
	echo "✗ Working tree is dirty. Commit or stash first." >&2
	exit 1
fi

# The generated SDKs (ts, laravel, java) are built from the OpenAPI spec. Refuse to
# release them from a stale spec.
for entry in "${plan[@]}"; do
	k="${entry%%|*}"
	if [ "$k" = ts ] || [ "$k" = laravel ] || [ "$k" = java ]; then
		echo "Checking the SDK OpenAPI inputs are current (make sdk-spec)..."
		make --no-print-directory sdk-spec >/dev/null
		if [ -n "$(git status --porcelain)" ]; then
			echo "✗ make sdk-spec changed files: the generated SDKs are stale. Run make sdk-generate, review, commit, then retry." >&2
			git status --short
			exit 1
		fi
		break
	fi
done

if [ "${RELEASE_DRY_RUN:-}" != "1" ] && [ "${RELEASE_ALL_YES:-}" != "1" ]; then
	unpushed="$(git rev-list --count '@{u}..HEAD' 2>/dev/null || echo '?')"
	read -r -p "Release ${#plan[@]} line(s), pushing HEAD ($unpushed unpushed commit(s)) and one tag each? [y/N] " ok || ok=n
	case "$ok" in y|Y|yes|YES) ;; *) echo "Aborted."; exit 1 ;; esac
fi

done_lines=()
for entry in "${plan[@]}"; do
	IFS='|' read -r kind b <<< "$entry"
	echo ""
	echo "=== $kind ($b) ==="
	if ! RELEASE_ASSUME_YES=1 scripts/release.sh "$kind" "$b"; then
		echo "✗ Release of $kind failed. Released so far: ${done_lines[*]:-none}. Fix it, then rerun with ONLY set to the remaining lines." >&2
		exit 1
	fi
	done_lines+=("$kind")
done
echo ""
echo "✓ Released: ${done_lines[*]}"
