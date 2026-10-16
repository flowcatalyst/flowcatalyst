# fcdev profiles: using `fcdev fn` against nonprod and prod

`fcdev fn` talks to the platform as an OAuth client (client_credentials). Against a local fcdev it
uses the dev credential `fcdev start` writes. Against a **remote** platform it uses a **profile**:
a named target whose client secret is held by reference (OS keychain, environment variable, file or
command), never in the profile file.

```
fcdev profile add np --platform https://np.example.com --env np --client-id fc_client_abc
fcdev fn status example.test --profile np
```

## The rules

| Situation | Behaviour |
|---|---|
| `--profile NAME` (or `FC_PROFILE`) | The profile fully determines platform URL, runner URL, client id and secret. `--platform`, `--client-id` and `--client-secret` are **refused** alongside it; only `--runner` may override. `FC_PLATFORM_URL`, `FC_RUNNER_URL`, `FC_CLIENT_ID` and `FC_CLIENT_SECRET` are ignored, with a stderr warning each. `--profile` beats `FC_PROFILE`. |
| No profile, loopback target | Unchanged: flags, then `FC_*` env, then the local dev credential file. When an `FC_*` variable supplied a value, a stderr line says so. |
| No profile, **non-loopback** target (from `--platform`, `FC_PLATFORM_URL`, or the dev file) | **Refused**, with a message naming `fcdev profile add`. The same applies to a non-loopback runner when a token would be sent to it (`invoke --anonymous` sends none and may use any runner). |

CI therefore uses a profile too: `env:`, `file:` and `command:` references work without a keychain.
The old explicit `--platform URL --client-id ID --client-secret S` form keeps working for loopback
URLs only.

Loopback means `localhost`, `*.localhost` or a loopback IP (`127.0.0.0/8`, `::1`). Profile URLs must be
`https` unless loopback, and must not embed credentials (`user:pass@`).

## Safety banner, prod confirmation, `--dry-run`

Every `fcdev fn` command that talks to a platform prints one line to stderr first:

```
profile prod-eu (prod) https://fc.example.com
```

(`profile (none) (local) http://127.0.0.1:8080` for an unprofiled loopback call.)

Commands that change remote state are `publish`, `promote`, `deploy`, `set`, `delete`, and `invoke`
with any method other than GET/HEAD (a call can run arbitrary function code). Against a profile with
`env = "prod"`:

* **On a terminal** you must type `prod`. `--yes` does **not** bypass this. `delete` additionally asks
  for the function address, as before (prod first, then the address).
* **Without a terminal** the command is refused unless you pass `--yes` **and** the profile has
  `allow_noninteractive = true` (set it on CI profiles: `profile add ... --allow-noninteractive`).

`np` and `local` profiles need no confirmation. (The tag is a convention you set: a prod platform
registered with `--env np` is not protected.)

`fcdev fn deploy --dry-run` builds, validates the describe document, looks the function up (reads
only) and prints what would be created, published and promoted. Nothing is written remotely and no
confirmation is needed. It also reports the CI flags below: `new:` / `unchanged:` for the digest, and
whether it would promote.

## Profile file

Default location: `os.UserConfigDir()/flowcatalyst/profiles.toml`
(`~/.config/flowcatalyst/profiles.toml` on Linux, `~/Library/Application Support/flowcatalyst/profiles.toml`
on macOS, `%AppData%\flowcatalyst\profiles.toml` on Windows). Override with `FC_PROFILES_PATH`.

```toml
[profiles.np]
platform = "https://np.example.com"
runner = "https://np-runner.example.com"   # optional; used by `fcdev fn invoke`
env = "np"                                   # local | np | prod
client_id = "fc_client_abc"
secret_ref = "keychain"                      # default

[profiles.prod-ci]
platform = "https://fc.example.com"
env = "prod"
client_id = "fc_client_ci"
secret_ref = "env:FC_PROD_SECRET"
allow_noninteractive = true
```

The format is a strict subset of TOML (tables `[profiles.NAME]`, strings, booleans, `#` comments; the
parser is about 100 lines in `cmd/fcdev/profile_store.go`, chosen over a new dependency because no TOML
library was in `go.mod`). Anything else is an error. Commands that rewrite the file (`add`, `remove`)
regenerate it, so hand-written comments are not preserved.

Guards:

* The file must not be group- or other-writable (it can name a command that fcdev will run), or fcdev
  refuses to load it. `chmod 600` it. fcdev writes it 0600 in a 0700 directory. (No mode check on Windows.)
* It never holds a secret value. Any key that looks like one (`client_secret`, `password`, `token`, ...)
  is rejected, as is any unknown key. A `secret_ref` that is not a recognised reference (for example a
  pasted plaintext secret) is rejected, and the error never echoes the value.

### Secret references

| `secret_ref` | Meaning |
|---|---|
| `keychain` (default) | The OS secret store: service `flowcatalyst-fcdev`, account = profile name. |
| `env:NAME` | The value of environment variable `NAME` (must be set and non-empty). |
| `file:/abs/path` | The file's contents, trimmed. Must be a regular file with mode `0600` or tighter (no group/other access) or fcdev refuses it. |
| `command:<argv...>` | Run the program with those arguments and use its trimmed stdout. |

`command:` is split into an argv with simple quoting (`"..."` and `'...'` group words; backslash escapes
`\"` and `\\` inside double quotes) and executed **without a shell**: `$(...)`, `;`, `|`, `>` and globbing
are literal characters. It has a 30 second limit; stderr (first 4 KB) is shown on failure.

**Risk:** whatever the profile says is executed, as you. That is why the profile file must be
owner-writable only, and why you should treat it like a shell rc file.

AWS SSM Parameter Store (CI):

```toml
secret_ref = "command:aws ssm get-parameter --name /flowcatalyst/prod/fn-cli-secret --with-decryption --query Parameter.Value --output text"
```

or with the CLI:

```
fcdev profile add prod-ci --platform https://fc.example.com --env prod --client-id fc_client_ci \
  --allow-noninteractive \
  --secret-ref 'command:aws ssm get-parameter --name /flowcatalyst/prod/fn-cli-secret --with-decryption --query Parameter.Value --output text'
fcdev fn deploy example.test ./fn --profile prod-ci --yes
```

## Using fcdev in CI

A pipeline deploys a function only when it changed, decided by the artifact's **digest** (the sha256 of
the built bundle, which is the identity of a version on the platform). Nothing in this flow needs a
terminal, a keychain or a prompt.

### Authentication

`fcdev profile add` is non-interactive when `--secret-ref env:NAME` (or `file:` / `command:`) is given:
it never touches the keychain and never prompts. The profile file holds only the reference; the secret
stays in the CI secret store and is read from the environment variable at run time. The profile file
goes to the user config directory (`$XDG_CONFIG_HOME` or `$HOME/.config`; set `FC_PROFILES_PATH` to put
it elsewhere, e.g. the runner's temp dir). The OAuth client needs the `platform:function-publisher`
role (see "Least privilege"; `--create --client <code>` also needs permission to read clients, or pass
the `clt_` id).

For a prod profile add `--allow-noninteractive` when creating it, and pass `--yes` on the write
commands: without both, a prod write is refused with no terminal. Runs that make no remote write
(`--skip-unchanged` with an unchanged digest, `--dry-run`) need neither.

```yaml
# GitHub Actions: secrets FC_CLIENT_ID / FC_CLIENT_SECRET, variable FC_PLATFORM_URL
- name: Install fcdev
  run: |
    VERSION=0.5.0   # pin a released fcdev
    TARGET=linux-amd64
    base="https://github.com/flowcatalyst/flowcatalyst/releases/download/fcdev/v${VERSION}"
    curl -fsSLO "${base}/fcdev-v${VERSION}-${TARGET}.tar.gz"
    curl -fsSLO "${base}/fcdev-v${VERSION}-${TARGET}.tar.gz.sha256"
    shasum -a 256 -c "fcdev-v${VERSION}-${TARGET}.tar.gz.sha256"
    tar -xzf "fcdev-v${VERSION}-${TARGET}.tar.gz"
    sudo install -m 0755 "fcdev-v${VERSION}-${TARGET}/fcdev" /usr/local/bin/

- name: Deploy changed functions
  env:
    FC_CLIENT_SECRET: ${{ secrets.FC_CLIENT_SECRET }}   # read via secret_ref env:FC_CLIENT_SECRET
  run: |
    fcdev profile add ci --platform "${{ vars.FC_PLATFORM_URL }}" --env prod --allow-noninteractive \
      --client-id "${{ secrets.FC_CLIENT_ID }}" --secret-ref env:FC_CLIENT_SECRET
    fcdev profile test ci
    fcdev fn deploy myapp.myfn ./fn --profile ci --yes --skip-unchanged --json > deploy.json
    cat deploy.json
```

(`--env np` for a nonprod platform; then `--yes` and `--allow-noninteractive` are not needed.)

### Flags for pipelines

```
fcdev fn publish <address> <artifact> [--create --client <code|clt_id>] [--json]
fcdev fn deploy  <address> [dir|artifact] [--no-promote] [--skip-unchanged] [--dry-run] [--json]
```

`publish` reads the function's versions first (read-only) and compares digests. Publishing is
idempotent per digest, so this only decides what to say and avoids the write:

```
new: myapp.myfn v7 (js, 3f2a9c01b7de)                       # a version was created
unchanged: myapp.myfn v6 already has digest 3f2a9c01b7de   # nothing uploaded or written, no confirmation
requeued: myapp.myfn v6 was FAILED (<reason>), retrying    # the digest's version was FAILED: published again to retry
```

A version a runner marked `FAILED` is never "unchanged". The failure is latched by the first runner
report and is often transient (an artifact-store blip), so the CLI treats `FAILED` as "needs retry":
it uploads and publishes the same digest again, and the platform re-queues that version (same number,
back to `PUBLISHED`, runners re-fetch desired state and retry). Re-running a pipeline whose deploy
failed that way therefore retries instead of re-reading the stored failure. Against a platform that
predates re-queue support the CLI stops with `... is still FAILED after re-publishing its digest`
rather than wait for a version that will never load.

`publish` never promotes: no alias moves, no subscription or schedule is wired (that happens at promote).

`deploy`:

| Situation | What happens |
|---|---|
| new digest | publish, wait until a runner has proved it loadable (READY), promote `live` |
| `--no-promote`, new digest | publish and wait for READY; `live` is **not** moved (`not promoted: ...`) |
| `--no-promote`, digest already a version | no write at all (only waits if it is not READY yet); no confirmation |
| `--skip-unchanged`, `live` already has this digest | prints `unchanged: myapp.myfn live v6 already has digest 3f2a9c01b7de`, exits 0: no publish, no wait, no promote, no remote write, no confirmation |
| `--skip-unchanged`, digest is a version but not `live` | no upload, but it is promoted (unless `--no-promote`) |
| digest's version is `FAILED` (with or without `--skip-unchanged`) | never skipped: prints `requeued: ... was FAILED (<reason>), retrying`, re-publishes the digest (re-queues the same version), waits for READY, then promotes (unless `--no-promote`). It is a remote write, so a prod profile asks for confirmation |
| neither flag, digest already a version | no upload; promoted again (as before) |

`--json` prints exactly one JSON object on stdout; the banner and all progress go to stderr.

```json
{"address":"myapp.myfn","functionId":"fn_0HZ...","version":7,"digest":"<full sha256>","created":true,"promoted":true,"skipped":false,"runtime":"js","client":"clt_0HZ..."}
```

`publish --json` has `address, functionId, version, digest, created, runtime, client`, plus `requeued`
(only when true) when the digest's version was `FAILED` and this run re-queued it (`created` is false;
`deploy --json` has `requeued` the same way). `created` means
this run created the version (false: the digest already existed); `promoted` means this run moved
`live`; `skipped` means `--skip-unchanged` found `live` already at this digest; `client` is the owning
client's `clt_` id and is omitted for a platform-owned function. A `--dry-run --json` adds
`"dryRun":true`, writes nothing and reports the same fields for what would happen.

### What changes the digest

The digest is the sha256 of the bundle bytes, so anything that changes the bytes makes a "new" version.
Two builds of the same source with the same inputs give identical bytes (checked by
`TestFnBuildIsReproducible` against `examples/functions/dispatch-demo/payload-logger`: no timestamps,
no absolute paths, no randomness in the bundle). What does change it:

* **Source** changes, including values baked in with `define` (such as the demo's `APP_CODE`).
* **Dependency versions**: commit the lockfile and install with `npm ci`, never `npm install`; a
  floating `^` range with no lockfile makes the digest drift between runs.
* **The bundler version**: pin `esbuild` exactly (the examples use `0.24.2`); a new esbuild can change
  its output.
* **Node/TypeScript toolchain** only through the above (they are not embedded).
* **Relative paths of linked dependencies**: esbuild writes a `// path/to/file.ts` comment per bundled
  file, relative to the project. Registry packages appear as `node_modules/...` and are the same in
  every checkout. A `file:` or `link:` dependency outside the project (as in this repo's examples) is
  written as `../../../clients/...`, so the same source built at a different directory depth gives a
  different digest. In CI build from a fixed layout, or depend on a published package.
* Go and Rust artifacts: the same toolchain version in CI, `go.sum` / a committed `Cargo.lock`. The CLI
  builds Go with `-trimpath`, so the checkout's path is not embedded. (Reproducibility of the Go and
  Rust builds is not tested here, only the JS one.)

When a digest changes without a source change, diff the two bundles.

## Commands

```
fcdev profile add <name> --platform URL --env np|prod|local --client-id ID [--runner URL] [--secret-ref REF] [--allow-noninteractive]
fcdev profile list                 # names, env, URL, secret-ref kind; never secrets
fcdev profile show <name>
fcdev profile set-secret <name>    # prompt (no echo, twice) and store in the keychain
fcdev profile remove <name> [--yes]   # type the name to confirm; also deletes the keychain entry
fcdev profile test <name>          # mint a token; reports principal id, tier and granted scope names
```

`add` without `--secret-ref` prompts for the secret (no echo) and stores it in the keychain. Without a
terminal it refuses unless `--secret-ref env:/file:/command:` is given. `test` never prints the token or
the secret, and never follows redirects (a 307 would replay the secret).

## Keychain backends and caveats

Backend: `github.com/zalando/go-keyring` v0.2.8 (MIT).

| OS | Store | Notes |
|---|---|---|
| macOS | Keychain, through `/usr/bin/security` | Writes go through `security -i` and **stdin**, so the secret does not appear in the process list (older go-keyring releases passed `-w <secret>` as an argument; the pinned version does not). Reads use `security find-generic-password -w`, which only prints to the pipe. Because fcdev reaches the Keychain only through `/usr/bin/security`, the item's access list names that tool, not fcdev: rebuilding or not signing `fcdev` does not by itself trigger a prompt (a binary that called the Keychain API directly would). macOS may still ask for your login password when the keychain is locked, or the first time `security` reads an item created by another tool. (Expected behaviour from how the tool works; not verified on a real keychain here.) |
| Windows | Credential Manager | Generic credential `flowcatalyst-fcdev:<profile>`. Limit 2560 bytes. |
| Linux | Secret Service (libsecret over D-Bus: GNOME Keyring, KWallet) | Needs a running session bus and an unlocked keyring. **Headless machines and CI have none**: the error says so; use `env:`, `file:` or `command:` references there. |

Not verified by the automated tests, which use an in-memory fake and never touch a real keychain.

## Least privilege for the CLI credential

Do not give a nonprod/prod CLI credential super-admin. Create a dedicated service-account OAuth client
and grant it:

* `platform:function-publisher`: view, publish and promote versions (enough for `publish`, `promote`,
  `deploy` of an existing function, `status`).
* For `deploy --create` / `publish --create`: also function manage (`platform:function:function:manage`)
  and application view, to create the function resource under its application.
* For `--client <code>` (with `--create`): client view (`CanReadClients`). The owning client is given by its
  code (the client's identifier, e.g. `bevco`, stable across environments) or by its `clt_...` id. A code is
  resolved with a read-only `GET /api/clients/by-identifier/{code}` using the same credential; an id is used
  as-is with no lookup. If the credential cannot read clients, the lookup fails with the platform's 403 reason
  and a hint to pass the `clt_` id instead. An unknown code errors with `no client with code "x" on <URL>`.
  `--client` is only consulted when the function is being created; for an existing function it is ignored.
* For `set`/`delete`: function manage and (for `--secret`/`--db`) `platform:function:secret:manage`.
* For `invoke` with a token: `platform:function:version:invoke`.

The local dev CLI credential (`fcdev-fn-cli`, written by `fcdev start`) is super-admin, but only ever
exists on a local fcdev.

## Secrets in output

fcdev never prints a resolved secret or token. Errors from the token endpoint, the secret command and
the keychain are scrubbed of the secret before display, and `slog` fields never include it.
