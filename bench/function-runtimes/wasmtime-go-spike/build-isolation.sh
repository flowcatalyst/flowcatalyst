#!/bin/sh
# Question 6: what adding wasmtime-go behind a build tag does to the main module.
# Works on a copy of the worktree's HEAD in the scratch directory; the worktree is untouched.
set -eu
SC=$1; WT=$2
R=$SC/rootcopy
rm -rf "$R" && mkdir -p "$R"
git -C "$WT" archive HEAD | tar -x -C "$R"
mkdir -p "$R/frontend/dist" && echo '<!doctype html>' > "$R/frontend/dist/index.html"
cd "$R"
cp go.mod go.mod.orig; cp go.sum go.sum.orig
cat > internal/functions/engine/wasmtime_probe.go <<'GO'
//go:build wasmtime

package engine

import wasmtime "github.com/bytecodealliance/wasmtime-go/v50"

// The phase-4 shape: a wasmtime file in the engine package, behind the tag.
var _ = wasmtime.NewEngine
GO
export GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go get github.com/bytecodealliance/wasmtime-go/v50@v50.0.0 2>&1 | tail -2
go mod tidy 2>&1 | tail -2
echo "== go.mod diff"; diff go.mod.orig go.mod || true
echo "== go.sum diff"; diff go.sum.orig go.sum || true
unset GOFLAGS
echo "== CGO_ENABLED=0 go build ./cmd/fcdev ./cmd/fc-server (no tag)"
CGO_ENABLED=0 go build -o /dev/null ./cmd/fcdev && CGO_ENABLED=0 go build -o /dev/null ./cmd/fc-server && echo OK
echo "== deps of fcdev/fc-server mentioning wasmtime-go (no tag)"; go list -deps ./cmd/fcdev ./cmd/fc-server | grep -c wasmtime-go || true
echo "== with the tag and CGO_ENABLED=0"; CGO_ENABLED=0 go build -tags wasmtime -o /dev/null ./cmd/fcdev 2>&1 | tail -2 || true
echo "== with the tag and cgo (go1.27.2)"; GOTOOLCHAIN=go1.27.2 go build -tags wasmtime -o /dev/null ./cmd/fcdev && echo OK
echo "== Go 1.27.4 with the tag"; go build -tags wasmtime -o /dev/null ./cmd/fcdev 2>&1 | tail -1 || true
echo "== fresh module cache, local file proxy: what each command downloads"
for cmd in "go build -o /dev/null ./cmd/fcdev" "go vet ./internal/functions/..." "go test -count=1 -run XXX ./internal/functions/engine" "go mod download" "go mod tidy"; do
  F=$SC/modcache-fresh; chmod -R u+w "$F" 2>/dev/null || true; rm -rf "$F"
  GOMODCACHE=$F GOPROXY=file://$HOME/go/pkg/mod/cache/download GOSUMDB=off CGO_ENABLED=0 sh -c "$cmd" >/dev/null 2>&1 || echo "  ($cmd failed)"
  z=$(ls "$F/cache/download/github.com/bytecodealliance/wasmtime-go/v50/@v/" 2>/dev/null | tr '\n' ' ')
  echo "  $cmd -> wasmtime-go files fetched: [${z}] total cache $(du -sh "$F" | cut -f1)"
done
chmod -R u+w "$SC/modcache-fresh" 2>/dev/null || true; rm -rf "$SC/modcache-fresh"
