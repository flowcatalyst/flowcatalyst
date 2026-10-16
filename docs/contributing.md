# Contributing: before you push

CI failures here have mostly been avoidable: a tool resolved to `latest` and
grew new findings, or a conformance check (OpenAPI lock, generated code) was
never run locally. The rule now is that **everything CI runs is reproducible
locally with the same pinned tools**.

## One-time setup

```sh
make install-tools   # builds pinned golangci-lint + sqlc into ./.tools with YOUR Go
make install-hooks   # git config core.hooksPath scripts/hooks  (pre-push runs `make pre-push`)
```

`make lint` / `make sqlc-verify` use the pinned binaries in `./.tools`
(never whatever is on `PATH`) and build them on first use. They are built with
the local Go toolchain because prebuilt golangci-lint releases lag the Go
release and cannot read newer stdlib export data. A Go upgrade rebuilds them
automatically (the directory name carries the Go version).

## Before every push: `make pre-push` (about a minute, no Docker)

Runs automatically from the git hook. In order, stopping at the first failure
and naming it (with the command to rerun just that step):

| Step | What |
|------|------|
| tidy | `go mod tidy -diff` for the root, `pkg/fcsdk`, `pkg/fcsdk/tests` modules |
| build | `go build ./...` (root + Go SDK) |
| vet | `go vet ./...` on all three modules |
| lint | pinned golangci-lint, incl. `-tags integration`, root + SDK modules |
| templ | generated `*_templ.go` current (`make ui-verify`) |
| analyze | UoW seal + id-conversion analyzers |
| sdk | Go SDK module tests (includes the `pkg/fcsdk/client` OpenAPI lock conformance test) |
| spec | `tools/dump-spec` tests and `make api-diff` (committed lock == live spec) |
| sqlc | `make sqlc-verify` with the pinned sqlc |
| tests | `go test -short` for packages changed vs `origin/main` (all of `./...` if `go.mod` changed or there is no `origin/main`) |
| types | `make frontend-types-verify` (skipped, loudly, without `frontend/node_modules`) |

Bypass in an emergency: `git push --no-verify` or `SKIP_PREPUSH=1 git push`.
The hook only runs checks; it never pushes. The verify steps regenerate output
into the working tree to diff it (sqlc, templ, SPA types); if one fails,
the fix is the regenerate command it prints, then commit the diff.

## Full CI mirror: `make ci`

Same steps, same order, same pinned tools as `.github/workflows/ci.yml` and
`go-sdk.yml`: tidy, templ, build, unit tests, integration tests (embedded
Postgres, no Docker; also the Go SDK's `pkg/fcsdk/tests` module), analyze,
lint, Go SDK build/vet/race tests, Go SDK scratch-module smoke, sqlc, OpenAPI
lock, SPA types. Unit tests are `make test-unit` (`-short`, no DB); integration
tests are `make test-integration` (`-tags=integration`, `*_pg_test.go`).
`make build` writes `bin/`; use `make ci BIN_DIR=/some/scratch` to keep your
`bin/` untouched.

Not reproducible locally: the `go-sdk.yml` matrix leg on Go 1.26 (only the local
toolchain is available).

## Bumping a pin

Tool versions live in the Makefile (`GOLANGCI_LINT_VERSION`, `SQLC_VERSION`,
`AIR_VERSION`, `GOTESTSUM_VERSION`). golangci-lint's version is repeated as
`version:` in `ci.yml` and `go-sdk.yml`, and the Go toolchain as `go-version`
(`1.27.2`) in the workflows. Change them together, and fix any new findings in
the same commit.
