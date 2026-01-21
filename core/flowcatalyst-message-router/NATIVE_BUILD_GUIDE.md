# Native Image Build Guide

## Overview

This guide explains how to build native executables for FlowCatalyst Message Router for Windows, Linux, and macOS using both local builds and GitHub Actions.

## Table of Contents

- [Quick Start](#quick-start)
- [Local Builds](#local-builds)
- [GitHub Actions](#github-actions)
- [Build Types](#build-types)
- [Troubleshooting](#troubleshooting)

---

## Quick Start

### Using GitHub Actions (Recommended)

1. Push to `main` branch or create a tag:
```bash
git tag v1.0.0
git push origin v1.0.0
```

2. Download binaries from GitHub Actions artifacts or releases

### Local Build (Your Platform)

```bash
# Developer build (with embedded SQLite queue)
./gradlew nativeBuildDev

# Production build (SQS/ActiveMQ)
./gradlew nativeBuild

# Binary location:
# flowcatalyst-message-router/build/flowcatalyst-message-router-1.0.0-SNAPSHOT-runner[.exe]
```

---

## Local Builds

### Prerequisites

#### All Platforms
- **GraalVM 21** or later with `native-image` component
- **Java 21** (GraalVM JDK)

#### macOS
```bash
# Install GraalVM
brew install --cask graalvm/tap/graalvm-jdk21

# Add to PATH (add to ~/.zshrc or ~/.bash_profile)
export JAVA_HOME=/Library/Java/JavaVirtualMachines/graalvm-jdk-21/Contents/Home
export PATH=$JAVA_HOME/bin:$PATH

# Verify
java --version
native-image --version
```

#### Linux (Ubuntu/Debian)
```bash
# Install GraalVM
curl -L https://github.com/graalvm/graalvm-ce-builds/releases/download/jdk-21.0.1/graalvm-community-jdk-21.0.1_linux-x64_bin.tar.gz -o graalvm.tar.gz
tar -xzf graalvm.tar.gz
sudo mv graalvm-community-openjdk-21.0.1 /usr/lib/jvm/graalvm-21

# Add to PATH (add to ~/.bashrc)
export JAVA_HOME=/usr/lib/jvm/graalvm-21
export PATH=$JAVA_HOME/bin:$PATH

# Install native-image component
gu install native-image

# Install build tools
sudo apt-get update
sudo apt-get install build-essential zlib1g-dev

# Verify
java --version
native-image --version
```

#### Windows
1. Download and install [GraalVM JDK 21](https://www.graalvm.org/downloads/)
2. Install [Visual Studio Build Tools 2019+](https://visualstudio.microsoft.com/downloads/)
   - Select "Desktop development with C++"
3. Add GraalVM to PATH:
   ```powershell
   # In PowerShell (Admin)
   $env:JAVA_HOME = "C:\Program Files\Java\graalvm-jdk-21"
   $env:PATH = "$env:JAVA_HOME\bin;$env:PATH"

   # Make permanent (System Properties -> Environment Variables)
   ```
4. Install `native-image`:
   ```powershell
   gu.cmd install native-image
   ```
5. Open "x64 Native Tools Command Prompt for VS 2019" for builds

### Build Commands

#### Developer Build (with Embedded SQLite Queue)
```bash
# Gradle
./gradlew nativeBuildDev

# Or directly with Quarkus
./gradlew build -Dquarkus.package.type=native -Dquarkus.profile=embedded-dev
```

This creates a self-contained binary with embedded SQLite queue for zero-setup development.

**Output:**
- Linux/macOS: `build/flowcatalyst-message-router-1.0.0-SNAPSHOT-runner`
- Windows: `build\flowcatalyst-message-router-1.0.0-SNAPSHOT-runner.exe`

#### Production Build (SQS/ActiveMQ)
```bash
# Gradle
./gradlew nativeBuild

# Or directly
./gradlew build -Dquarkus.package.type=native
```

This creates a production binary that requires external queue brokers (SQS or ActiveMQ).

### Run the Binary

```bash
# Linux/macOS
chmod +x build/flowcatalyst-message-router-1.0.0-SNAPSHOT-runner
./build/flowcatalyst-message-router-1.0.0-SNAPSHOT-runner

# Windows
.\build\flowcatalyst-message-router-1.0.0-SNAPSHOT-runner.exe
```

### Build Options

```bash
# Fast build (less optimization)
./gradlew build -Dquarkus.package.type=native -Dquarkus.native.additional-build-args="-O1"

# Small binary (more optimization, slower build)
./gradlew build -Dquarkus.package.type=native -Dquarkus.native.additional-build-args="-O3"

# Verbose output
./gradlew build -Dquarkus.package.type=native -Dquarkus.native.additional-build-args="--verbose"

# Skip tests
./gradlew build -Dquarkus.package.type=native -x test
```

---

## GitHub Actions

### Automatic Builds

The repository is configured to automatically build native images for all platforms on:
- **Push to `main`** - Builds all platforms, artifacts available for 30 days
- **Pull requests** - Builds all platforms for testing
- **Version tags** (`v*`) - Creates GitHub release with binaries

### Manual Build Trigger

1. Go to **Actions** tab in GitHub
2. Select **"Build Native Images"** workflow
3. Click **"Run workflow"**
4. Choose options:
   - Branch: `main` (or your branch)
   - Embedded Queue: `true` (developer) or `false` (production)
5. Click **"Run workflow"**

### Download Artifacts

#### From Actions
1. Go to **Actions** tab
2. Click on the workflow run
3. Scroll to **"Artifacts"** section
4. Download:
   - `flowcatalyst-message-router-linux-amd64`
   - `flowcatalyst-message-router-windows-amd64`
   - `flowcatalyst-message-router-macos-amd64`

#### From Releases (Tagged Builds)
1. Go to **Releases** tab
2. Find your version (e.g., `v1.0.0`)
3. Download binary for your platform
4. Verify checksum:
   ```bash
   sha256sum -c flowcatalyst-message-router-linux-amd64.sha256
   ```

### Workflow Configuration

File: `.github/workflows/native-build.yml`

**Key features:**
- Parallel builds on 3 platforms
- GraalVM caching for faster builds
- Automated testing of binaries
- Checksum generation
- GitHub releases on tags
- Build summary in workflow output

---

## Build Types

### Developer Build (Embedded SQLite Queue)

**Includes:**
- Embedded SQLite queue with SQS FIFO semantics
- REST API for queue operations
- Zero external dependencies
- Perfect for local development

**Use when:**
- Distributing to developers
- Creating demo/sandbox environments
- Need quick setup without Docker

**Configuration:**
```properties
message-router.queue-type=EMBEDDED
message-router.embedded.visibility-timeout-seconds=30
message-router.embedded.receive-timeout-ms=1000
```

**Binary size:** ~60-80 MB

### Production Build

**Includes:**
- SQS client
- ActiveMQ client
- No embedded queue

**Use when:**
- Deploying to production
- Using AWS SQS
- Using external ActiveMQ broker

**Configuration:**
```properties
message-router.queue-type=SQS  # or ACTIVEMQ
```

**Binary size:** ~50-70 MB

---

## Build Performance

### Expected Build Times

| Platform | Build Time | Binary Size |
|----------|------------|-------------|
| Linux | 5-8 minutes | ~60 MB |
| Windows | 8-12 minutes | ~70 MB |
| macOS | 6-10 minutes | ~65 MB |

*Times are for GitHub Actions. Local builds may be faster with warm Gradle cache.*

### Build Tips

1. **First build is slow** - Subsequent builds are faster with Gradle cache
2. **Use `-x test`** to skip tests and speed up builds
3. **Use `-O1`** for faster builds during development
4. **GitHub Actions caches** dependencies between runs

---

## Troubleshooting

### "native-image: command not found"

**Solution:**
```bash
# Install native-image component
gu install native-image

# Or on Windows
gu.cmd install native-image
```

### Windows: "Cannot find vcvarsall.bat"

**Solution:** Install Visual Studio Build Tools and use "x64 Native Tools Command Prompt"

```powershell
# Or set up environment manually
"C:\Program Files (x86)\Microsoft Visual Studio\2019\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
```

### macOS: "ld: library not found"

**Solution:** Install Xcode Command Line Tools
```bash
xcode-select --install
```

### Linux: "gcc: command not found"

**Solution:**
```bash
sudo apt-get update
sudo apt-get install build-essential
```

### Out of Memory During Build

**Solution:** Increase memory for native-image
```bash
./gradlew build -Dquarkus.package.type=native \
  -Dquarkus.native.additional-build-args="-J-Xmx8g"
```

### Binary Fails to Start

**Check:**
1. File permissions: `chmod +x <binary>`
2. Architecture match: `file <binary>` (should show your CPU architecture)
3. Required libraries: `ldd <binary>` (Linux) to see missing dependencies

### GitHub Actions: Build Fails

**Common fixes:**
1. Check Gradle cache hasn't corrupted
2. Re-run workflow (might be transient issue)
3. Check workflow logs for specific error
4. Verify `build.gradle.kts` syntax is valid

---

## Advanced

### Cross-Platform Builds

**Not supported natively.** You must build on each target platform.

**Options:**
1. Use GitHub Actions (builds all platforms automatically)
2. Use CI/CD with runners for each platform
3. Use cloud build services

### Container-Based Builds

```bash
# Linux native image using Docker
./gradlew build -Dquarkus.package.type=native \
  -Dquarkus.native.container-build=true

# Uses: quay.io/quarkus/ubi-quarkus-mandrel-builder-image
```

**Note:** This only works for Linux targets.

### Static Linking (Linux)

For maximum portability on Linux:

```bash
./gradlew build -Dquarkus.package.type=native \
  -Dquarkus.native.additional-build-args="--static,--libc=musl"
```

Requires `musl` toolchain installed.

### Optimizing Binary Size

```bash
# Enable all optimizations
./gradlew build -Dquarkus.package.type=native \
  -Dquarkus.native.additional-build-args="-march=native,-O3,--gc=serial"

# Remove debug info
./gradlew build -Dquarkus.package.type=native \
  -Dquarkus.native.additional-build-args="--no-fallback,-H:-GenerateDebugInfo"
```

---

## Distribution

### For End Users

1. **Build all platforms** using GitHub Actions
2. **Download binaries** from releases
3. **Package** with documentation:
   ```
   flowcatalyst-message-router/
   ├── flowcatalyst-message-router-linux-amd64
   ├── flowcatalyst-message-router-windows-amd64.exe
   ├── flowcatalyst-message-router-macos-amd64
   ├── README.md
   ├── ARCHITECTURE.md
   └── application.properties (optional config)
   ```

4. **Distribute** via:
   - GitHub releases
   - Internal artifact repository
   - Direct download links

### Example Release Script

```bash
#!/bin/bash
# build-release.sh

VERSION="1.0.0"

# Tag and push
git tag "v${VERSION}"
git push origin "v${VERSION}"

echo "GitHub Actions will build binaries automatically."
echo "Check: https://github.com/YOUR_ORG/flowcatalyst/actions"
```

---

## See Also

- [Architecture Documentation](./ARCHITECTURE.md)
- [GraalVM Native Image](https://www.graalvm.org/latest/reference-manual/native-image/)
- [Quarkus Native Guide](https://quarkus.io/guides/building-native-image)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
