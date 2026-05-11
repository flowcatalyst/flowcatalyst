#!/usr/bin/env sh
# fc-dev installer for macOS and Linux.
#
# Quick install (latest release):
#   curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
#
# Pin a version:
#   curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | FC_DEV_VERSION=0.3.8 sh
#
# Environment variables:
#   FC_DEV_VERSION       Pin a specific version (default: latest fc-dev/v* release)
#   FC_DEV_INSTALL_DIR   Where to write the binary (default: /usr/local/bin if
#                        writable without sudo, else $HOME/.local/bin)
#   FC_DEV_FORCE         "1" to reinstall even if the same version is already
#                        present
#
# Re-runs are safe: if the binary is already installed at the same version,
# the script exits without touching anything.

set -eu

REPO="flowcatalyst/flowcatalyst"
TAG_PREFIX="fc-dev/v"
BIN="fc-dev"

# ─── output helpers ────────────────────────────────────────────────────────

info()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn()  { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }
err()   { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# ─── platform detection ────────────────────────────────────────────────────

detect_target() {
    os=$(uname -s)
    arch=$(uname -m)
    case "$os" in
        Darwin)
            case "$arch" in
                arm64|aarch64) target="aarch64-apple-darwin" ;;
                *) err "fc-dev is only published for Apple Silicon (arm64) macOS." ;;
            esac
            ;;
        Linux)
            case "$arch" in
                x86_64|amd64) target="x86_64-unknown-linux-gnu" ;;
                aarch64|arm64) target="aarch64-unknown-linux-gnu" ;;
                *) err "Unsupported Linux architecture: $arch" ;;
            esac
            ;;
        *)
            err "Unsupported OS '$os'. For Windows, use install.ps1 in PowerShell."
            ;;
    esac
    printf '%s' "$target"
}

# ─── dependency checks ─────────────────────────────────────────────────────

need_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        err "required command not found: $1"
    fi
}

# ─── latest-version lookup ─────────────────────────────────────────────────

# Fetch the highest semver tag matching fc-dev/vX.Y.Z. Anonymous GitHub API
# requests are limited to 60/hour per source IP — fine for a manual install
# but documented here in case someone tries to script bulk installs.
fetch_latest_version() {
    # `releases?per_page=100` returns all relevant tags in one page until the
    # repo has hundreds of releases. Filtering by prefix here (rather than
    # using /releases/latest) is essential because the same repo also tags
    # `laravel-sdk/v…` and `typescript-sdk/v…`, which would otherwise show
    # up as "latest".
    api="https://api.github.com/repos/${REPO}/releases?per_page=100"
    body=$(curl -fsSL "$api") || err "could not query GitHub API at $api"

    # POSIX-pure parsing of the JSON. Each tag_name lives on its own line
    # after this transformation; we then keep only `fc-dev/v*` ones, strip
    # the prefix, drop draft/prerelease (best-effort — we filter by tag
    # shape only), and pick the max semver.
    version=$(
        printf '%s' "$body" \
            | tr ',' '\n' \
            | grep '"tag_name"' \
            | sed -e 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//' -e 's/".*//' \
            | grep "^${TAG_PREFIX}" \
            | sed "s|^${TAG_PREFIX}||" \
            | awk -F. '
                /^[0-9]+\.[0-9]+\.[0-9]+$/ {
                    # Convert X.Y.Z into a sortable key like 000010002000003
                    printf "%010d%010d%010d %s\n", $1, $2, $3, $0
                }' \
            | sort -r \
            | awk 'NR==1 {print $2}'
    )

    if [ -z "$version" ]; then
        err "no fc-dev releases found at $api (looking for tags matching ${TAG_PREFIX}X.Y.Z)"
    fi
    printf '%s' "$version"
}

# ─── install destination ───────────────────────────────────────────────────

default_install_dir() {
    # Prefer /usr/local/bin if we can write to it without sudo (common dev
    # workstation case where the user already owns it). Otherwise install
    # into the user-local equivalent — no sudo prompt during a curl-pipe-sh
    # install, which is what most users expect.
    if [ -w /usr/local/bin ] 2>/dev/null; then
        printf '%s' "/usr/local/bin"
    else
        printf '%s' "$HOME/.local/bin"
    fi
}

# ─── checksum verification ─────────────────────────────────────────────────

verify_sha256() {
    file=$1
    sidecar=$2
    expected=$(awk '{print $1}' "$sidecar")
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        warn "no sha256 tool found — skipping checksum verification"
        return 0
    fi
    if [ "$expected" != "$actual" ]; then
        err "checksum mismatch for $file (expected $expected, got $actual)"
    fi
}

# ─── main ──────────────────────────────────────────────────────────────────

main() {
    need_cmd curl
    need_cmd tar
    need_cmd uname

    info "Detecting platform"
    target=$(detect_target)
    info "Target: $target"

    if [ -n "${FC_DEV_VERSION:-}" ]; then
        version=$FC_DEV_VERSION
        info "Using pinned version $version (FC_DEV_VERSION)"
    else
        info "Looking up the latest fc-dev release"
        version=$(fetch_latest_version)
        info "Latest: $version"
    fi

    install_dir=${FC_DEV_INSTALL_DIR:-$(default_install_dir)}
    info "Installing into $install_dir"

    # Idempotency: if the same version is already at the destination, exit
    # quietly. Set FC_DEV_FORCE=1 to override (useful when debugging a
    # corrupted install).
    if [ -x "$install_dir/$BIN" ] && [ "${FC_DEV_FORCE:-0}" != "1" ]; then
        existing=$("$install_dir/$BIN" --version 2>/dev/null | awk '{print $NF}' || true)
        if [ "$existing" = "$version" ]; then
            info "fc-dev v$version is already installed at $install_dir/$BIN — nothing to do."
            info "Set FC_DEV_FORCE=1 to reinstall."
            return 0
        fi
    fi

    asset="fc-dev-v${version}-${target}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${TAG_PREFIX}${version}/${asset}"
    sha_url="${url}.sha256"

    tmp=$(mktemp -d 2>/dev/null || mktemp -d -t fc-dev-install)
    # Defensive cleanup in case the trap doesn't fire under a piped sh.
    trap 'rm -rf "$tmp"' EXIT INT HUP TERM

    info "Downloading $asset"
    if ! curl -fL --progress-bar -o "$tmp/$asset" "$url"; then
        err "download failed: $url"
    fi
    if ! curl -fsSL -o "$tmp/$asset.sha256" "$sha_url"; then
        warn "no SHA256 sidecar at $sha_url — skipping verification"
    else
        info "Verifying SHA256"
        verify_sha256 "$tmp/$asset" "$tmp/$asset.sha256"
    fi

    info "Extracting"
    tar -xzf "$tmp/$asset" -C "$tmp"
    extracted_bin="$tmp/fc-dev-v${version}-${target}/$BIN"
    if [ ! -x "$extracted_bin" ]; then
        err "extracted archive missing $BIN at $extracted_bin"
    fi

    info "Installing to $install_dir/$BIN"
    if ! mkdir -p "$install_dir" 2>/dev/null; then
        err "cannot create $install_dir — set FC_DEV_INSTALL_DIR to a writable location"
    fi
    # `install` (BSD/GNU coreutils) gives atomic placement + correct perms in
    # one step. `cp + chmod` is the fallback when install(1) isn't on PATH.
    if command -v install >/dev/null 2>&1; then
        install -m 0755 "$extracted_bin" "$install_dir/$BIN"
    else
        cp "$extracted_bin" "$install_dir/$BIN"
        chmod 0755 "$install_dir/$BIN"
    fi

    # macOS quarantines anything downloaded by curl. Strip the attribute so
    # the first launch doesn't hit "fc-dev cannot be opened because it is
    # from an unidentified developer". Once we ship notarized builds (see
    # docs/release-signing.md) this line can go away.
    if [ "$(uname -s)" = "Darwin" ]; then
        xattr -d com.apple.quarantine "$install_dir/$BIN" 2>/dev/null || true
    fi

    info "Installed: $install_dir/$BIN"
    "$install_dir/$BIN" --version 2>/dev/null || true

    # PATH hint — if the install dir isn't on PATH the user will scratch their
    # head trying to invoke `fc-dev`. Tell them explicitly with the right
    # rc-file line for their shell.
    case ":$PATH:" in
        *":$install_dir:"*) ;;
        *)
            warn "$install_dir is not on your PATH."
            rc=""
            case "$(basename "${SHELL:-}")" in
                zsh)  rc="$HOME/.zshrc" ;;
                bash) rc="$HOME/.bashrc" ;;
                fish) rc="$HOME/.config/fish/config.fish" ;;
            esac
            if [ -n "$rc" ]; then
                printf '\nAdd to %s:\n  export PATH="%s:$PATH"\n\n' "$rc" "$install_dir" >&2
            else
                printf '\nAdd to your shell rc:\n  export PATH="%s:$PATH"\n\n' "$install_dir" >&2
            fi
            ;;
    esac

    info "Done. Run: fc-dev"
}

main "$@"
