#!/bin/sh
# Rebuild the vendored UI kit assets in internal/ui from the framework
# repository's ui-kit (the kit's specs are the authority; this repo carries
# only built output, committed). See docs/ui/README.md, "Refreshing the kit".
#
#   scripts/ui-kit-sync.sh                 # kit at ../flowcatalyst-framework/ui-kit
#   KIT_DIR=/path/to/ui-kit scripts/ui-kit-sync.sh
#
# What it does:
#   1. runs the kit's own build.sh, which downloads its pinned Tailwind
#      standalone CLI into <kit>/.bin on first use (no Node, no npm);
#   2. builds internal/ui/assets/static/kit.css from the kit's entry file
#      (theme/kit.css, which scans the kit's gallery and specs) PLUS this
#      repo's *.templ and *.go files under internal/ui (not tests or
#      testdata), so classes used only by our pages are emitted too;
#   3. copies the icon sprite and the licence notices;
#   4. copies the conformance fixtures into internal/ui/testdata/fixtures;
#   5. checks the vendored datastar.js against its pinned sha256 (downloads
#      it from the pinned URL when missing);
#   6. records the kit commit, Tailwind version and the sprite's icon count and
#      sha256 in internal/ui/assets/KIT_SOURCE (assets_test checks the sprite).
#
# POSIX sh; needs git, curl (only when datastar.js is missing) and shasum or sha256sum.
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
kit=${KIT_DIR:-$repo/../flowcatalyst-framework/ui-kit}
if [ ! -f "$kit/build.sh" ] || [ ! -f "$kit/theme/kit.css" ]; then
  echo "ui-kit not found at $kit (set KIT_DIR)" >&2
  exit 1
fi
kit=$(cd "$kit" && pwd)

static=$repo/internal/ui/assets/static
fixtures=$repo/internal/ui/testdata/fixtures
mkdir -p "$static"

DATASTAR_VERSION=1.0.4
DATASTAR_URL="https://cdn.jsdelivr.net/gh/starfederation/datastar@$DATASTAR_VERSION/bundles/datastar.js"
DATASTAR_LICENSE_URL="https://cdn.jsdelivr.net/gh/starfederation/datastar@$DATASTAR_VERSION/LICENSE.md"
# Keep in sync with assets.DatastarSHA256 (a Go test checks the embedded file).
DATASTAR_SHA256=727844adfc825ee651fb93c544a2a739986f9a21820a94524b35f0cac470cf91

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# 1. The kit's build (also fetches the Tailwind CLI).
"$kit/build.sh" >/dev/null 2>&1
tw=$kit/.bin/tailwindcss
tw_version=$("$tw" --help 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)

# 2. Our stylesheet.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/ui.css" <<EOF
@import "$kit/theme/kit.css";
@source "$repo/internal/ui/**/*.templ";
@source "$repo/internal/ui/**/*.go";
@source not "$repo/internal/ui/**/*_test.go";
@source not "$repo/internal/ui/testdata";
EOF
"$tw" -i "$tmp/ui.css" -o "$static/kit.css" --minify >/dev/null 2>&1

# 3. Sprite and notices.
cp "$kit/icons/lucide-sprite.svg" "$static/lucide-sprite.svg"
cp "$kit/icons/LICENSE-lucide.txt" "$static/LICENSE-lucide.txt"
cp "$kit/THIRD-PARTY-NOTICES.md" "$static/KIT-THIRD-PARTY-NOTICES.md"

# 4. Fixtures (language-neutral; the Go conformance test reads this copy).
if [ -d "$kit/fixtures" ]; then
  rm -rf "$fixtures"
  mkdir -p "$(dirname "$fixtures")"
  cp -R "$kit/fixtures" "$fixtures"
fi

# 5. Datastar, pinned.
if [ ! -f "$static/datastar.js" ]; then
  curl -fsSL -o "$static/datastar.js" "$DATASTAR_URL"
fi
if [ ! -f "$static/LICENSE-datastar.md" ]; then
  curl -fsSL -o "$static/LICENSE-datastar.md" "$DATASTAR_LICENSE_URL"
fi
got=$(sha256 "$static/datastar.js")
if [ "$got" != "$DATASTAR_SHA256" ]; then
  echo "datastar.js sha256 mismatch: got $got, want $DATASTAR_SHA256" >&2
  exit 1
fi

# 6. Provenance.
commit=$(git -C "$kit" rev-parse HEAD)
dirty=""
if [ -n "$(git -C "$kit" status --porcelain -- . 2>/dev/null)" ]; then
  dirty=" (uncommitted kit changes present)"
  echo "warning: $kit has uncommitted changes; commit them in the framework repo before committing this output" >&2
fi
sprite_icons=$(grep -c '<symbol id="i-' "$static/lucide-sprite.svg")
sprite_sha=$(sha256 "$static/lucide-sprite.svg")
cat > "$repo/internal/ui/assets/KIT_SOURCE" <<EOF
kit-repo: flowcatalyst-framework (ui-kit/)
kit-commit: $commit$dirty
tailwind: $tw_version
datastar: $DATASTAR_VERSION sha256 $DATASTAR_SHA256
sprite: $sprite_icons icons sha256 $sprite_sha
EOF

echo "ui-kit synced from $kit @ $commit$dirty (tailwind $tw_version)"
