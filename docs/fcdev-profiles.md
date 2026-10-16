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
confirmation is needed.

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
* For `set`/`delete`: function manage and (for `--secret`/`--db`) `platform:function:secret:manage`.
* For `invoke` with a token: `platform:function:version:invoke`.

The local dev CLI credential (`fcdev-fn-cli`, written by `fcdev start`) is super-admin, but only ever
exists on a local fcdev.

## Secrets in output

fcdev never prints a resolved secret or token. Errors from the token endpoint, the secret command and
the keychain are scrubbed of the secret before display, and `slog` fields never include it.
