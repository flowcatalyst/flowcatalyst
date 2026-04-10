# fc-dev Self-Update

## Overview

fc-dev supports automatic version checking on startup and a `self-update` subcommand
that downloads the latest release binary from GitHub Releases and replaces itself.

## Architecture

### 1. Version Check on Startup (non-blocking)

On every startup, a background tokio task checks for newer versions. It does not
block the server — if the check fails or times out, startup continues silently.

```
fc-dev starting...
  New version available: v0.4.0 (current: v0.3.1)
  Run `fc-dev self-update` to upgrade.
```

**Rate limiting:** Cache the last check timestamp in `~/.fc-dev/last-update-check`.
Only hit the network if the last check was more than 24 hours ago. This avoids
unnecessary GitHub API calls on frequent restarts during development.

**Implementation:**

```rust
// Spawn non-blocking — never delays startup
tokio::spawn(async {
    match tokio::time::timeout(Duration::from_secs(2), check_for_update()).await {
        Ok(Ok(Some(new_version))) => {
            eprintln!(
                "\n  New version available: v{} (current: v{})\n  Run `fc-dev self-update` to upgrade.\n",
                new_version,
                env!("CARGO_PKG_VERSION")
            );
        }
        _ => {} // timeout, error, or no update — silently continue
    }
});
```

**Check logic:**

```rust
async fn check_for_update() -> Result<Option<String>> {
    let cache_path = dirs::home_dir().unwrap().join(".fc-dev/last-update-check");

    // Skip if checked within 24 hours
    if let Ok(metadata) = tokio::fs::metadata(&cache_path).await {
        if let Ok(modified) = metadata.modified() {
            if modified.elapsed().unwrap_or_default() < Duration::from_secs(86400) {
                // Read cached latest version
                let cached = tokio::fs::read_to_string(&cache_path).await?;
                let latest = cached.trim();
                if latest > env!("CARGO_PKG_VERSION") {
                    return Ok(Some(latest.to_string()));
                }
                return Ok(None);
            }
        }
    }

    // Fetch latest release tag from GitHub API
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(2))
        .build()?;
    let release: serde_json::Value = client
        .get("https://api.github.com/repos/{owner}/{repo}/releases/latest")
        .header("User-Agent", "fc-dev")
        .send()
        .await?
        .json()
        .await?;

    let tag = release["tag_name"].as_str().unwrap_or("").trim_start_matches('v');

    // Cache the result
    tokio::fs::create_dir_all(dirs::home_dir().unwrap().join(".fc-dev")).await.ok();
    tokio::fs::write(&cache_path, tag).await.ok();

    if tag > env!("CARGO_PKG_VERSION") {
        Ok(Some(tag.to_string()))
    } else {
        Ok(None)
    }
}
```

### 2. `fc-dev self-update` Subcommand

Downloads the correct platform binary from GitHub Releases and replaces the
current executable in place.

**Dependency:** `self_update` crate (supports GitHub Releases natively).

```toml
[dependencies]
self_update = { version = "0.41", features = ["archive-tar", "compression-flate2"] }
```

**Implementation:**

Add a clap subcommand:

```rust
#[derive(Parser)]
struct Args {
    #[command(subcommand)]
    command: Option<Command>,

    // ... existing args
}

#[derive(Subcommand)]
enum Command {
    /// Update fc-dev to the latest version
    SelfUpdate,
}
```

Handle it before starting the server:

```rust
if let Some(Command::SelfUpdate) = args.command {
    println!("Checking for updates...");
    let status = self_update::backends::github::Update::configure()
        .repo_owner("{owner}")
        .repo_name("{repo}")
        .bin_name("fc-dev")
        .show_download_progress(true)
        .current_version(cargo_crate_version!())
        .build()?
        .update()?;
    println!("Updated to v{}", status.version());
    return Ok(());
}
```

### 3. GitHub Release Asset Naming Convention

The `self_update` crate expects assets named with a target triple pattern.
CI must produce and upload assets matching this convention:

```
fc-dev-v{version}-x86_64-apple-darwin.tar.gz
fc-dev-v{version}-aarch64-apple-darwin.tar.gz
fc-dev-v{version}-x86_64-unknown-linux-gnu.tar.gz
fc-dev-v{version}-aarch64-unknown-linux-gnu.tar.gz
```

Each archive contains the single `fc-dev` binary.

### 4. CI: GitHub Actions Release Workflow

Trigger on tag push (`v*`). Build per-platform, upload as release assets.

```yaml
name: Release fc-dev
on:
  push:
    tags: ["v*"]

jobs:
  build:
    strategy:
      matrix:
        include:
          - target: x86_64-apple-darwin
            os: macos-13
          - target: aarch64-apple-darwin
            os: macos-14
          - target: x86_64-unknown-linux-gnu
            os: ubuntu-latest
          - target: aarch64-unknown-linux-gnu
            os: ubuntu-latest

    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4

      - name: Install Rust toolchain
        uses: dtolnay/rust-toolchain@stable
        with:
          targets: ${{ matrix.target }}

      - name: Install cross (Linux ARM)
        if: matrix.target == 'aarch64-unknown-linux-gnu'
        run: cargo install cross

      - name: Build
        run: |
          if [ "${{ matrix.target }}" = "aarch64-unknown-linux-gnu" ]; then
            cross build --release --bin fc-dev --target ${{ matrix.target }}
          else
            cargo build --release --bin fc-dev --target ${{ matrix.target }}
          fi

      - name: Package
        run: |
          cd target/${{ matrix.target }}/release
          tar czf ../../../fc-dev-${{ github.ref_name }}-${{ matrix.target }}.tar.gz fc-dev

      - name: Upload release asset
        uses: softprops/action-gh-release@v2
        with:
          files: fc-dev-${{ github.ref_name }}-${{ matrix.target }}.tar.gz

  create-release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
```

### 5. Version Embedding

Ensure the binary knows its own version at compile time. Cargo handles this
automatically via `Cargo.toml`:

```toml
[package]
name = "fc-dev"
version = "0.3.1"  # bump on each release
```

Access in code:

```rust
env!("CARGO_PKG_VERSION")      // "0.3.1"
cargo_crate_version!()          // from self_update macro, same thing
```

### 6. Dependencies to Add

```toml
# bin/fc-dev/Cargo.toml
[dependencies]
self_update = { version = "0.41", features = ["archive-tar", "compression-flate2"] }
dirs = "6"
```

### 7. Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `FC_DEV_UPDATE_CHECK` | `true` | Set to `false` to disable startup version check |
| `GITHUB_TOKEN` | (none) | Optional — avoids GitHub API rate limits for private repos |

### 8. Disable Check

For CI or scripted usage, skip the update check:

```bash
FC_DEV_UPDATE_CHECK=false ./fc-dev
```

## Summary

| Feature | Blocks startup? | Network call? | Frequency |
|---|---|---|---|
| Version check | No (background) | Yes (2s timeout) | Once per 24h (cached) |
| `self-update` | Yes (explicit) | Yes (downloads binary) | On demand |
