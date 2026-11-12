#!/usr/bin/env sh
# fcdev installer for macOS and Linux.
#
# Quick install (latest release):
#   curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
#
# Pin a version:
#   curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | FC_DEV_VERSION=0.5.0 sh
#
# Environment variables:
#   FC_DEV_VERSION       Pin a specific version (default: latest fcdev/v* release)
#   FC_DEV_INSTALL_DIR   Where to write the binary (default: /usr/local/bin if
#                        writable without sudo, else $HOME/.local/bin)
#   FC_DEV_FORCE         "1" to reinstall even if the same version is already
#                        present
#   FCDEV_INSECURE_SKIP_CHECKSUM
#                        "1" to proceed when the release publishes no readable
#                        SHA-256 sidecar (or no sha256 tool is installed).
#                        Without it the install stops. A checksum MISMATCH
#                        always stops, whatever this says.
#
# Integrity: the archive's SHA-256 is checked against the release's sidecar.
# On Linux, if `cosign` is installed and the release publishes the archive's
# .sig and .pem, the keyless signature is verified too (this script never
# installs cosign).
#
# Re-runs are safe: if the binary is already installed at the same version,
# the script exits without touching anything. Once fcdev is on your PATH,
# `fcdev upgrade` self-updates without re-running this script.

set -eu

REPO="flowcatalyst/flowcatalyst"
TAG_PREFIX="fcdev/v"
BIN="fcdev"

# ─── output helpers ────────────────────────────────────────────────────────

info()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn()  { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }
err()   { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# ─── platform detection ────────────────────────────────────────────────────

# fcdev release assets are named with Go's GOOS-GOARCH convention
# (darwin-arm64, linux-amd64, …), not Rust target triples.
detect_target() {
    os=$(uname -s)
    arch=$(uname -m)
    case "$os" in
        Darwin) goos="darwin" ;;
        Linux)  goos="linux" ;;
        *) err "Unsupported OS '$os'. For Windows, use install.ps1 in PowerShell." ;;
    esac
    case "$arch" in
        x86_64|amd64)  goarch="amd64" ;;
        arm64|aarch64) goarch="arm64" ;;
        *) err "Unsupported architecture: $arch" ;;
    esac
    printf '%s-%s' "$goos" "$goarch"
}

# ─── dependency checks ─────────────────────────────────────────────────────

need_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        err "required command not found: $1"
    fi
}

# ─── latest-version lookup ─────────────────────────────────────────────────

# Fetch the highest semver tag matching fcdev/vX.Y.Z. Anonymous GitHub API
# requests are limited to 60/hour per source IP — fine for a manual install
# but documented here in case someone tries to script bulk installs.
fetch_latest_version() {
    # Filtering by prefix (rather than using /releases/latest) is essential
    # because the same repo also tags `laravel-sdk/v…` and `typescript-sdk/v…`,
    # which would otherwise show up as "latest".
    api="https://api.github.com/repos/${REPO}/releases?per_page=100"
    body=$(curl -fsSL "$api") || err "could not query GitHub API at $api"

    # POSIX-pure parsing of the JSON. Each tag_name lands on its own line, we
    # keep only `fcdev/v*` ones, strip the prefix, keep clean X.Y.Z, and pick
    # the max semver via a zero-padded sort key.
    version=$(
        printf '%s' "$body" \
            | tr ',' '\n' \
            | grep '"tag_name"' \
            | sed -e 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//' -e 's/".*//' \
            | grep "^${TAG_PREFIX}" \
            | sed "s|^${TAG_PREFIX}||" \
            | awk -F. '
                /^[0-9]+\.[0-9]+\.[0-9]+$/ {
                    printf "%010d%010d%010d %s\n", $1, $2, $3, $0
                }' \
            | sort -r \
            | awk 'NR==1 {print $2}'
    )

    if [ -z "$version" ]; then
        err "no fcdev releases found at $api (looking for tags matching ${TAG_PREFIX}X.Y.Z)"
    fi
    printf '%s' "$version"
}

# ─── install destination ───────────────────────────────────────────────────

default_install_dir() {
    # Prefer /usr/local/bin if we can write to it without sudo (common dev
    # workstation case). Otherwise install into the user-local equivalent — no
    # sudo prompt during a curl-pipe-sh install, which is what most users want.
    if [ -w /usr/local/bin ] 2>/dev/null; then
        printf '%s' "/usr/local/bin"
    else
        printf '%s' "$HOME/.local/bin"
    fi
}

# ─── checksum verification ─────────────────────────────────────────────────

# A missing/unreadable checksum is fatal unless the user opted out explicitly.
checksum_unavailable() {
    if [ "${FCDEV_INSECURE_SKIP_CHECKSUM:-0}" = "1" ]; then
        warn "$1 — continuing without checksum verification (FCDEV_INSECURE_SKIP_CHECKSUM=1)"
    else
        err "$1; refusing to install an unverified archive (set FCDEV_INSECURE_SKIP_CHECKSUM=1 to accept the risk)"
    fi
}

verify_sha256() {
    file=$1
    sidecar=$2
    expected=$(awk 'NR==1 {print tolower($1)}' "$sidecar")
    case "$expected" in
        ????????????????????????????????????????????????????????????????) ;;
        *) checksum_unavailable "the SHA-256 sidecar is empty or not a SHA-256 digest"; return 0 ;;
    esac
    case "$expected" in
        *[!0-9a-f]*) checksum_unavailable "the SHA-256 sidecar is not a SHA-256 digest"; return 0 ;;
    esac
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        checksum_unavailable "no sha256 tool found (sha256sum or shasum)"
        return 0
    fi
    if [ "$expected" != "$actual" ]; then
        err "checksum mismatch for $file (expected $expected, got $actual)"
    fi
}

# ─── signature verification (Linux archives are cosign-signed) ─────────────

# The certificate identity is the release workflow's own path in the repository
# it runs in ($REPO, the one the archive is downloaded from), at the tag it was
# pushed on or, for a manual re-run, at main; the issuer is GitHub Actions'.
verify_cosign() {
    archive=$1
    sig_url=$2
    pem_url=$3
    if ! command -v cosign >/dev/null 2>&1; then
        info "Signature not verified: cosign is not installed (install it to verify the release signature)"
        return 0
    fi
    if ! curl -fsSL -o "$archive.sig" "$sig_url" || ! curl -fsSL -o "$archive.pem" "$pem_url"; then
        info "Signature not verified: the release publishes no .sig/.pem for this archive"
        return 0
    fi
    # shellcheck disable=SC2016 # the sed program is a literal, not a shell expansion
    repo_re=$(printf '%s' "$REPO" | sed 's/[].*[^$()+?{|\\]/\\&/g')
    identity="^https://github\\.com/${repo_re}/\\.github/workflows/release-fcdev\\.yml@refs/(tags/fcdev/v[0-9].*|heads/main)\$"
    info "Verifying cosign signature"
    if ! cosign verify-blob \
        --signature "$archive.sig" \
        --certificate "$archive.pem" \
        --certificate-identity-regexp "$identity" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        "$archive" >/dev/null 2>&1; then
        err "cosign signature verification FAILED for $archive — do not use this archive"
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
        info "Looking up the latest fcdev release"
        version=$(fetch_latest_version)
        info "Latest: $version"
    fi

    install_dir=${FC_DEV_INSTALL_DIR:-$(default_install_dir)}
    info "Installing into $install_dir"

    # Idempotency: if the same version is already at the destination, exit
    # quietly. Set FC_DEV_FORCE=1 to override (useful when debugging a
    # corrupted install).
    if [ -x "$install_dir/$BIN" ] && [ "${FC_DEV_FORCE:-0}" != "1" ]; then
        existing=$("$install_dir/$BIN" version 2>/dev/null | awk '{print $2}' || true)
        if [ "$existing" = "$version" ]; then
            info "fcdev v$version is already installed at $install_dir/$BIN — nothing to do."
            info "Set FC_DEV_FORCE=1 to reinstall."
            return 0
        fi
    fi

    asset="fcdev-v${version}-${target}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${TAG_PREFIX}${version}/${asset}"
    sha_url="${url}.sha256"

    tmp=$(mktemp -d 2>/dev/null || mktemp -d -t fcdev-install)
    # Defensive cleanup in case the trap doesn't fire under a piped sh.
    trap 'rm -rf "$tmp"' EXIT INT HUP TERM

    info "Downloading $asset"
    if ! curl -fL --progress-bar -o "$tmp/$asset" "$url"; then
        err "download failed: $url"
    fi
    if ! curl -fsSL -o "$tmp/$asset.sha256" "$sha_url"; then
        checksum_unavailable "no readable SHA-256 sidecar at $sha_url"
    else
        info "Verifying SHA256"
        verify_sha256 "$tmp/$asset" "$tmp/$asset.sha256"
    fi
    if [ "${target%%-*}" = "linux" ]; then
        verify_cosign "$tmp/$asset" "${url}.sig" "${url}.pem"
    fi

    info "Extracting"
    tar -xzf "$tmp/$asset" -C "$tmp"
    extracted_bin="$tmp/fcdev-v${version}-${target}/$BIN"
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

    # macOS quarantines anything downloaded by curl. Strip the attribute so the
    # first launch doesn't hit "fcdev cannot be opened because it is from an
    # unidentified developer".
    if [ "$(uname -s)" = "Darwin" ]; then
        xattr -d com.apple.quarantine "$install_dir/$BIN" 2>/dev/null || true
    fi

    info "Installed: $install_dir/$BIN"
    "$install_dir/$BIN" version 2>/dev/null || true

    # PATH hint — if the install dir isn't on PATH the user will scratch their
    # head trying to invoke `fcdev`. Tell them explicitly with the right
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

    info "Done. Run: fcdev"
}

main "$@"
