# Release Signing

This is the runbook for signing the fc-dev release binaries. Nothing in
this document is implemented yet — the v1 release pipeline ships unsigned
binaries. Use this when you're ready to take that step.

## Why sign

| Without signing | With signing |
|---|---|
| macOS Gatekeeper blocks first launch ("from an unidentified developer"). User must right-click → Open. | First launch works normally. |
| Windows SmartScreen warns ("Windows protected your PC"). User must click "More info" → "Run anyway". | Trusted by SmartScreen, no prompt. |
| Linux: only TLS-from-GitHub for integrity. Mirrors / proxies are a blind spot. | Verifiable with `cosign verify-blob` or GPG. |
| Corporate review burns goodwill — every desk is a one-off exception. | Standard "did you sign it?" review passes. |

We deferred signing to "Phase 2" in the v1 plan. This doc closes that
loop.

## Cost summary

| Platform | Status | One-time effort | Ongoing | Notes |
|---|---|---|---|---|
| macOS  | not done | ~half day | $99/year | Apple Developer Program. DUNS verification can take 1–2 weeks for orgs that don't already have one. |
| Windows | not done | ~half day | $120/year | Azure Trusted Signing ($9.99/mo). Requires an Azure subscription. |
| Linux (cosign) | **implemented** | — | $0 | Done in `release-fc-dev.yml`. Uses GitHub OIDC, no keys to manage. |
| Linux (GPG) | not done | ~1 hour | $0 | Adds a detached `.asc` for corporate reviewers who expect PGP. |
| Verification in upgrade | not done | ~1 day | n/a | ~150 LOC across platforms in `bin/fc-dev/src/upgrade.rs`. |

**Remaining Phase 2 effort: ~2 days work, ~$220/year ongoing** (macOS + Windows + GPG + upgrade-time verification).

---

## macOS — Apple Developer ID + notarization

### One-time setup (outside the repo)

1. **Enrol in the Apple Developer Program** at <https://developer.apple.com/programs/>.
   - $99/year. For an organisation, you'll need a DUNS number — [get one free
     from Dun & Bradstreet](https://developer.apple.com/support/D-U-N-S/) if
     you don't have one. DUNS issuance is 1–2 days; org enrolment after that
     is another 1–2 days. **Plan for a week** of calendar time.
   - Apple requires a legal entity name that matches your DUNS record.

2. **Generate a Developer ID Application certificate.**
   - In Apple Developer portal → Certificates → "+" → "Developer ID Application".
   - Generate a CSR locally first (`Keychain Access → Certificate Assistant
     → Request a Certificate from a Certificate Authority`, save to disk).
   - Upload CSR, download the issued `.cer`.
   - Double-click the `.cer` to import into Keychain. The private key is
     already in your Keychain from the CSR step; the cert pairs with it
     automatically.

3. **Export the cert + private key as a `.p12` (PKCS#12) bundle.**
   - In Keychain Access, select the cert + its private key together → right-click
     → "Export 2 items…" → `.p12` format → set a strong password.
   - Base64-encode for GitHub Secrets:
     ```bash
     base64 -i developer-id.p12 | pbcopy
     ```

4. **Generate an app-specific password for `notarytool`.**
   - <https://appleid.apple.com> → Sign-In and Security → App-Specific Passwords → Generate.
   - Label it "fc-dev notarization CI". Save the password.

5. **Find your Team ID.**
   - Apple Developer portal → Account → top-right shows the team. Or
     `xcrun altool --list-providers -u <apple-id> -p <app-specific-password>`.
   - It's a 10-character alphanumeric string like `A1B2C3D4E5`.

### GitHub Secrets to create

| Secret | Value |
|---|---|
| `MACOS_CERT_P12_BASE64` | Output of step 3's `base64` command |
| `MACOS_CERT_PASSWORD` | The `.p12` password from step 3 |
| `MACOS_APPLE_ID` | The Apple ID email used for the Developer Program |
| `MACOS_APPLE_TEAM_ID` | Step 5's team ID |
| `MACOS_APPLE_APP_SPECIFIC_PASSWORD` | Step 4's password |

### Workflow changes — `release-fc-dev.yml`

Add these steps to the macOS build job (`runner: macos-14`), **after**
`cargo build` and **before** `Package archive`:

```yaml
- name: Import signing certificate
  uses: apple-actions/import-codesign-certs@v3
  with:
    p12-file-base64: ${{ secrets.MACOS_CERT_P12_BASE64 }}
    p12-password: ${{ secrets.MACOS_CERT_PASSWORD }}

- name: Codesign binary
  env:
    BIN: target/${{ matrix.target }}/release/fc-dev
  run: |
    codesign --force --options runtime --timestamp \
      --sign "Developer ID Application: <YOUR_NAME_OR_ORG> (${{ secrets.MACOS_APPLE_TEAM_ID }})" \
      "$BIN"
    codesign --verify --deep --strict --verbose=2 "$BIN"
```

Then add **after** `Package archive`, replacing it with a sign-then-archive
sequence:

```yaml
- name: Notarize archive
  env:
    APPLE_ID: ${{ secrets.MACOS_APPLE_ID }}
    APPLE_TEAM_ID: ${{ secrets.MACOS_APPLE_TEAM_ID }}
    APPLE_APP_PASS: ${{ secrets.MACOS_APPLE_APP_SPECIFIC_PASSWORD }}
  run: |
    # Notarize the archive (Apple accepts .zip and .tar.gz). Submit and wait.
    xcrun notarytool submit "$ARCHIVE" \
      --apple-id "$APPLE_ID" \
      --team-id "$APPLE_TEAM_ID" \
      --password "$APPLE_APP_PASS" \
      --wait \
      --timeout 30m

    # Stapling embeds the notarization ticket so Gatekeeper works offline.
    # `stapler` only works on app bundles, .pkg, .dmg — NOT bare binaries
    # or tar.gz. So we don't staple here. Notarization status is verified
    # by Gatekeeper online on first launch (acceptable trade-off).
```

**Note on stapling:** `stapler` won't work on a tar.gz archive or a bare
binary. The two real options are:

1. Don't staple — Gatekeeper does an online check on first launch. Works
   offline once cached. Simplest.
2. Switch the macOS asset to a `.dmg` containing the binary, sign+notarize
   the `.dmg`, staple the ticket. More complex; users get a polished install
   experience. Probably overkill for a dev tool.

Recommendation: **option 1.** Document the online-check requirement.

### What users see

- **First launch online:** binary runs normally (Gatekeeper checks Apple's
  notarization service silently).
- **First launch offline:** Gatekeeper falls back to the local cache. If
  the binary's signature has been used before on this machine, it works;
  otherwise blocked. Users can right-click → Open to bypass once, then
  it's cached forever.

### Verification by users

```bash
codesign --verify --deep --strict --verbose=2 fc-dev
spctl --assess --type execute --verbose fc-dev
```

---

## Windows — Azure Trusted Signing

Microsoft's 2024 cloud signing service. Designed for CI/CD. No hardware
tokens, no certificate files in secrets, full automation from GitHub
Actions. SmartScreen builds reputation from your signed downloads over
time, the same way it does for OV/EV certs.

### One-time setup (outside the repo)

1. **Create an Azure subscription** if you don't have one. Free tier
   includes Trusted Signing trial credits.

2. **Create a Trusted Signing account.**
   - Azure Portal → "Trusted Signing accounts" → Create.
   - Pricing tier: **Basic** ($9.99/month) supports up to 5,000 signatures/month.
     Premium ($99.99/month) supports 100,000. Basic is plenty for fc-dev.
   - Region: pick one geographically close to GitHub Actions runners
     (East US 2 or West Europe).

3. **Verify your identity** — this is the slow part. Two paths:
   - **Public organisation** — submit business registration docs. Takes
     1–3 business days for Microsoft to verify.
   - **Individual** — submit government ID. Typically 1–2 business days.

4. **Create a Certificate Profile** under the Trusted Signing account.
   - Profile type: "Public Trust" for SmartScreen-recognised signing.
   - This profile holds the actual signing identity used by `signtool`.

5. **Create a service principal for GitHub Actions.**
   - Azure Portal → Microsoft Entra ID → App registrations → New registration.
   - Name: `fc-dev-signing-ci`.
   - Note the **Application (client) ID** and **Directory (tenant) ID**.

6. **Grant the service principal access** to the Trusted Signing account.
   - In the Trusted Signing account → Access control (IAM) → Add role
     assignment → "Trusted Signing Certificate Profile Signer" → assign to
     the service principal from step 5.

7. **Set up federated credentials** (so we don't need a long-lived secret).
   - In the app registration from step 5 → Certificates & secrets →
     Federated credentials → Add credential.
   - Scenario: GitHub Actions deploying Azure resources.
   - Organisation: `flowcatalyst`, Repository: `flowcatalyst`.
   - Entity type: Tag (since fc-dev uses tag-triggered releases).
   - Tag: `fc-dev/v*`.
   - This lets GitHub Actions auth via OIDC — **no client secret needed**.

### GitHub Secrets to create

With OIDC federation (recommended), you only need three non-secret values
(safe to put in plain repo variables, but secrets work too):

| Secret/Variable | Value |
|---|---|
| `AZURE_TENANT_ID` | Step 5's directory tenant ID |
| `AZURE_CLIENT_ID` | Step 5's application client ID |
| `AZURE_TRUSTED_SIGNING_ACCOUNT_NAME` | Step 2's account name |
| `AZURE_TRUSTED_SIGNING_PROFILE_NAME` | Step 4's profile name |
| `AZURE_TRUSTED_SIGNING_ENDPOINT` | Region endpoint, e.g. `https://eus.codesigning.azure.net/` |

### Workflow changes — `release-fc-dev.yml`

Add `id-token: write` permission at the top of the workflow (needed for
OIDC):

```yaml
permissions:
  contents: write
  id-token: write   # for Azure OIDC federation
```

Add these steps to the Windows build job (`runner: windows-2022`),
**after** `cargo build` and **before** `Package archive`:

```yaml
- name: Azure login (OIDC)
  uses: azure/login@v2
  with:
    client-id: ${{ secrets.AZURE_CLIENT_ID }}
    tenant-id: ${{ secrets.AZURE_TENANT_ID }}
    allow-no-subscriptions: true

- name: Sign binary with Trusted Signing
  uses: azure/trusted-signing-action@v0.5.1
  with:
    azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
    azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
    endpoint: ${{ secrets.AZURE_TRUSTED_SIGNING_ENDPOINT }}
    trusted-signing-account-name: ${{ secrets.AZURE_TRUSTED_SIGNING_ACCOUNT_NAME }}
    certificate-profile-name: ${{ secrets.AZURE_TRUSTED_SIGNING_PROFILE_NAME }}
    files-folder: target/${{ matrix.target }}/release
    files-folder-filter: exe
    file-digest: SHA256
    timestamp-rfc3161: http://timestamp.acs.microsoft.com
    timestamp-digest: SHA256
```

The action uses `signtool` under the hood and authenticates via OIDC.

### What users see

- **Day one:** SmartScreen still says "publisher: <your org name>" but
  may show "Windows protected your PC" until reputation builds. Sign
  enough downloads (a few hundred) and SmartScreen warms up.
- **After warm-up:** runs without prompts. The binary's "Properties →
  Digital Signatures" tab shows your org as the verified publisher.

### Verification by users

```cmd
signtool verify /pa /v fc-dev.exe
```

Or via PowerShell:

```powershell
Get-AuthenticodeSignature fc-dev.exe
```

---

## Linux — both cosign (recommended) and GPG (traditional)

Both are free; cosign needs no key management; GPG is what corporate
reviewers expect to see.

### Cosign keyless — IMPLEMENTED

> **Status:** shipped. Linux archives produced by `release-fc-dev.yml`
> are signed automatically. See the workflow file for the actual steps;
> the section below documents the design.

Cosign uses GitHub Actions' OIDC token as the signing identity. No keys
stored anywhere — every signature is bound to "the GitHub Actions workflow
that ran on tag X in repo Y". Verifiers check that binding by passing
`--certificate-identity-regexp` and `--certificate-oidc-issuer` flags;
the verification command is in the README and in every release's notes.

The workflow needs `permissions.id-token: write` to mint the OIDC token,
runs `sigstore/cosign-installer@v3`, then `cosign sign-blob --yes` per
Linux archive, producing `.sig` (signature) and `.pem` (cert chain)
sidecars. The upload-artifact `path` is a glob (`${{ env.ARCHIVE }}*`)
so both Linux sidecars and the SHA256 are picked up uniformly.

### GPG detached signature

For the corporate-reviewer audience that wants a "real" PGP signature.

#### One-time setup (outside the repo)

1. **Generate a long-lived release-signing key locally:**
   ```bash
   gpg --full-generate-key
   # Choose: RSA and RSA, 4096 bits, no expiry (or 2 years if you prefer)
   # Real name: "FlowCatalyst Release Signing"
   # Email: releases@flowcatalyst.io  (any address; doesn't have to receive mail)
   ```

2. **Export the private key:**
   ```bash
   gpg --armor --export-secret-keys releases@flowcatalyst.io > release-signing-private.asc
   ```

3. **Export the public key + commit it to the repo:**
   ```bash
   gpg --armor --export releases@flowcatalyst.io > docs/release-signing.gpg
   ```
   Users `gpg --import docs/release-signing.gpg` once and can verify all
   future releases.

4. **Publish the public key to a keyserver** (optional, but conventional):
   ```bash
   gpg --send-keys <KEY_ID>
   ```

#### GitHub Secrets to create

| Secret | Value |
|---|---|
| `GPG_PRIVATE_KEY` | Contents of `release-signing-private.asc` |
| `GPG_PASSPHRASE` | The passphrase you set in step 1 |

#### Workflow changes

In each Linux build job, **after** `Package archive`:

```yaml
- name: Import GPG key
  uses: crazy-max/ghaction-import-gpg@v6
  with:
    gpg_private_key: ${{ secrets.GPG_PRIVATE_KEY }}
    passphrase: ${{ secrets.GPG_PASSPHRASE }}

- name: GPG-sign archive
  run: |
    gpg --batch --yes --detach-sign --armor \
      --output "${ARCHIVE}.asc" \
      "$ARCHIVE"
```

Add `${{ env.ARCHIVE }}.asc` to the upload-artifact paths.

### Verification by users

```bash
# Cosign keyless — verifies the workflow that produced the archive
cosign verify-blob \
  --signature fc-dev-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz.sig \
  --certificate fc-dev-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz.pem \
  --certificate-identity-regexp "^https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fc-dev.yml@refs/tags/fc-dev/v" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  fc-dev-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz

# GPG — verifies against the public key
gpg --import docs/release-signing.gpg  # one-time
gpg --verify fc-dev-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz.asc \
              fc-dev-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz
```

---

## Verification in `fc-dev upgrade`

Currently `fc-dev upgrade` trusts HTTPS + GitHub for download integrity.
Adding signature verification before install is defence-in-depth that
matters if releases are ever distributed via mirrors, S3 buckets, or
internal proxies.

### Per-platform verification approach

| Platform | Verification method | Implementation |
|---|---|---|
| macOS | `codesign --verify --deep --strict` on the unpacked binary | `Command::new("codesign")…` from Rust. macOS-only `cfg`. |
| Windows | WinTrust API via `windows` crate, OR shell out to `signtool verify /pa` | The `windows` crate option is cleaner (no PATH dep) but ~50 LOC. The shell-out is 5 LOC but assumes signtool is on PATH (it isn't on user machines without the SDK). Recommend bundling a tiny WinTrust call. |
| Linux | Verify the `.sig`/`.pem` cosign artifacts via the [`sigstore` crate](https://crates.io/crates/sigstore), OR shell out to `cosign` | Crate is preferred — no external binary dependency. |

### Code structure

Add a new module `bin/fc-dev/src/upgrade/verify.rs`:

```rust
//! Pre-install signature verification. Called from `upgrade::install`
//! after the archive is extracted but before the new binary replaces
//! the running one. Failure aborts the upgrade.

#[cfg(target_os = "macos")]
pub fn verify(binary_path: &Path) -> anyhow::Result<()> { … }

#[cfg(target_os = "windows")]
pub fn verify(binary_path: &Path) -> anyhow::Result<()> { … }

#[cfg(target_os = "linux")]
pub fn verify(archive_path: &Path, sig_url: &str, cert_url: &str) -> anyhow::Result<()> { … }
```

The current `upgrade::install` uses `self_update`'s all-in-one flow which
hides the extracted file. We'd need to switch to the lower-level
`self_update::Download` + `self_update::Extract` + `self_update::Move`
APIs to insert a verification step between extract and move. ~50 LOC of
restructuring.

### Trust escape hatch

Add `fc-dev upgrade --no-verify` for the case where verification breaks
in some edge case and the user explicitly wants to bypass. Logs a warning;
not for routine use.

### What this does NOT solve

- **TOFU bootstrap** — the very first install of fc-dev still has to
  trust the download from GitHub Releases. Signing only protects
  *upgrades*, not the initial install. (Linux cosign verification would
  protect first-install too if users do it manually.)
- **Compromised CI** — if our GitHub Actions workflow is compromised, an
  attacker can sign malicious binaries with our legitimate signing keys.
  Mitigations: protect the signing secrets, enable required-reviewers on
  the release workflow, use OIDC federation (no long-lived secrets).

---

## Recommended order of operations

If you want to do this incrementally rather than all at once:

1. **Linux first** — cosign is free and instant, no enrolment delay. Demonstrates the pattern with zero risk. ~1 hour.
2. **macOS** — start the Apple Developer enrolment early (calendar-time delay is the bottleneck), then wire up signing once the cert arrives. ~1 week elapsed, ~half a day of actual work.
3. **Windows** — Azure Trusted Signing identity verification is 1–3 days; do it in parallel with macOS enrolment. Wire up the workflow once verified. ~half a day of work.
4. **Verification in upgrade** — last, once all three signing flows are stable. ~1 day.

## Open questions to revisit

- Code-signing key custody: does it stay in GH Actions Secrets / Azure / GitHub OIDC, or do we want a separate offline backup? (For a stolen-key scenario.)
- Required reviewers on the release workflow? Currently anyone with push access to a `fc-dev/v*` tag can produce a signed release. For internal use that's fine; for external distribution it's worth gating.
- Reproducible builds: signing breaks bit-for-bit reproducibility (signature is unique per build). Acceptable for our use case but documented for completeness.
