# Adding PostGIS to the fc-dev embedded Postgres

`fc-dev` runs a **bundled, self-contained PostgreSQL** via
[`fergusstrange/embedded-postgres`], which downloads the **Zonky**
`embedded-postgres-binaries`. Those are *vanilla* PostgreSQL builds — they ship
the standard contrib extensions (`hstore`, `pg_trgm`, `bloom`, …) but **not
PostGIS**, and there is no flag or env that bundles it. `BinaryRepositoryURL`
only swaps the Maven mirror serving the same vanilla archive.

PostGIS is, however, "just files": a loadable module plus its extension
SQL/control files. So you can **transplant** a matching PostGIS build into the
embedded tree after first run, then `CREATE EXTENSION postgis;`.

> If your app's database genuinely needs PostGIS in anger, prefer running your
> own Postgres (e.g. the `postgis/postgis` Docker image) and pointing the
> outbox poller at it (`fc-dev outbox --source-db-url=…`). The transplant below
> is for keeping the all-in-one embedded dev loop while gaining PostGIS.

## The version that must match

The embedded server is **PostgreSQL 18** (`embedded-postgres` `DefaultConfig`
→ `V18`; fc-dev does not override it). **Everything you install must be PostGIS
built for PostgreSQL 18** on your OS + CPU. Mixing majors (e.g. a PG16 PostGIS)
fails to load with `undefined symbol` / `incompatible library`.

### Golden rules

1. **Match the major: PostgreSQL 18.** Verify against the embedded binary, not
   your system Postgres (Step 0).
2. **Match OS + arch + libc** (the exact build the loader will `dlopen`).
3. **Keep PostGIS's runtime deps installed** (GEOS, PROJ, GDAL, libxml2,
   protobuf-c, …) — the module links against them at load time.
4. **Do it after the first `fc-dev start`** (so the tree exists), then restart
   fc-dev and run `CREATE EXTENSION postgis;`.

### Which CPU arch the embedded build uses

`embedded-postgres` picks the Zonky artifact like this — install the PostGIS
build that matches the **right column**:

| Your platform            | Embedded PG build it runs        | Install PostGIS for |
|--------------------------|----------------------------------|---------------------|
| macOS Apple Silicon      | `darwin-arm64v8` (native)        | macOS **arm64**     |
| macOS Intel              | `darwin-amd64`                   | macOS **x86_64**    |
| Linux x86_64 (glibc)     | `linux-amd64`                    | Linux **amd64**     |
| Linux x86_64 (musl/Alpine)| `linux-amd64-alpine`            | Alpine **amd64**    |
| Linux arm64 (glibc)      | `linux-arm64v8`                  | Linux **arm64**     |
| Windows x64              | `windows-amd64`                  | Windows **x64**     |
| Windows on ARM           | ⚠️ no Zonky build — see note      | run the x64 fc-dev (emulated) → Windows **x64** |

> **Windows ARM:** Zonky publishes no `windows-arm64` binary, so a *native*
> arm64 `fc-dev` cannot start the embedded DB at all. Run the **x64** `fc-dev`
> under emulation; it pulls `windows-amd64` Postgres, and you install **x64**
> PostGIS.

## Step 0 — Locate and verify the embedded tree

The install lives under `<user-cache>/flowcatalyst/embedded-pg/bin`, containing
`bin/`, `lib/`, `share/`:

| OS      | Path |
|---------|------|
| macOS   | `~/Library/Caches/flowcatalyst/embedded-pg/bin` |
| Linux   | `~/.cache/flowcatalyst/embedded-pg/bin` (or `$XDG_CACHE_HOME/flowcatalyst/embedded-pg/bin`) |
| Windows | `%LOCALAPPDATA%\flowcatalyst\embedded-pg\bin` |

**macOS / Linux** — set a variable and confirm the version + the two target
directories (works regardless of exact layout, by anchoring on `plpgsql`):

```bash
# macOS:
EMBED=~/Library/Caches/flowcatalyst/embedded-pg/bin
# Linux:
# EMBED=~/.cache/flowcatalyst/embedded-pg/bin

"$EMBED/bin/postgres" --version          # must say: postgres (PostgreSQL) 18.x
file "$EMBED/bin/postgres"               # confirm the arch

# Module dir (where loadable modules live) and extension dir:
MODDIR=$(dirname "$(find "$EMBED/lib" -name 'plpgsql.*' | head -1)")   # e.g. .../lib/postgresql
EXTDIR=$(dirname "$(find "$EMBED/share" -name 'plpgsql.control' | head -1)")
echo "modules -> $MODDIR"
echo "extensions -> $EXTDIR"
```

On macOS modules are `.dylib` (e.g. `plpgsql.dylib`); on Linux they're `.so`.

**Windows (PowerShell):**

```powershell
$Embed = "$env:LOCALAPPDATA\flowcatalyst\embedded-pg\bin"
& "$Embed\bin\postgres.exe" --version    # postgres (PostgreSQL) 18.x
$ModDir = "$Embed\lib"                    # modules: lib\*.dll
$ExtDir = (Get-ChildItem "$Embed\share" -Recurse -Filter plpgsql.control).Directory.FullName
"modules -> $ModDir"; "extensions -> $ExtDir"
```

---

## macOS (Apple Silicon)

```bash
# 1. Install PostGIS for PG18 + keep its deps (Homebrew installs GEOS/PROJ/GDAL).
brew install postgresql@18 postgis
#    Ensure the postgis you got is the PG18 build:
brew info postgis | grep -i postgresql

# 2. Find the source files in the Homebrew keg.
SRC_MOD=$(dirname "$(find "$(brew --prefix)" -name 'postgis-3.*' -path '*postgresql*' | head -1)")
SRC_EXT=$(dirname "$(find "$(brew --prefix)" -name 'postgis.control' | head -1)")

# 3. Copy modules + extension files into the embedded tree (EMBED/MODDIR/EXTDIR from Step 0).
cp "$SRC_MOD"/postgis*-3.* "$SRC_MOD"/rtpostgis*-3.* "$SRC_MOD"/address_standardizer*-3.* "$MODDIR"/ 2>/dev/null
cp "$SRC_EXT"/postgis* "$SRC_EXT"/address_standardizer* "$EXTDIR"/ 2>/dev/null
```

**macOS dylib note:** Homebrew's `postgis-3.dylib` references its deps by their
absolute keg paths (`/opt/homebrew/opt/geos/lib/…`), so as long as the Homebrew
`geos`/`proj`/`gdal` formulae stay installed, it loads. If you later
`brew uninstall` them you'll get *"Library not loaded"* on `CREATE EXTENSION`.

---

## Linux — Ubuntu / Debian (x64 and arm64)

Use the official PGDG apt repo (it has PG18 PostGIS for both amd64 and arm64),
then copy. System packages install GEOS/PROJ/GDAL system-wide, so the loader
resolves them via `ldconfig` — **no path fixing needed**.

```bash
# 1. PGDG repo (once):
sudo apt-get install -y curl ca-certificates
sudo install -d /usr/share/postgresql-common/pgdg
sudo curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
  https://www.postgresql.org/media/keys/ACCC4CF8.asc
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] \
  https://apt.postgresql.org/pub/repos/apt $(. /etc/os-release; echo $VERSION_CODENAME)-pgdg main" \
  | sudo tee /etc/apt/sources.list.d/pgdg.list
sudo apt-get update

# 2. PostGIS for PG18:
sudo apt-get install -y postgresql-18-postgis-3

# 3. Copy into the embedded tree (MODDIR/EXTDIR from Step 0):
sudo cp /usr/lib/postgresql/18/lib/postgis*-3.so \
        /usr/lib/postgresql/18/lib/rtpostgis*-3.so \
        /usr/lib/postgresql/18/lib/address_standardizer*-3.so "$MODDIR"/ 2>/dev/null
cp /usr/share/postgresql/18/extension/postgis* \
   /usr/share/postgresql/18/extension/address_standardizer* "$EXTDIR"/ 2>/dev/null
```

`arm64`: identical commands — apt pulls the arm64 build automatically.

> If you run `fc-dev` on **Alpine/musl**, the embedded build is
> `linux-amd64-alpine`; install PostGIS from Alpine's repo
> (`apk add postgis`) built against PG18 and copy the same way. Do **not** mix
> glibc PostGIS into a musl build.

---

## Linux — RHEL / Rocky / AlmaLinux (x64 and arm64)

Use the PGDG yum repo.

```bash
# 1. PGDG repo for your release (example: EL9). Pick the matching RPM from
#    https://download.postgresql.org/pub/repos/yum/reporpms/
sudo dnf install -y https://download.postgresql.org/pub/repos/yum/reporpms/EL-9-x86_64/pgdg-redhat-repo-latest.noarch.rpm
#    (arm64: use the EL-9-aarch64 reporpm URL instead)
sudo dnf -qy module disable postgresql   # let PGDG provide PG, not the distro module

# 2. PostGIS for PG18:
sudo dnf install -y postgis34_18          # name is postgis<MAJOR>_18; adjust to the available version

# 3. Copy into the embedded tree (MODDIR/EXTDIR from Step 0):
sudo cp /usr/pgsql-18/lib/postgis*-3.so \
        /usr/pgsql-18/lib/rtpostgis*-3.so \
        /usr/pgsql-18/lib/address_standardizer*-3.so "$MODDIR"/ 2>/dev/null
cp /usr/pgsql-18/share/extension/postgis* \
   /usr/pgsql-18/share/extension/address_standardizer* "$EXTDIR"/ 2>/dev/null
```

`arm64`: same, with the `aarch64` reporpm. Dependencies (geos/proj/gdal) are
pulled in by the RPM and resolved system-wide.

---

## Windows (x64)

There's no apt/yum, so source the files from a PG18 PostGIS bundle:

1. Install **PostgreSQL 18 (x64)** from EDB, then run **Stack Builder** →
   *Spatial Extensions* → **PostGIS for PG18**. (Or grab the matching PostGIS
   zip from the OSGeo/PostGIS Windows downloads.)
2. From that install (`C:\Program Files\PostgreSQL\18\`), copy into the embedded
   tree (`%LOCALAPPDATA%\flowcatalyst\embedded-pg\bin\`):

```powershell
$Src = "C:\Program Files\PostgreSQL\18"
# Loadable modules + the dependency DLLs PostGIS needs (geos_c, proj, libgdal, …):
Copy-Item "$Src\lib\postgis*-3.dll","$Src\lib\rtpostgis*-3.dll","$Src\lib\address_standardizer*-3.dll" $ModDir
Copy-Item "$Src\bin\*geos*.dll","$Src\bin\*proj*.dll","$Src\bin\*gdal*.dll","$Src\bin\libxml2*.dll","$Src\bin\*protobuf*.dll" "$Embed\bin"  # next to postgres.exe so they're found
# Extension SQL/control:
Copy-Item "$Src\share\extension\postgis*","$Src\share\extension\address_standardizer*" $ExtDir
```

On Windows the loader finds DLLs in the directory of `postgres.exe`, which is
why the dependency DLLs go in `…\embedded-pg\bin\bin\`. If `CREATE EXTENSION`
reports a missing DLL, copy the named DLL there too.

**Windows on ARM:** run the **x64** `fc-dev` (emulated) and follow the x64 steps
above — there is no native arm64 embedded build.

---

## Step 3 — Enable and verify

The embedded tree doesn't ship `psql`. Use the `psql` from the Postgres you just
installed (or any client / GUI) against the embedded DB
(default port **15432**, db `flowcatalyst`, `postgres`/`postgres`):

```bash
# macOS/Linux example (use the psql from the PG18 you installed):
psql "postgresql://postgres:postgres@localhost:15432/flowcatalyst" \
  -c "CREATE EXTENSION IF NOT EXISTS postgis;" \
  -c "SELECT postgis_full_version();"
```

```powershell
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" `
  "postgresql://postgres:postgres@localhost:15432/flowcatalyst" `
  -c "CREATE EXTENSION IF NOT EXISTS postgis;" -c "SELECT postgis_full_version();"
```

Restart `fc-dev` first if it was running while you copied files.

## Lifecycle / gotchas

- **Restart required.** Copy files, then restart `fc-dev` so the server sees the
  new module.
- **`--embedded-db-reset` (or `FC_EMBEDDED_DB_RESET`)** wipes the *data*
  directory — your databases, and therefore the registered extension — but
  **not** the binary tree, so the PostGIS files survive; just re-run
  `CREATE EXTENSION postgis;` on the fresh DB.
- **Deleting the cache** (`…/flowcatalyst/embedded-pg/bin`) makes fc-dev
  re-download the *vanilla* binaries, removing your transplant. Re-do the copy.
- **PG minor bumps:** if the embedded PG18 minor changes and the binaries are
  re-extracted, redo the transplant with a matching PostGIS minor.
- **`undefined symbol` / `incompatible library version`** on `CREATE EXTENSION`
  → the PostGIS you copied is for the wrong PG major (not 18) or wrong arch.
- **`Library/Image not loaded`** → a runtime dep (GEOS/PROJ/GDAL) isn't where
  the module expects it; keep those packages installed (Linux/macOS) or copy the
  named DLL next to `postgres.exe` (Windows).

[`fergusstrange/embedded-postgres`]: https://github.com/fergusstrange/embedded-postgres
