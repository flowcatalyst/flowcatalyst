# Go modernization plan — hand-rolled code over language/stdlib features

_Written 2026-09-05 against `9f7be62` (branch `feat/owner-rulings`). Module is `go 1.26`,
local toolchain go1.27.1. Companion to [`router-go-idiom-plan.md`](./router-go-idiom-plan.md),
which covers design-shape issues in `internal/router`; this document covers the narrower
"someone wrote by hand what the language or stdlib already provides" class across the
whole repo._

## 1. TL;DR

1. **The seed example is real and representative.** `internal/router/error.go:119`
   (`AsRouterError`) is a hand-rolled `errors.As`. The same instinct shows up ~330 more
   times: manual contains-loops, per-package `ptr[T]` helpers, `sort.Strings` where
   `slices.Sort` exists, `append([]T(nil), s...)` instead of `slices.Clone`, C-style
   counted loops, `wg.Add(1)/go/defer wg.Done()` triplets, and `sync.Once` + package
   variable pairs.
2. **About 60% is fixable by a tool in one commit.** Go 1.26's `go fix` now runs the
   `modernize` analyzers; a dry run touches 95 files (unit + integration tags). Every
   rewrite is semantics-preserving by construction.
3. **The rest is a short hand-edit list** (§4), each item small, plus two design-level
   options (§5) that need an owner decision because they change behaviour or test
   architecture.
4. **The guard rail is the point.** None of this is a bug; it accumulated because nothing
   flagged it. `errorlint` (which catches the `AsRouterError` pattern directly) and
   `modernize` are both available in the already-installed golangci-lint v2 and are not
   enabled. Switch them on first (§3) so the class cannot come back.
5. **Bump to Go 1.27 first (owner decision 2026-09-05).** `go build`/`go vet` already pass on
   the 1.27.1 toolchain. The bump is five files plus a golangci-lint caveat (§3 step 0) and
   unlocks three more `go fix` rewrites (generic methods are not used here; the visible gain
   is the 1.27 struct-literal-keys fixer flattening `Outer{inner: inner{…}}` literals).
6. **Local `make lint` is currently broken** on this machine: the Homebrew golangci-lint
   is built with go1.26.3 and panics loading the go1.27 stdlib
   (`file requires newer Go version go1.27`). CI is unaffected (setup-go 1.26). Fix is
   `make install-tools` (rebuilds against the local toolchain) or
   `GOTOOLCHAIN=go1.26.3 golangci-lint run`.

## 2. Findings inventory

Sources: `go fix -diff ./...`, `GOTOOLCHAIN=go1.26.3 golangci-lint run --default=none
-E errorlint,modernize --build-tags integration ./...`, and targeted greps for patterns
the analyzers do not cover. Counts exclude `internal/db/gen` and sqlc output.

### 2.1 Auto-fixable by `go fix` (modernize analyzers)

| Analyzer | Count | What it rewrites | Needs Go |
|---|---:|---|---|
| `newexpr` | 200 | `ptr(x)`/`strptr(x)`/`strp(x)`/`ptrStr(x)`/`reason(x)`/`intPtr(x)`/`scopePtr(x)`/`ptrInt32(x)`/`ptrU32(x)` → `new(x)`; then the 10 helper definitions become dead | 1.26 |
| `rangeint` | 29 | `for i := 0; i < n; i++` → `for i := range n` | 1.22 |
| `slicescontains` | 26 | manual `for … if x == y { return true }` → `slices.Contains` / `ContainsFunc` | 1.21 |
| `minmax` | 21 | `if a > b { a = b }` → `min`/`max` builtins | 1.21 |
| `waitgroupgo` | 14 | `wg.Add(1); go func(){ defer wg.Done(); … }()` → `wg.Go(func(){…})` | 1.25 |
| `stringsseq` | 9 | `range strings.Split(…)` → `range strings.SplitSeq(…)` (+2 `FieldsSeq`) | 1.24 |
| `forvar` | 6 | delete `x := x` loop-var copies | 1.22 |
| `any` | 6 | `interface{}` → `any` (sqlc output excluded) | 1.18 |
| `stringscut` / `stringscutprefix` | 6 | `IndexByte`+slicing → `strings.Cut`; `HasPrefix`+`TrimPrefix` → `CutPrefix` | 1.18/1.20 |
| `errorsastype` | 3 | `var e *T; errors.As(err,&e)` → `e, ok := errors.AsType[*T](err)` | 1.26 |
| `stringsbuilder` | 2 | `s += …` in a loop → `strings.Builder` | — |
| `atomictypes` | 2 | `atomic.AddInt32(&x, …)` on a plain field → `atomic.Int32` | 1.19 |
| `slicessort`, `slicesbackward`, `mapsloop` | 3 | `sort.Slice` → `slices.SortFunc`; backward index loop → `slices.Backward`; `m[k]=v` copy loop → `maps.Copy` | 1.21–1.23 |

Total ≈ 330 sites, 95 files at `go 1.26`; 98 files at `go 1.27`, where the new
struct-literal-keys language feature lets `go fix` flatten embedded-struct literals in
`scheduledjob/operations/ops.go`, `loginbackoff_test.go` and
`router/manager_stopped_consumer_test.go`. By area: `internal/platform/principal` 65, `internal/router` 49,
`internal/platform/auth` 35, `subscription` 25, `shared` 24; 199 of the sites are in tests
(mostly the `ptr(x)` calls in `ops_pg_test.go` files).

### 2.2 `errorlint` findings (not enabled today) — 20

| Kind | Count | Where |
|---|---:|---|
| `fmt.Errorf("… %v", err)` should be `%w` | 13 | `pkg/fcsdk/cache/{postgrescache,rediscache}`, `pkg/fcsdk/lock/{postgreslock,redislock}`, `authservice.go:615` |
| `err == redis.Nil` should be `errors.Is` | 1 | `internal/platform/shared/versioncache/store.go:112` (real: go-redis wraps in some paths) |
| `err != ErrMismatch` in tests | 4 | `passwordhash_test.go` |
| type assertion on `error` | 2 | **`internal/router/error.go:125`** (the seed example), `pkg/fcsdk/scheduledjobs/runner_test.go:223` |

The 13 `%v` sites in `pkg/fcsdk` are SDK-facing: callers of the cache/lock packages cannot
`errors.Is` through them today. That is the only item in this document that changes
observable behaviour for the better rather than just tidying.

### 2.3 Hand-rolled helpers the analyzers do not flag

| Pattern | Count | Sites | Replacement |
|---|---:|---|---|
| Manual unwrap loop | 1 | `internal/router/error.go:119` `AsRouterError` | `errors.AsType[*RouterError]` (4 lines) |
| Named contains helpers | 7 | `clientselection.go:276 contains`, `sessiontoken.go:240 containsString`, `login/twofactor.go:519,538`, `openapispecs/diff.go:203`, `developer_credential.go:29 hasRole`, `router/manager_detach_test.go:221 containsPool` | delete; `slices.Contains` / `slices.ContainsFunc` at call sites |
| Collect map keys then `sort.Strings` | 5 | `openapispecs/diff.go:49,158`, `shared/sdk/dispatch_job_create.go:163`, `bff/filter_options.go:113`, `bff/event_types.go:371` (+ test `keysOf`) | `slices.Sorted(maps.Keys(m))` **only where the slice is iterated internally** (diff.go:49, dispatch_job_create, router/manager). `slices.Sorted` of an empty map is nil; the three JSON-facing sites rely on `make(…, 0)` to emit `[]` not `null`. Left as is. |
| `sort.Strings` / `sort.Slice` | 33 | listed by `grep -rn 'sort\.\(Strings\|Slice\)'` | `slices.Sort` / `slices.SortFunc(xs, func(a,b) int { return strings.Compare(a.Name, b.Name) })` |
| `append([]T(nil), s...)` clone idiom | 50 | mostly router tests; `role/operations/sync.go`, entity copy paths | **Not a drop-in, left as is.** `slices.Clone` preserves the input's nilness (`append(s[:0:0], s...)`), whereas this idiom normalises an empty non-nil input to nil. Callers such as `role/repository.go` visibly care about nil vs empty for persistence and JSON, so the swap would be a silent semantic change. |
| Dedupe via `map[string]struct{}` + append | 3 | `principal/api/api.go:937 dedupeStrings`, `:1767 uniqueRoleNames`, `bff/event_types.go:361 distinctOptions` | `slices.Sorted(slices.Values(…))` + `slices.Compact`, or keep the map where order is irrelevant |
| `sync.Once` + package var pair | 3 | `passwordhash.go:77 dummyHash`, `docsapi/api.go:90 publishedIndex`, `pkg/fcsdk/auth/jwks.go:41` | `sync.OnceValue` / `sync.OnceValues` |
| `if x == "" { x = default }` | 48 | e.g. `authservice.go:220,223`, `seed/admin.go:68`, `oauthapi/authorize.go:171` | `cmp.Or(x, default)` where the fallback is a plain expression; leave the ones whose comment explains a legacy fallback |
| `strings.SplitN(s, sep, 2)` + `len(parts)` check | 3 | `bff/filter_options.go:103`, `loginattempt.go:489`, `pkg/fcsdk/tsid/tsid.go:124` | `strings.Cut` |
| `bytes.Buffer` used only to build a string | 3 | `mfa/crypto.go:69`, `audit/api/dto.go:37`, `mcp/tools.go:299` | **On inspection, none apply**: all three feed `png.Encode`, `json.Compact`, `json.Indent`, which need a byte writer. Left as is. |
| `HasSuffix` + `TrimSuffix` | 2 | `oauthapi/redirecturi.go:130`, `queue/nats/nats.go:457` | **Left as is**: redirecturi trims only the `*` to keep the slash; nats trims two alternative suffixes. `CutSuffix` fits neither. |
| `log.Printf` alongside `slog` | 3 | — | `slog` (skip if in a `cmd/` tool that intentionally prints) |

### 2.4 Test-only idioms

| Pattern | Count | Replacement | Gotcha |
|---|---:|---|---|
| `ctx := context.Background()` at top of a `Test…` func | 288 | `t.Context()` (1.24) | **Deferred** (2026-09-05): `t.Context()` is cancelled **before** `t.Cleanup` runs, and three Cleanup bodies use the test ctx for DB teardown. The gain is cosmetic; the sweep is 288 sites plus a full integration run per package. Do it opportunistically when touching a test file, not as a sweep. |
| `_ = os.Setenv("FLOWCATALYST_APP_KEY", …)` | 5 | `t.Setenv` | **Not applicable**: all five are in `TestMain`, where there is no `*testing.T`. Left as is. |
| `atomic.AddInt32(&remaining, …)` on locals | 10 | `atomic.Int32` | all in `outbox/group_distributor_test.go` |
| `time.Sleep`-driven concurrency tests | 50 | `testing/synctest` (1.25) | see §5.2 — architecture change, not a sweep |

### 2.5 Things checked and found clean

No `io/ioutil`, no `math/rand` v1, no `strings.Title`, no `time.Now().Sub`, no
`strings.Replace(…, -1)`, no `sort.Interface` implementations, no channel-based generators
that want `iter.Seq`, no `reflect.TypeOf((*T)(nil)).Elem()`, no `http.NewRequest` without
context, no `errors.New(fmt.Sprintf(…))`. `errors.Is` is already used 110 times; the
codebase is not error-naive, it just predates `slices`/`maps` (3 and 0 imports respectively).

## 3. Phase 0 — Go 1.27 bump + guard rails (do first, two commits)

0. **Bump to Go 1.27.** `go.mod` `go 1.26` → `go 1.27`; `.github/workflows/ci.yml`
   `go-version: "1.26"` ×2 → `"1.27"`; `Dockerfile` `golang:1.26-alpine` → `1.27-alpine`;
   `README.md:292`. `release-fcdev.yml` reads `go-version-file: go.mod` and follows. Then
   `go mod tidy` (1.27 merges the require blocks into direct + indirect; 3-line diff, no
   `go.sum` change) and `make ci`. Two things to watch:
   - **golangci-lint.** Upstream latest (v2.13.2, 2026-08-27) is still built with go1.26
     and panics type-checking the 1.27 stdlib — the same failure `make lint` has locally
     today. In CI, switch `golangci-lint-action@v7` to `install-mode: goinstall` so it is
     compiled with the workflow's Go 1.27; locally, `make install-tools`. Revisit once a
     go1.27-built release exists.
   - **`encoding/json` is now backed by the v2 implementation.** v1 semantics are kept but
     error messages can differ. No test in the repo asserts JSON error text, and
     `make api-diff` guards the spec. Run `make test-integration` too, since the wire DTO
     round-trips live there.
1. **Enable `errorlint` and `modernize` in `.golangci.yml`.** Add both under
   `linters.enable`. Keep `modernize` at its default analyzer set; the only one worth
   disabling is `stringsseq` if the 9 `SplitSeq` rewrites are judged noise (they are
   correct but the gain is negligible on short strings). Fix the 20 errorlint findings in
   the same commit so lint is green.
2. **Fix local lint.** `make lint` panics on this machine because the prebuilt
   golangci-lint (go1.26.3) cannot type-check the go1.27.1 stdlib. `make install-tools`
   rebuilds it with the local toolchain. Optionally pin the Makefile `lint` target with
   `GOTOOLCHAIN=$(shell awk '/^go /{print "go"$$2".0"}' go.mod)` so the linter always sees
   the module's declared Go version regardless of what is installed. CI (`setup-go 1.26`
   + `golangci-lint-action@v7 latest`) does not need changing.
3. **Add `go fix` as a drift check.** A `modernize-check` target
   (`go fix -diff ./... && go fix -tags integration -diff ./...`, failing on non-empty
   output) folded into `make ci` catches anything golangci's bundled (older) modernize
   misses. Cheap, and it is the same tool that will do the Phase 1 rewrite.

Verification: `make lint` green, `make ci` green.

## 4. Phase 1 — mechanical rewrite (one commit)

```
go fix ./...
go fix -tags integration ./...
gofumpt -w . && goimports -w .        # go fix leaves import blocks in need of a tidy
```

Then by hand in the same commit:

- Delete the now-unused pointer helpers (`ptr`, `strptr`, `strp`, `ptrStr`, `intPtr`,
  `ptrInt32`, `ptrU32`, `scopePtr`, `reason`, `timePtr`) — `unused` will list them.
  `internal/platform/seed/roles.go` and `internal/router/traffic.go` keep theirs only if
  they are still called after inlining.
- Rewrite `AsRouterError` (`internal/router/error.go:119`) to `errors.AsType`.
- Fix `runner_test.go:223` (`err.(*RunnerError)` → `errors.AsType`).

Verification: `make ci`, plus `make test-integration` (the `-tags integration` rewrite is
not covered by `make test`). `make api-diff` must report no change — none of these touch
wire types, but the `newexpr` rewrite runs through DTO construction in tests and the check
is free.

Risk: low. Every `go fix` rewrite is an AST-level equivalence. The two places to read the
diff rather than trust it: (a) `waitgroupgo` in `internal/server/run.go` and
`internal/router/lifecycle.go`, because `wg.Go` panics if called after `wg.Wait` has
started and a hand-written `Add` may have been placed outside a loop deliberately;
(b) `minmax` rewrites where the original `if` had a side effect in its body (the analyzer
does not fire on those, but confirm).

## 5. Phase 2 — hand edits (one commit per bullet group, ordered by value)

1. **`pkg/fcsdk` error wrapping (13 sites).** `%v` → `%w` in cache/lock packages. This is
   the one behavioural improvement: SDK users get `errors.Is(err, pgx.ErrNoRows)` etc.
   through the wrapper. Check nothing in `pkg/fcsdk` documents the error as opaque.
2. **Contains/dedupe/keys helpers (15 sites, §2.3 rows 2–3, 6).** Delete the helpers,
   use `slices`/`maps` at call sites.
3. **`sort` → `slices` (33 sites).** Mechanical; `slices.Sort` for `sort.Strings`,
   `slices.SortFunc` + `strings.Compare`/`cmp.Compare` for `sort.Slice`. Where a
   `sort.SliceStable` is used (`principal/api/api.go:266`) use `slices.SortStableFunc`.
4. **`slices.Clone` (50 sites).** `sed`-able: `append([]string(nil), X...)` →
   `slices.Clone(X)`. Nil in → nil out, same as before.
5. **`sync.OnceValue(s)` (3 sites).** `passwordhash.EqualizeTiming` and
   `docsapi.publishedIndex` are the clearest wins; both become a single package-level
   `var x = sync.OnceValue(func() T {…})`.
6. **`cmp.Or` (subset of the 48).** Only where both sides are plain expressions and no
   comment explains the fallback. Expect ~25 to qualify.
7. **`strings.Cut` / `CutSuffix` / `strings.Builder` (8 sites).**
8. **Tests: `t.Context()`, `t.Setenv`, `atomic.Int32`.** Package by package; run
   `make test-integration` after each package because of the Cleanup gotcha in §2.4.

`internal/router` sites: do them in this plan's Phase 1 (they are pure syntax), but leave
anything structural — stop channels, `stopOnce`, notification lifecycle — to
`router-go-idiom-plan.md`, which already owns that.

## 6. Design-level options — owner decision, not part of the sweep

### 6.1 `omitempty` on `time.Time` — RETRACTED (no finding)

The first draft of this document claimed 37 `time.Time` fields carried a no-op `omitempty`
and proposed `omitzero`. That was a regex error: every one of the 37 is `*time.Time` or
`*jsontime.Time`, on which `omitempty` works exactly as intended (nil → field absent).
There is no wire change to make and nothing for `omitzero` to fix. huma v2.38 honours
both tags identically for the spec, so the lockfile is unaffected either way.

### 6.2 `testing/synctest` for the router timing tests

50 `time.Sleep` calls across `router/manager_lifecycle_test.go`, `semaphore_test.go`,
`circuit_breaker_test.go` and friends are wall-clock waits. Go 1.25's `synctest.Test`
gives a fake clock and deterministic goroutine scheduling: the tests get faster and stop
being flake candidates. It is a rewrite of each test's structure, not a substitution, so it
belongs with the router plan's test items rather than here.

### 6.3 Not recommended

`iter.Seq` for repository listing (no consumer would benefit yet), `context.WithCancelCause`
for shutdown reasons (nice-to-have, the router plan should decide), `os.Root` (no untrusted
path handling found).

## 7. Order and cost

| Step | Files | Effort | Risk |
|---|---:|---|---|
| Phase 0 guard rails + 20 errorlint fixes | ~10 | 1 h | none |
| Phase 1 `go fix` + helper deletion | 95 | 1 h incl. reading the diff | low |
| Phase 2.1–2.5 | ~40 | 2 h | low |
| Phase 2.6–2.8 (`cmp.Or`, tests) | ~60 | 2–3 h | low; `t.Context` needs care |
| Go 1.27 bump | 5 + CI | 30 min | low; json v2 backing, lint toolchain |

Do Phase 0 before anything else (the 1.27 bump first, so `go fix` runs at the final
language version once); after it, Phases 1–2 cannot regress because the linters
own the rules.

## 8. Status — 2026-09-05

| Commit | What |
|---|---|
| `cfe1237` | Go 1.27, alpine3.24 on every Docker stage, CI lint via goinstall, `install-tools` v2 path, five baseline lint findings |
| `1df39d0` | `go fix` (unit + integration), 20 dead pointer helpers removed, `AsRouterError` → `errors.AsType`, all errorlint sites, `errorlint` + `modernize` enabled |
| `<phase 2>` | sort → slices, keys → `slices.Sorted(maps.Keys)`, contains helpers → `slices.Contains`, `sync.OnceValue(s)`, `strings.Cut` |

Verified after each commit: `make lint` (0 issues), unit suite, full integration
suite (108 packages) after Phase 1 and the 18 touched packages after Phase 2,
`make api-diff` (lock unchanged), `make analyze`.

Not done, by decision (see the tables above for the reasoning): `slices.Clone`
swap, `t.Context()` sweep, `t.Setenv` (all in `TestMain`), `bytes.Buffer` sites,
`CutSuffix` sites. Not done, still open and cheap: `cmp.Or` for the plain
`if x == "" { x = default }` sites (each needs a read, roughly 25 qualify),
`atomic.Int32` for the six remaining `atomic.AddInt32` calls in
`outbox/group_distributor_test.go`, and the three `log.Printf` calls. §6.2
(`testing/synctest` for router timing tests) remains an owner decision.
