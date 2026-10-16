#!/usr/bin/env bash
# Fast pre-push gate: `make pre-push`. No Docker or Postgres.
#
# Runs, in order, stopping at the first failure and saying which step failed:
#   1 tidy      go mod tidy -diff for the root, pkg/fcsdk and pkg/fcsdk/tests modules
#   2 build     go build ./... (root + both SDK modules)
#   3 vet       go vet ./... (all three modules)
#   4 lint      the PINNED golangci-lint (make lint: root, SDK, SDK tests module)
#   5 templ     make ui-verify (generated *_templ.go are current)
#   6 analyze   make analyze (UoW seal + id-conversion analyzers)
#   7 sdk       Go SDK module tests (includes pkg/fcsdk/client OpenAPI lock conformance)
#   8 spec      tools/dump-spec tests + api-diff (committed lock == live spec)
#   9 sqlc      make sqlc-verify (pinned sqlc output is current)
#  10 tests     go test -short for packages changed vs origin/main (all if the base is unknown)
#  11 types     make frontend-types-verify (SPA generated types == lock); needs frontend/node_modules
#
# Never pushes and never edits tracked files, except regenerating output that the
# verify steps diff (sqlc/templ/openapi types), which is exactly what they report.
# Bypass: SKIP_PREPUSH=1 git push   or   git push --no-verify
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
MAKE="${MAKE:-make} --no-print-directory"
GO="${GO:-go}"

if [ "${SKIP_PREPUSH:-}" = "1" ]; then
	echo "pre-push: SKIP_PREPUSH=1, gate skipped"
	exit 0
fi

mkdir -p tmp
if [ -z "$(ls -A frontend/dist 2>/dev/null)" ]; then
	mkdir -p frontend/dist && touch frontend/dist/.gitkeep
	echo "pre-push: frontend/dist was empty, wrote a .gitkeep stub so the embed typechecks"
fi

start_all=$SECONDS
step=""
fix=""
run() { # run <name> <fix/rerun hint> <cmd...>
	step="$1"; fix="$2"; shift 2
	echo "== pre-push [$step]"
	local t=$SECONDS
	if ! "$@"; then
		echo
		echo "pre-push FAILED at step: $step"
		echo "  rerun just this step: $fix"
		echo "  rerun the whole gate: make pre-push"
		echo "  bypass (not recommended): SKIP_PREPUSH=1 git push   |   git push --no-verify"
		exit 1
	fi
	echo "   ok ($((SECONDS - t))s)"
}

run tidy    "make tidy-verify  (fix: go mod tidy in the failing module)" $MAKE tidy-verify
run build   "go build ./... && go -C pkg/fcsdk build ./..." bash -c "$GO build ./... && $GO -C pkg/fcsdk build ./..."
run vet     "make vet" $MAKE vet
run lint    "make lint  (autofix some: make lint-fix)" $MAKE lint
run templ   "make ui-verify  (fix: make ui-generate)" $MAKE ui-verify
run analyze "make analyze" $MAKE analyze
run sdk     "make test-go-sdk" $GO -C pkg/fcsdk test -short ./...
run spec    "go test ./tools/dump-spec/... && make api-diff  (fix: make api-bump)" bash -c "$GO test ./tools/dump-spec/... && $MAKE api-diff"
run sqlc    "make sqlc-verify  (fix: make sqlc)" $MAKE sqlc-verify

# Changed packages: .go files differing from the merge-base with origin/main
# (committed + working tree), mapped to package directories.
changed_tests() {
	local base pkgs
	base="$(git merge-base HEAD origin/main 2>/dev/null || true)"
	if [ -z "$base" ]; then
		echo "pre-push: no origin/main found; testing ./... (-short)"
		$GO test -short ./...
		return
	fi
	files="$( { git diff --name-only "$base" -- '*.go' go.mod go.sum; git ls-files --others --exclude-standard -- '*.go'; } | sort -u)"
	if echo "$files" | grep -qx -e go.mod -e go.sum; then
		echo "pre-push: go.mod/go.sum changed; testing ./... (-short)"
		$GO test -short ./...
		return
	fi
	pkgs=""
	for f in $files; do
		case "$f" in pkg/fcsdk/*|clients/*) continue ;; esac
		[ -f "$f" ] && pkgs="$pkgs
$(dirname "$f")"
	done
	pkgs="$(echo "$pkgs" | sed '/^$/d' | sort -u)"
	if [ -z "$pkgs" ]; then
		echo "pre-push: no changed root-module Go packages vs origin/main"
		return
	fi
	echo "pre-push: changed packages:"; echo "$pkgs" | sed 's/^/  /'
	# shellcheck disable=SC2086
	$GO test -short $(echo "$pkgs" | sed 's#^#./#')
}
run tests "go test -short <changed packages>  (all: make test-unit)" changed_tests

if [ -d frontend/node_modules ]; then
	run types "make frontend-types-verify  (fix: cd frontend && pnpm api:generate)" $MAKE frontend-types-verify
else
	echo "== pre-push [types] SKIPPED: frontend/node_modules missing (run: make frontend-install). CI still checks the SPA build."
fi

echo
echo "pre-push: all steps passed in $((SECONDS - start_all))s"
