# Releasing

Every release line has a `VERSION` file and is cut with one `make` target;
**one release per make invocation**. `scripts/release.sh` refuses a dirty
tree, refuses a tag that already exists, bumps from `max(VERSION, highest
tag)`, commits the bump, tags `<prefix>/vX.Y.Z` and pushes `HEAD` plus the tag.

| Line | Target | Tag | What the tag triggers |
|---|---|---|---|
| fcdev | `make release-dev BUMP=…` | `fcdev/vX.Y.Z` | `release-fcdev.yml` |
| TypeScript SDK | `make release-ts-sdk BUMP=…` | `typescript-sdk/vX.Y.Z` | `split-typescript-sdk.yml` (disabled, `if: false`) |
| Laravel SDK | `make release-laravel-sdk BUMP=…` | `laravel-sdk/vX.Y.Z` | `split-laravel-sdk.yml` (disabled, `if: false`) |
| Java SDK | `make release-java-sdk BUMP=…` | `java-sdk/vX.Y.Z` | none |
| Function Go SDK | `make release-fn-go-sdk BUMP=…` | `clients/fn-go/vX.Y.Z` | nothing: the Go module proxy serves the tag |
| Function TS SDK | `make release-fn-ts-sdk BUMP=…` | `fn-ts-sdk/vX.Y.Z` | `split-fn-ts-sdk.yml` (**off**) |
| Function Rust SDK | `make release-fn-rust-sdk BUMP=…` | `fn-rust-sdk/vX.Y.Z` | `split-fn-rust-sdk.yml` (**off**) |

`BUMP` is `patch`, `minor`, `major` or an explicit `X.Y.Z[-suffix]`. Set
`RELEASE_DRY_RUN=1` to run every check and print the would-be diff without
committing, tagging or pushing (`RELEASE_DRY_RUN=1 make release-fn-ts-sdk BUMP=0.1.0`).

## Function SDKs

The function runner SDKs (`clients/fn-go`, `fn-ts`, `fn-rust`, `fn-js-engine`)
are unreleased. Their `VERSION` files are seeded at `0.0.0`, so the **first
release of each must be explicit**: `BUMP=0.1.0` (the manifests already say
0.1.0). A bare `patch` would produce 0.0.1.

`fcdev fn init` (`cmd/fcdev/fn_init.go`) writes the dependency a user gets
without `--sdk-path`: Go `require …/clients/fn-go v0.0.0`, Rust
`flowcatalyst-fn = "0.1"` (crates.io), TypeScript `"@flowcatalyst/fn": "^0.1.0"`
(npm). Those templates are what each release decision below has to satisfy.

### fn-go: tag the monorepo, no split

A Go module living in a subdirectory is fetched straight from the repo with a
tag prefixed by the directory: `clients/fn-go/v0.1.0`. `make release-fn-go-sdk`
cuts exactly that tag. No workflow, no standalone repo: splitting would only
add a second source of truth. The module has zero third-party dependencies and
no `replace` directives (the only `replace` is the one `fcdev fn init
--sdk-path` writes into a user's project).

**Module path (done).** `clients/fn-go/go.mod` declares
`github.com/flowcatalyst/flowcatalyst/clients/fn-go`, matching the GitHub repo
`flowcatalyst/flowcatalyst`; the proxy resolves the module path to a repo URL,
so a tag `clients/fn-go/vX.Y.Z` is fetchable. Every importer was updated
(`examples/hello/main.go`, the README, `docs/published/60-functions.md`, and
the Go template plus `replace` line in `cmd/fcdev/fn_init.go`). Still to do
with the first release:

1. change the template's `require … v0.0.0` to the released version (v0.0.0
   does not resolve without the `replace` that `--sdk-path` writes);
2. the repo must be public (or consumers set `GOPRIVATE` and have access).

`scripts/release.sh fn-go` guards the module path: it refuses if the module path is
not the expected one. (The root module `github.com/flowcatalyst/flowcatalyst-go`
is unaffected: nothing imports it externally.)

### fn-ts: split to `flowcatalyst/fn-ts-sdk`, npm publish optional

Same shape as the TypeScript SDK: a standalone repo, built from a
splitsh-lite split of `clients/fn-ts/`, with plain `vX.Y.Z` tags. Name
`fn-ts-sdk` mirrors `typescript-sdk` (the platform API client) and keeps the two
unambiguous; `fn` alone would collide with the Go and Rust function SDKs, and
`functions-ts` would hide that it is a guest SDK. Unlike `typescript-sdk` there
is no `dist/` to build: the package ships TypeScript source (`main`/`types` are
`src/index.ts`, bundled by the `fc-fn-build` bin), so the workflow runs
`npm ci`, `typecheck` and the tests on the monorepo before splitting, and tags
the split commit itself.

npm publish is a separate job, off by default (needs its own variable and
`NPM_TOKEN`). Owner changes required before it can work (not done):

- `"private": true` in `clients/fn-ts/package.json` must be removed (npm refuses
  to publish a private package);
- `esbuild` is a `devDependency` but the shipped `fc-fn-build` bin imports it:
  move it to `dependencies` or consumers' `fc-fn-build` fails after install;
- add `files` (`src`, `bin`, `README.md`), `license` (`Apache-2.0`),
  `repository`, `publishConfig.access: public`;
- own the `@flowcatalyst` npm scope.

### fn-rust: split to `flowcatalyst/fn-rust-sdk`, crates.io publish optional

Split to `flowcatalyst/fn-rust-sdk` (same naming rationale). The workflow runs
`cargo test --locked`, splits, force-pushes and tags `vX.Y.Z`; consumers can
depend with `git = …, tag = "vX.Y.Z"` meanwhile. The `fcdev fn init` template
assumes crates.io (`flowcatalyst-fn = "0.1"`), so the crate job is the real
distribution path; it is off by default (own variable and
`CARGO_REGISTRY_TOKEN`). `cargo publish --dry-run` passes today but
`clients/fn-rust/Cargo.toml` is missing publish metadata that crates.io expects:

- `license = "Apache-2.0"` (**required** by crates.io; repo LICENSE is Apache-2.0)
- `repository = "https://github.com/flowcatalyst/fn-rust-sdk"`
- `readme = "README.md"`
- `keywords`, `categories`, `rust-version`, optionally `homepage`/`documentation`
- consider `exclude = ["examples/", "VERSION"]` to keep the package small.

`release.sh` bumps `Cargo.toml` and the crate's own entry in `Cargo.lock`
(so `--locked` keeps working). `examples/hello/Cargo.lock` is not touched.

### fn-js-engine: internal artifact, not released

The engine is QuickJS compiled to Wasm (`build.sh`), and the built
`internal/functions/jsengine/engine.wasm` is committed and embedded in the
runner. Function authors never depend on it: they bundle with `fc-fn-build` and
the runner loads the engine itself. Its `Cargo.toml` already says
`publish = false`. It ships inside fcdev / the runner image, versioned by the
fcdev release. No release target, no split, no publish.

### One-time owner setup

All of this is currently **not done; every function-SDK automation is OFF**.

1. **fn-go**: the module path is fixed. Make sure the repo is public, then
   `make release-fn-go-sdk BUMP=0.1.0`, and afterwards bump the template's
   `require … v0.0.0` in `cmd/fcdev/fn_init.go` to the released version.
2. **Create the target repos** (empty, default branch `main`):
   `flowcatalyst/fn-ts-sdk` and `flowcatalyst/fn-rust-sdk`.
3. **Create tokens and store them as repository secrets** on
   `flowcatalyst/flowcatalyst` (fine-grained PATs with Contents: read/write on
   the one target repo): `FN_TS_SDK_TOKEN`, `FN_RUST_SDK_TOKEN`.
4. **Switch the split on**: Settings > Secrets and variables > Actions >
   Variables > `SPLIT_FN_SDKS` = `true`. It gates both workflows; unset or any
   other value and tag pushes run nothing.
5. **npm (optional)**: remove `private`, fix the manifest items above, create an
   automation token, store `NPM_TOKEN`, set variable `PUBLISH_FN_TS_NPM` = `true`.
6. **crates.io (optional)**: add the metadata above, create an API token, store
   `CARGO_REGISTRY_TOKEN`, set variable `PUBLISH_FN_RUST_CRATE` = `true`.
7. **First release**, one per invocation: `make release-fn-ts-sdk BUMP=0.1.0`,
   then `make release-fn-rust-sdk BUMP=0.1.0`.

Do not enable the split while another source repo force-pushes the same target
`main`; the workflows force-push by design.

### Status

| | State |
|---|---|
| `make release-fn-{go,ts,rust}-sdk`, `RELEASE_DRY_RUN` | working |
| `split-fn-ts-sdk.yml`, `split-fn-rust-sdk.yml` | in repo, **off** (`SPLIT_FN_SDKS` unset) |
| npm publish, crates.io publish | **off** (second variables unset, metadata missing) |
| fn-go module path | fixed (`github.com/flowcatalyst/flowcatalyst/clients/fn-go`); release guard passes |
| target repos, tokens | not created |
