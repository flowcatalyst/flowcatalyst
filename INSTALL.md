# Installing fc-dev

`fc-dev` is the all-in-one local-development binary for FlowCatalyst. It
ships pre-built for macOS, Linux, and Windows; the install commands below
fetch the latest release from GitHub, verify it, and put it on your `PATH`.

If you'd rather download the archive by hand, see
[Manual install](#manual-install) further down.

---

## macOS (Apple Silicon)

```sh
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
```

This:

- Downloads the latest `fc-dev-vX.Y.Z-aarch64-apple-darwin.tar.gz` from GitHub Releases.
- Verifies the SHA256 against the `.sha256` sidecar.
- Installs to `/usr/local/bin/fc-dev` if writable (falls back to `~/.local/bin/fc-dev` otherwise — no `sudo` prompt).
- Strips the macOS quarantine attribute so Gatekeeper doesn't block the first launch.

> macOS Intel (x86_64) is **not** currently published — Apple-Silicon-only.

## Linux (x86_64 or aarch64)

```sh
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
```

Same script — it detects your architecture from `uname -m`. Tested on
Ubuntu 22.04 / 24.04 and Debian 12; should work on any glibc-2.31+ distro.

## Windows

PowerShell 5.1+ (the version that ships with Windows 10/11):

```powershell
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex
```

This:

- Downloads `fc-dev-vX.Y.Z-x86_64-pc-windows-msvc.zip`.
- Verifies the SHA256.
- Extracts to `%LOCALAPPDATA%\Programs\fc-dev\fc-dev.exe`.
- Adds that directory to your **user** PATH (re-open your terminal afterwards).

> On first launch SmartScreen may show a "Windows protected your PC" prompt
> because fc-dev isn't currently Authenticode-signed. Click **More info** →
> **Run anyway**. The next launch is silent.

---

## Re-running / upgrading

The installers are **idempotent**. If the same version is already installed
at the destination, they exit without changes. To always upgrade to the
latest release, set `FC_DEV_FORCE=1`:

```sh
# Force re-download even if already current
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | FC_DEV_FORCE=1 sh
```

```powershell
# Same idea in PowerShell
$env:FC_DEV_FORCE = '1'
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex
```

Once fc-dev is on your `PATH`, **the recommended upgrade path is the
binary's own self-update**:

```sh
fc-dev upgrade           # download & replace if a newer release exists
fc-dev upgrade --check   # just check, don't install
fc-dev upgrade --force   # re-install even if already current
```

The self-update flow uses the same release artefacts the installer scripts
do, plus the atomic rename-then-replace dance on Windows so the live `.exe`
can be replaced safely.

---

## Environment variables

Both installers honour the same three variables:

| Variable | Default | Purpose |
|---|---|---|
| `FC_DEV_VERSION` | latest stable | Pin a specific version (e.g. `0.3.8`). The leading `v` is optional and stripped. |
| `FC_DEV_INSTALL_DIR` | platform-default (see above) | Where to write `fc-dev`. Useful for managed environments / multi-user installs. |
| `FC_DEV_FORCE` | `0` | When `1`, reinstalls even if the requested version is already present. |

Example — pin to a specific version on Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh \
  | FC_DEV_VERSION=0.3.8 FC_DEV_INSTALL_DIR="$HOME/bin" sh
```

---

## Manual install

If you'd rather not pipe a remote script into your shell, the releases
page lists each archive plus its SHA256 sidecar:

<https://github.com/flowcatalyst/flowcatalyst/releases>

```sh
# 1. Pick your target triple
TARGET=aarch64-apple-darwin       # macOS Apple Silicon
# TARGET=x86_64-unknown-linux-gnu  # Linux x86_64
# TARGET=aarch64-unknown-linux-gnu # Linux ARM64
# TARGET=x86_64-pc-windows-msvc    # Windows (use the .zip variant)

VERSION=0.3.8

# 2. Download archive + checksum
curl -LO "https://github.com/flowcatalyst/flowcatalyst/releases/download/fc-dev/v${VERSION}/fc-dev-v${VERSION}-${TARGET}.tar.gz"
curl -LO "https://github.com/flowcatalyst/flowcatalyst/releases/download/fc-dev/v${VERSION}/fc-dev-v${VERSION}-${TARGET}.tar.gz.sha256"

# 3. Verify
shasum -a 256 -c "fc-dev-v${VERSION}-${TARGET}.tar.gz.sha256"

# 4. Extract + install
tar -xzf "fc-dev-v${VERSION}-${TARGET}.tar.gz"
sudo install -m 0755 "fc-dev-v${VERSION}-${TARGET}/fc-dev" /usr/local/bin/

# 5. (macOS only) strip Gatekeeper quarantine
xattr -d com.apple.quarantine /usr/local/bin/fc-dev 2>/dev/null || true

fc-dev --version
```

### Cryptographic verification of Linux archives (optional)

Linux release archives are additionally signed via Sigstore **cosign**
keyless — the signature is bound to the exact GitHub Actions workflow run
that built it, recorded in the public Rekor transparency log. You don't
have to verify, but if you'd like to:

```sh
# Install cosign once (https://docs.sigstore.dev/system_config/installation/)
cosign verify-blob \
  --signature  "fc-dev-v${VERSION}-${TARGET}.tar.gz.sig" \
  --certificate "fc-dev-v${VERSION}-${TARGET}.tar.gz.pem" \
  --certificate-identity-regexp '^https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fc-dev.yml@refs/tags/fc-dev/v' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  "fc-dev-v${VERSION}-${TARGET}.tar.gz"
```

macOS and Windows archives are not yet codesigned — see
[`docs/release-signing.md`](docs/release-signing.md) for the planned work.

---

## Troubleshooting

### "fc-dev: command not found" after install

Your shell hasn't picked up the install dir yet.

- macOS / Linux: open a new terminal **or** `source ~/.zshrc` (or `~/.bashrc`).
- Windows: close + re-open the terminal; user-PATH changes don't propagate to existing sessions.

You can also explicitly verify the binary exists:

```sh
ls -l /usr/local/bin/fc-dev   # macOS / Linux
where fc-dev                  # Windows
```

### macOS "fc-dev cannot be opened because it is from an unidentified developer"

The installer strips the quarantine xattr automatically. If you downloaded
manually:

```sh
xattr -d com.apple.quarantine /usr/local/bin/fc-dev
```

Or right-click the binary in Finder → **Open** → **Open** (Gatekeeper
remembers your "yes" for future launches).

### Windows SmartScreen warning

Click **More info** → **Run anyway**. fc-dev isn't Authenticode-signed
yet; SmartScreen flags any unsigned binary on first launch.

### Corporate proxies / restricted networks

The installer needs HTTPS access to:

- `api.github.com` (release metadata)
- `github.com` / `objects.githubusercontent.com` (asset download CDN)

If those are blocked, download the archive on an unrestricted machine and
copy it across, then follow the [Manual install](#manual-install) steps.

### `error: failed to run custom build command for postgresql_embedded` (cargo install / source builds)

You're trying to build fc-dev from source rather than installing the
release binary. The embedded-PostgreSQL build script needs network access
to fetch the platform-specific PG bundle, which fails behind some
proxies. **Use the release archive instead** — it has the PG bundle
already embedded, no network or build-time tooling required.
