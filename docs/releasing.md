# Releasing

Every release line has a `VERSION` file and is cut with one `make` target;
**one release per make invocation**. `scripts/release.sh` refuses a dirty
tree, refuses a tag that already exists, bumps from `max(VERSION, highest
tag)`, commits the bump, tags `<prefix>/vX.Y.Z` and pushes `HEAD` plus the tag.

| Line | Target | Tag | What the tag triggers |
|---|---|---|---|
| fcdev + fc-server image | `make release-dev BUMP=…` | `fcdev/vX.Y.Z` | `release-fcdev.yml` (binaries, GitHub Release) and `release-fc-server-image.yml` (`ghcr.io/flowcatalyst/fc-server:X.Y.Z` + `:latest`, amd64 + arm64) |
| TypeScript SDK | `make release-ts-sdk BUMP=…` | `typescript-sdk/vX.Y.Z` | `split-typescript-sdk.yml`: splits `clients/typescript-sdk` to `flowcatalyst/typescript-sdk`, builds `dist/`, tags `vX.Y.Z` (token `TYPESCRIPT_SDK_TOKEN`) |
| Laravel SDK | `make release-laravel-sdk BUMP=…` | `laravel-sdk/vX.Y.Z` | `split-laravel-sdk.yml`: splits `clients/laravel-sdk` to `flowcatalyst/laravel-sdk`, tags `vX.Y.Z` (token `LARAVEL_SDK_TOKEN`) |
| Java SDK | `make release-java-sdk BUMP=…` | `java-sdk/vX.Y.Z` | `release-java-sdk.yml`: tests, then publishes `io.flowcatalyst:flowcatalyst-sdk` (jar + sources) to GitHub Packages |
| Go SDK | `make release-go-sdk BUMP=…` | `go-sdk/vX.Y.Z` | nothing: the script itself publishes `vX.Y.Z` to `flowcatalyst/go-sdk` |
| Function Go SDK | `make release-fn-go-sdk BUMP=…` | `clients/fn-go/vX.Y.Z` | nothing: the Go module proxy serves the tag |
| Function TS SDK | `make release-fn-ts-sdk BUMP=…` | `fn-ts-sdk/vX.Y.Z` | `split-fn-ts-sdk.yml`: split to `flowcatalyst/fn-ts-sdk` |
| Function Rust SDK | `make release-fn-rust-sdk BUMP=…` | `fn-rust-sdk/vX.Y.Z` | `split-fn-rust-sdk.yml` (**off** unless `SPLIT_FN_SDKS` is set) |
| Ingestor | `make release-ingestor BUMP=…` | `ingestor/vX.Y.Z` | `ingestor-release.yml`: fat jar as a GitHub Release, and `ghcr.io/flowcatalyst/ingestor:X.Y.Z` + `:latest` (amd64 + arm64); `split-ingestor.yml` (**off** unless `SPLIT_INGESTOR` is set) splits to `flowcatalyst/ingestor` |
| Field Android library | `make release-field-android BUMP=…` | `field-android/vX.Y.Z` | `release-field-android.yml`: tests, then publishes `io.flowcatalyst:field-android` to GitHub Packages |
| Field JS library | `make release-field-js BUMP=…` | `field-js/vX.Y.Z` | nothing: the script checks, packs and publishes `vX.Y.Z` to `flowcatalyst/field-js` |
| Field React Native library | `make release-field-rn BUMP=…` | `field-react-native/vX.Y.Z` | nothing: the script checks, packs and publishes `vX.Y.Z` to `flowcatalyst/field-react-native` |

`BUMP` is `patch`, `minor`, `major` or an explicit `X.Y.Z[-suffix]`. Set
`RELEASE_DRY_RUN=1` to run every check and print the would-be diff without
committing, tagging or pushing (`RELEASE_DRY_RUN=1 make release-fn-ts-sdk BUMP=0.1.0`).

## Published artifacts: where each lands

| Artifact | Where | Notes |
|---|---|---|
| fcdev binaries | GitHub Release `fcdev/vX.Y.Z` | sha256 sidecars; Linux archives cosign-signed |
| fc-server image | `ghcr.io/flowcatalyst/fc-server:X.Y.Z`, `:latest` | linux/amd64 + linux/arm64; same tag and commit as fcdev |
| Ingestor image | `ghcr.io/flowcatalyst/ingestor:X.Y.Z`, `:latest` | linux/amd64 + linux/arm64 |
| Ingestor jar | GitHub Release `ingestor/vX.Y.Z` | fcdev downloads it to run an ingestor locally |
| Java SDK | GitHub Packages Maven, `https://maven.pkg.github.com/flowcatalyst/flowcatalyst` | `io.flowcatalyst:flowcatalyst-sdk`; not on Maven Central |
| Field Android | the same GitHub Packages registry | `io.flowcatalyst:field-android` |
| Go SDK, field JS/RN | their own git repositories | see the sections below |

Pre-release tags (`-rc.1`) push an image tagged with their own version only, never `:latest`.
Re-running a workflow: Actions > the workflow > Run workflow, with the existing tag as input.
GitHub Packages Maven refuses to overwrite a published version, so a failed Java SDK
publish after the deploy step started is fixed with a new version.

### Container images (GHCR)

`release-fc-server-image.yml` builds the root `Dockerfile` (SPA built inside, Go
cross-compiled for the target architecture) and `ingestor-release.yml` builds
`ingestor/Dockerfile` around the fat jar. Both log in with `GITHUB_TOKEN`
(`packages: write`) and set OCI labels (`org.opencontainers.image.source` links the
package to this repository).

- **Visibility.** A package first pushed by a workflow is private. If the owner wants
  anonymous pulls, set each package public once after its first push: organisation
  `flowcatalyst` > Packages > `fc-server` / `ingestor` > Package settings > Change
  visibility. It stays public for later pushes.
- Pull: `docker pull ghcr.io/flowcatalyst/fc-server:X.Y.Z` (private packages need
  `docker login ghcr.io` with a token that has `read:packages`).
- The ingestor build resolves the framework from GitHub Packages and needs the
  repository secret `FRAMEWORK_PACKAGES_TOKEN` (`read:packages` on
  `flowcatalyst/framework`).
- Neither image workflow could be run end to end before the first tag: the first
  release is the first real test, so cut it as a pre-release tag
  (`BUMP=X.Y.Z-rc.1`) if a dry run is wanted.

### Java SDK (GitHub Packages)

`release-java-sdk.yml` checks `clients/java-sdk/VERSION` and the pom version against
the tag, runs `mvn verify`, then `mvn deploy` (jar + sources) with `GITHUB_TOKEN`.

GitHub Packages Maven **always needs a token with `read:packages`, even for public
packages**. A consumer adds the repository and credentials:

```xml
<!-- pom.xml -->
<repositories>
  <repository>
    <id>github-flowcatalyst</id>
    <url>https://maven.pkg.github.com/flowcatalyst/flowcatalyst</url>
  </repository>
</repositories>

<dependency>
  <groupId>io.flowcatalyst</groupId>
  <artifactId>flowcatalyst-sdk</artifactId>
  <version>X.Y.Z</version>
</dependency>
```

```xml
<!-- ~/.m2/settings.xml -->
<settings>
  <servers>
    <server>
      <id>github-flowcatalyst</id>
      <username>YOUR_GITHUB_USERNAME</username>
      <password>${env.GITHUB_TOKEN}</password> <!-- classic PAT with read:packages -->
    </server>
  </servers>
</settings>
```

The server `id` must equal the repository `id`. In GitHub Actions use the workflow's
`GITHUB_TOKEN` (the consuming repository needs access to the package) via
`actions/setup-java` `server-id` / `server-username` / `server-password`.
The pom's own `distributionManagement` id is `github`; consumers do not use it.

## Releasing every SDK at once

`make release-all-sdks BUMP=patch|minor|major` (default `patch`) runs
`scripts/release-all-sdks.sh`. It prints a plan, asks once, then calls
`scripts/release.sh` for each line whose directory changed since its last tag:
ts, laravel, java, go-sdk, fn-ts, fn-rust, fn-go, field-android, field-js, field-rn (and fcdev with `WITH_DEV=1`; the ingestor is not part of it, release it with `make release-ingestor`).

- `RELEASE_ALL_PLAN=1` prints the plan only. `RELEASE_DRY_RUN=1` runs every
  check per line and changes and pushes nothing.
- `ONLY="fn-ts fn-go"` and `SKIP="java"` choose lines.
- A line never released (VERSION 0.0.0, no tag) is skipped unless named in
  `ONLY`; it then gets `0.1.0`.
- The generated SDKs (ts, laravel, java) are refused if `make sdk-spec` would
  change files (stale spec): run `make sdk-generate`, review and commit first.
- `release.sh` pushes `HEAD`, so every unpushed commit goes up with the first
  release. A failed line stops the run; rerun with `ONLY` set to the rest.

## Field JS packages (`clients/field-js`, `clients/field-react-native`)

`@flowcatalyst/field-js` and `@flowcatalyst/field-react-native` are installed
from git, not from a registry (owner ruling 2026-11-07). Each has its own
repository, `github.com/flowcatalyst/field-js` and
`github.com/flowcatalyst/field-react-native`, and an app depends on a tag:

```json
"@flowcatalyst/field-js": "github:flowcatalyst/field-js#v0.1.0",
"@flowcatalyst/field-react-native": "github:flowcatalyst/field-react-native#v0.1.0"
```

npm, pnpm and yarn all install that; a private repository needs the
installer's own git access (SSH key or token), nothing else.

- **What a release does.** `release.sh` first runs
  `scripts/publish-git-package.sh` in check-only mode on `HEAD`: it exports the
  package as committed, installs with its `pnpm-lock.yaml`
  (`--frozen-lockfile`), runs `typecheck`, `lint` and `test` where the package
  has them, and packs it (`prepack` builds `dist/` or `lib/`). Then it bumps
  `VERSION` and `"version"`, commits, tags `<prefix>/vX.Y.Z` here and pushes,
  and runs the script for real on that tag: one commit whose tree is exactly
  what `npm pack` produced, parented on the published repository's `main`,
  message `Release vX.Y.Z` (naming the source commit), pushed as `main` plus
  the tag `vX.Y.Z`. The published `package.json` has no `scripts` and no
  `devDependencies`, so installing from git never builds on the consumer's
  machine. It runs locally with your own git access: no workflow, no token.
- **Checks.** `release.sh` refuses before committing if the published
  repository can't be reached or already has `vX.Y.Z`, or if the package's
  checks fail. If publishing fails after the push here, rerun
  `scripts/publish-git-package.sh <dir> <prefix>/vX.Y.Z X.Y.Z <repo-url>`: it
  says "already published" for the same tree and refuses a different tree (cut
  a new version rather than move a tag an app may have locked).
- **Changes go here, not there.** The published repositories are output; the
  next release replaces their tree.
- Both start at `0.0.0`, so the **first release of each must be explicit**:
  `make release-field-js BUMP=0.1.0`, then `make release-field-rn BUMP=0.1.0`.

Owner setup before the first release: create the two repositories on GitHub,
**empty** (no README or licence commit; the first release makes `main`), and
make sure the people who install them can read them.

`field-react-native` drives the native module in `clients/field-android`; it
does not depend on it at the npm level (the Android library is consumed from
GitHub Packages by the app). Release `field-js` first if the two change together.

## Go SDK (`pkg/fcsdk`)

The Go SDK is its own Go module, `github.com/flowcatalyst/go-sdk`
(`pkg/fcsdk/go.mod`), published to its own repository
`github.com/flowcatalyst/go-sdk` (owner ruling 2026-10-30: consumers must not
depend on this repository, which need not stay public). Consumers run
`go get github.com/flowcatalyst/go-sdk@vX.Y.Z`.

- **One commit per release.** `make release-go-sdk BUMP=…` bumps
  `pkg/fcsdk/VERSION`, commits, tags `go-sdk/vX.Y.Z` here and pushes, then runs
  `scripts/publish-go-sdk.sh go-sdk/vX.Y.Z X.Y.Z`. That script makes one commit
  whose tree is `pkg/fcsdk` exactly as tagged, parented on the published repo's
  `main`, message `Release vX.Y.Z`, and pushes it as `main` plus the tag
  `vX.Y.Z` (what the module proxy serves). No history or commit message from
  this repository goes with it. It runs locally with your own git access; no
  workflow, no token.
- **Checks.** `release.sh` refuses before committing if `go.mod` does not
  declare `github.com/flowcatalyst/go-sdk`, if the published repo can't be
  reached, or if it already has `vX.Y.Z`. If the publish step fails after the
  push here, rerun `scripts/publish-go-sdk.sh go-sdk/vX.Y.Z X.Y.Z`: it says
  "already published" for the same tree and refuses a different tree (a
  published Go version is immutable; the proxy has cached it).
- **Changes go here, not there.** The published repo is output: the next
  release replaces its tree with `pkg/fcsdk`, so anything committed directly
  there is lost (its history is kept).
- **Old module path.** v0.0.1 to v0.0.8 were tags `pkg/fcsdk/vX.Y.Z` on this
  repository, module `github.com/flowcatalyst/flowcatalyst/pkg/fcsdk`. Those
  versions stay on the module proxy; moving to the new path is an import path
  change only. The first release under the new path is v0.0.9.
- **The platform uses the tree's copy**: the root `go.mod` requires the module
  through `replace github.com/flowcatalyst/go-sdk => ./pkg/fcsdk`
  (the version in its `require` line is a placeholder). The Dockerfile copies
  `pkg/fcsdk/go.{mod,sum}` before `go mod download` for that reason.
- **Its own CI**: `.github/workflows/go-sdk.yml` (tidy, build, vet, race tests
  on the `go 1.26.0` floor and on 1.27, golangci-lint, and
  `scripts/go-sdk-smoke.sh`, which consumes the SDK from a scratch module
  through a local `replace`, and through `go get` after
  `scripts/publish-go-sdk.sh` has published HEAD to a throwaway bare repository).
  Locally: `make test-go-sdk`, `make lint-go-sdk`, `make go-sdk-smoke`;
  `make test-unit` and `make lint` include the module. The tests that read files
  outside `pkg/fcsdk` (the OpenAPI lock, the framework's conformance vectors)
  skip when the SDK is checked out on its own.
- **Licence**: Apache-2.0; the module carries its own `LICENSE` and `NOTICE`.
- The application framework's Go FlowCatalyst module
  (`../flowcatalyst-framework/go/flowcatalyst`) depends on this module by its
  released version.

## Function SDKs

The function runner SDKs (`clients/fn-go`, `fn-ts`, `fn-rust`, `fn-js-engine`).
Released so far: fn-go tags up to `clients/fn-go/v0.4.0`, fn-ts up to
`fn-ts-sdk/v0.6.0` (served from the split repo `flowcatalyst/fn-ts-sdk`). fn-rust
has no release yet, so its first release must be explicit (`BUMP=0.1.0`; its
`VERSION` is seeded at 0.0.0 and a bare `patch` would produce 0.0.1). The
one-time setup list and status table below were written before the first
releases and are partly historical; check the repository variables and tags for
the current state.

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
