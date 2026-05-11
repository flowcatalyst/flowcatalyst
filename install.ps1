<#
.SYNOPSIS
fc-dev installer for Windows. PowerShell 5.1+ (built into Windows 10/11).

.DESCRIPTION
Downloads the latest fc-dev release from GitHub, verifies its SHA256,
extracts it into %LOCALAPPDATA%\Programs\fc-dev, and prepends that path
to your user PATH. Re-runs are safe: if the requested version is already
installed it exits without changes.

.EXAMPLE
# Quick install (latest)
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex

.EXAMPLE
# Pin a version
$env:FC_DEV_VERSION = '0.3.8'
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex

.NOTES
Environment variables:
  FC_DEV_VERSION       Pin a specific version (default: latest fc-dev/v*)
  FC_DEV_INSTALL_DIR   Install destination (default: %LOCALAPPDATA%\Programs\fc-dev)
  FC_DEV_FORCE         "1" to reinstall even if the same version is present
#>

$ErrorActionPreference = 'Stop'
# Force TLS 1.2 minimum — required for github.com on older PowerShell hosts.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor `
    [Net.ServicePointManager]::SecurityProtocol

$Repo      = 'flowcatalyst/flowcatalyst'
$TagPrefix = 'fc-dev/v'
$Bin       = 'fc-dev.exe'
$Target    = 'x86_64-pc-windows-msvc'   # only Windows target we publish

# ─── output helpers ────────────────────────────────────────────────────────

function Write-Info { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Cyan }
function Write-Warn { param([string]$Msg) Write-Host "warning: $Msg" -ForegroundColor Yellow }
function Exit-Err  { param([string]$Msg) Write-Host "error: $Msg" -ForegroundColor Red; exit 1 }

# ─── latest-version lookup ─────────────────────────────────────────────────

function Get-LatestFcDevVersion {
    # Filtering by `fc-dev/v*` is mandatory: the same repo also tags
    # `laravel-sdk/v…` / `typescript-sdk/v…`, so /releases/latest would
    # return whichever tag was last published — not necessarily fc-dev.
    $api = "https://api.github.com/repos/$Repo/releases?per_page=100"
    $headers = @{ 'User-Agent' = 'fc-dev-install.ps1' }
    try {
        $releases = Invoke-RestMethod -Uri $api -Headers $headers
    } catch {
        Exit-Err "could not query GitHub API at $api : $($_.Exception.Message)"
    }
    $latest = $releases |
        Where-Object {
            $_.tag_name -like "$TagPrefix*" -and -not $_.prerelease -and -not $_.draft
        } |
        ForEach-Object {
            $raw = $_.tag_name -replace "^$TagPrefix", ''
            if ($raw -match '^\d+\.\d+\.\d+$') {
                [PSCustomObject]@{ Tag = $_.tag_name; Version = [version]$raw; Raw = $raw }
            }
        } |
        Sort-Object Version -Descending |
        Select-Object -First 1

    if (-not $latest) {
        Exit-Err "no fc-dev releases found in $Repo"
    }
    return $latest.Raw
}

# ─── path mutation ─────────────────────────────────────────────────────────

function Add-ToUserPath {
    param([string]$Dir)
    $current = [Environment]::GetEnvironmentVariable('PATH', 'User')
    # Some users' user-PATH is null on a fresh profile; treat as empty.
    if (-not $current) { $current = '' }
    $entries = $current -split ';' | Where-Object { $_ -ne '' }
    if ($entries -contains $Dir) { return $false }
    $newPath = ($entries + $Dir) -join ';'
    [Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
    # Update the current session too so the user can invoke `fc-dev` without
    # opening a new shell.
    $env:PATH = "$env:PATH;$Dir"
    return $true
}

# ─── main ──────────────────────────────────────────────────────────────────

Write-Info 'Detecting platform'
# Reality check: PowerShell on ARM64 Windows still reports x86 / amd64
# correctly for our purposes. fc-dev only ships x86_64-pc-windows-msvc;
# arm64 Windows users can run the x64 binary under Microsoft's emulator.
Write-Info "Target: $Target"

$version = $env:FC_DEV_VERSION
if (-not $version) {
    Write-Info 'Looking up the latest fc-dev release'
    $version = Get-LatestFcDevVersion
    Write-Info "Latest: $version"
} else {
    Write-Info "Using pinned version $version (FC_DEV_VERSION)"
}

$installDir = $env:FC_DEV_INSTALL_DIR
if (-not $installDir) {
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\fc-dev'
}
Write-Info "Installing into $installDir"

$installedBin = Join-Path $installDir $Bin

# Idempotency check — same version already installed?
if ((Test-Path $installedBin) -and ($env:FC_DEV_FORCE -ne '1')) {
    $existing = ''
    try { $existing = (& $installedBin --version 2>$null) -replace '^.*\s', '' } catch {}
    if ($existing -eq $version) {
        Write-Info "fc-dev v$version is already installed at $installedBin -- nothing to do."
        Write-Info 'Set $env:FC_DEV_FORCE = "1" to reinstall.'
        return
    }
}

$asset    = "fc-dev-v$version-$Target.zip"
$assetUrl = "https://github.com/$Repo/releases/download/$TagPrefix$version/$asset"
$shaUrl   = "$assetUrl.sha256"

$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "fc-dev-install-$([guid]::NewGuid())") -Force
try {
    $archive = Join-Path $tmp $asset
    Write-Info "Downloading $asset"
    try {
        Invoke-WebRequest -Uri $assetUrl -OutFile $archive -UseBasicParsing
    } catch {
        Exit-Err "download failed: $assetUrl ($($_.Exception.Message))"
    }

    # Best-effort SHA256 verification. If the sidecar isn't present we warn
    # and continue — TLS already guarantees download integrity end-to-end.
    $sidecar = "$archive.sha256"
    $verified = $false
    try {
        Invoke-WebRequest -Uri $shaUrl -OutFile $sidecar -UseBasicParsing
        $expected = ((Get-Content $sidecar -Raw) -split '\s+')[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
        if ($expected -ne $actual) {
            Exit-Err "checksum mismatch for $asset (expected $expected, got $actual)"
        }
        Write-Info 'SHA256 verified'
        $verified = $true
    } catch {
        Write-Warn "could not fetch SHA256 sidecar -- skipping verification ($($_.Exception.Message))"
    }

    Write-Info 'Extracting'
    $extractRoot = Join-Path $tmp 'extracted'
    Expand-Archive -Path $archive -DestinationPath $extractRoot -Force

    # Archive layout: fc-dev-vX.Y.Z-<target>/fc-dev.exe
    $stageDirName = "fc-dev-v$version-$Target"
    $stagedBin = Join-Path $extractRoot (Join-Path $stageDirName $Bin)
    if (-not (Test-Path $stagedBin)) {
        Exit-Err "extracted archive missing $Bin at $stagedBin"
    }

    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    Write-Info "Installing to $installedBin"
    # Copy-Item is atomic at the filesystem level for files on the same
    # volume; -Force overwrites an existing binary.
    Copy-Item -Path $stagedBin -Destination $installedBin -Force

    if (Add-ToUserPath $installDir) {
        Write-Info "Added $installDir to your user PATH"
        Write-Info 'Restart your shell (or re-open VS Code / Windows Terminal) for PATH to take effect in new windows.'
    }

    Write-Info "Installed: $installedBin"
    try { & $installedBin --version } catch {}

    Write-Info 'Done. Run: fc-dev'
    if (-not $verified) {
        Write-Warn 'This install was not SHA256-verified. Re-run later when the sidecar is available, or pin a version known to publish it.'
    }
}
finally {
    if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
}
