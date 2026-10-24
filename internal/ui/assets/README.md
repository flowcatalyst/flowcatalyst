# internal/ui/assets

Static files for the server-rendered UI, embedded into the binary and served
under `/ui/assets/` (hashed names, one-year immutable cache; plain names with
ETag revalidation). No CDN is used at run time.

| File | Source | Licence |
|---|---|---|
| `static/kit.css` | Built by `scripts/ui-kit-sync.sh` from `flowcatalyst-framework/ui-kit` (`theme/kit.css` + this repo's `internal/ui/**/*.{templ,go}`) with the kit's pinned Tailwind standalone CLI | Topcoat UI MIT notice in `static/KIT-THIRD-PARTY-NOTICES.md`; Tailwind's banner is kept in the file |
| `static/lucide-sprite.svg` | Copied from `ui-kit/icons/` | ISC/MIT, `static/LICENSE-lucide.txt` |
| `static/datastar.js` | Datastar **1.0.4**, `https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.4/bundles/datastar.js` (the URL the kit gallery loads), unmodified | MIT, `static/LICENSE-datastar.md` |
| `static/mermaid.min.js`, `static/fc-diagram.js` | Mermaid **12.1.0** `dist/mermaid.min.js` (npm), unmodified; `fc-diagram.js` is ours | MIT, `static/LICENSE-mermaid.md` |
| `static/fc-code-editor.js` | CodeMirror 6 bundle built by `tools/vendor-codemirror` (docs/ui/README.md, Islands) | MIT, `static/LICENSE-codemirror.md` |

`datastar.js` sha256: `727844adfc825ee651fb93c544a2a739986f9a21820a94524b35f0cac470cf91`
(also `assets.DatastarSHA256`; `TestDatastarIsThePinnedFile` fails on any other content).
The upstream source map (`datastar.js.map`) is not vendored; browsers' devtools
will report it missing, nothing else.

`KIT_SOURCE` records the kit commit, Tailwind version and Datastar pin of the
last sync. Refresh with `make ui-kit-sync` (see `docs/ui/README.md`), then
commit the changed files together with `KIT_SOURCE`.
