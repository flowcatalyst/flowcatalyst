#!/usr/bin/env bash
# Consume the Go SDK (pkg/fcsdk) the way an application does, from a scratch
# module outside this repository. Two modes, both run by default:
#
#   replace  a scratch module that requires the SDK through a local `replace`
#            (what an application developing against a checkout does);
#   vcs      `go get <module>@v0.0.0-smoke` resolved through git, as the module
#            proxy resolves it: scripts/publish-go-sdk.sh publishes HEAD as
#            v0.0.0-smoke to a throwaway bare repository and git's
#            url.insteadOf points https://github.com/flowcatalyst/go-sdk at it.
#            This checks that what a release publishes is fetchable under the
#            module path (a mismatch makes a published tag permanently
#            unfetchable). Commits only: uncommitted changes are not in HEAD.
#
# Usage: scripts/go-sdk-smoke.sh [replace|vcs|all]
# Nothing is tagged or pushed in this repository; everything lives in a temp dir.
set -euo pipefail

mode="${1:-all}"
module="github.com/flowcatalyst/go-sdk"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

write_consumer() {
	local dir="$1"
	mkdir -p "$dir"
	cat > "$dir/go.mod" <<'EOF'
module example.com/fcsdk-smoke

go 1.26.0
EOF
	# Touches the packages with third-party dependencies (pgx, jwx, go-redis)
	# as well as the pure ones, so a missing require in the SDK's go.mod fails.
	cat > "$dir/main.go" <<'EOF'
package main

import (
	"fmt"

	"github.com/flowcatalyst/go-sdk/auth"
	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/lock/redislock"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/sync"
	"github.com/flowcatalyst/go-sdk/tsid"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

var (
	_ = auth.NewTokenValidator
	_ = redislock.New
	_ = usecaseop.Public[struct{}]
)

func main() {
	c := client.New("http://localhost:8080")
	set := sync.ForApplication("smoke")
	fmt.Println(c != nil, set.ApplicationCode, len(tsid.GenerateUntyped()), outboxpgx.DefaultConfig().TableName)
}
EOF
}

run_replace() {
	echo ">> replace: scratch module requiring $module via a local replace"
	local dir="$work/replace"
	write_consumer "$dir"
	(
		cd "$dir"
		go mod edit -require="$module@v0.0.0-00010101000000-000000000000" -replace="$module=$repo_root/pkg/fcsdk"
		GOFLAGS=-mod=mod go mod tidy
		go vet ./...
		go build -o smoke . && ./smoke
	)
}

run_vcs() {
	echo ">> vcs: go get $module@v0.0.0-smoke through git (HEAD published to a throwaway repository)"
	git init -q --bare "$work/repo"
	GIT_AUTHOR_NAME=smoke GIT_AUTHOR_EMAIL=smoke@example.invalid \
		GIT_COMMITTER_NAME=smoke GIT_COMMITTER_EMAIL=smoke@example.invalid \
		"$repo_root/scripts/publish-go-sdk.sh" HEAD 0.0.0-smoke "$work/repo"
	cat > "$work/gitconfig" <<EOF
[url "file://$work/repo"]
	insteadOf = https://github.com/flowcatalyst/go-sdk
[protocol "file"]
	allow = always
EOF
	local dir="$work/vcs"
	write_consumer "$dir"
	(
		cd "$dir"
		# Only the SDK goes direct (GOPRIVATE); its dependencies still come
		# from GOPROXY. A fresh module cache proves the SDK was fetched, not
		# reused.
		export GIT_CONFIG_GLOBAL="$work/gitconfig" GIT_ALLOW_PROTOCOL=file:https \
			GOMODCACHE="$work/modcache" GOPRIVATE="$module" GOFLAGS=-mod=mod
		go get "$module@v0.0.0-smoke"
		go mod tidy
		test "$(go list -m -f '{{.Version}}' "$module")" = "v0.0.0-smoke"
		sdk_dir="$(go list -m -f '{{.Dir}}' "$module")"
		for f in go.mod LICENSE NOTICE README.md; do
			test -f "$sdk_dir/$f" || { echo "✗ module zip lacks $f" >&2; exit 1; }
		done
		# The tests module (tests/, embedded Postgres) is a nested
		# module of its own, so it is not in the SDK's zip.
		test ! -e "$sdk_dir/tests" || { echo "✗ module zip contains the nested tests module" >&2; exit 1; }
		go vet ./...
		go build -o smoke . && ./smoke
	)
}

case "$mode" in
	replace) run_replace ;;
	vcs) run_vcs ;;
	all) run_replace; run_vcs ;;
	*) echo "usage: scripts/go-sdk-smoke.sh [replace|vcs|all]" >&2; exit 2 ;;
esac
echo "✓ go-sdk smoke ($mode) passed"
