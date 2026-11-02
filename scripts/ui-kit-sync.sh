#!/bin/sh
# Rebuild the platform's UI stylesheet (internal/ui/uiassets/static/platform.css).
#
# The kit itself (components, datastar.js, icon sprite, islands, fixtures) is the
# framework's published Go module github.com/flowcatalyst/framework/go/ui; this
# repo no longer vendors any of it. What it still builds is its own Tailwind
# stylesheet, because the module's kit.css cannot carry the classes the
# platform's pages use. See docs/ui/README.md, "Refreshing the kit".
#
#   scripts/ui-kit-sync.sh                 # theme from ../flowcatalyst-framework/ui-kit
#   KIT_DIR=/path/to/ui-kit scripts/ui-kit-sync.sh
#
# What it does:
#   1. resolves the module's directory (go list -m) for the kit's sources and
#      version;
#   2. runs the framework ui-kit's own build.sh (downloads its pinned Tailwind
#      standalone CLI into <kit>/.bin on first use: no Node, no npm);
#   3. builds platform.css from the kit's theme (theme/kit.css, @import) with
#      @source of this repo's *.templ/*.go under internal/ui AND the module's
#      kit sources (*.templ/*.go of kit/ and views/), so both the kit's classes
#      and ours are emitted;
#   4. records the module version and the Tailwind version in
#      internal/ui/uiassets/KIT_SOURCE.
#
# The theme CSS comes from the framework repo checkout (the module does not ship
# the Tailwind theme sources); the module version recorded is the one go.mod
# requires. Keep the checkout at the commit the module tag points to.
#
# POSIX sh; needs go and git.
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
kit=${KIT_DIR:-$repo/../flowcatalyst-framework/ui-kit}
if [ ! -f "$kit/build.sh" ] || [ ! -f "$kit/theme/kit.css" ]; then
  echo "ui-kit not found at $kit (set KIT_DIR)" >&2
  exit 1
fi
kit=$(cd "$kit" && pwd)

mod=github.com/flowcatalyst/framework/go/ui
moddir=$(cd "$repo" && go list -m -f '{{.Dir}}' "$mod")
modver=$(cd "$repo" && go list -m -f '{{.Version}}' "$mod")
if [ -z "$moddir" ] || [ ! -d "$moddir/kit" ]; then
  echo "module $mod not available (run go mod download)" >&2
  exit 1
fi

static=$repo/internal/ui/uiassets/static
mkdir -p "$static"

"$kit/build.sh" >/dev/null 2>&1
tw=$kit/.bin/tailwindcss
tw_version=$("$tw" --help 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/ui.css" <<CSS
@import "$kit/theme/kit.css";
@source "$repo/internal/ui/**/*.templ";
@source "$repo/internal/ui/**/*.go";
@source not "$repo/internal/ui/**/*_test.go";
@source "$moddir/kit/**/*.templ";
@source "$moddir/kit/**/*.go";
@source "$moddir/views/**/*.templ";
@source "$moddir/views/**/*.go";
@source not "$moddir/**/*_test.go";
CSS
"$tw" -i "$tmp/ui.css" -o "$static/platform.css" --minify >/dev/null 2>&1

kitrev=$(git -C "$kit" rev-parse HEAD 2>/dev/null || echo unknown)
dirty=""
if [ -n "$(git -C "$kit" status --porcelain -- . 2>/dev/null)" ]; then
  dirty=" (uncommitted theme changes present)"
  echo "warning: $kit has uncommitted changes; commit them in the framework repo before committing this output" >&2
fi
cat > "$repo/internal/ui/uiassets/KIT_SOURCE" <<REC
kit-module: $mod $modver
theme-repo: flowcatalyst-framework (ui-kit/theme)
theme-commit: $kitrev$dirty
tailwind: $tw_version
REC

echo "platform.css built (module $modver, theme @ $kitrev$dirty, tailwind $tw_version)"
