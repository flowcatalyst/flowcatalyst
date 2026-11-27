# PostGIS in the fcdev embedded Postgres

`fcdev` runs a **bundled, self-contained PostgreSQL 18** through
[`fergusstrange/embedded-postgres`], which runs **Zonky**'s
`embedded-postgres-binaries`. Those are *vanilla* PostgreSQL builds: they ship
the standard contrib extensions (`hstore`, `pg_trgm`, `bloom`, …) but **not
PostGIS**. fcdev adds PostGIS with one command:

```bash
fcdev install postgis
```

then, in the database that needs it:

```sql
CREATE EXTENSION postgis;
```

While PostGIS is not installed, `fcdev start` starts as always and logs one
warning that says so, with a link to this page. On a platform with no add-on
build (Windows, Alpine/musl Linux), use the [manual method](#by-hand-platforms-with-no-add-on).

## `fcdev install postgis`

It downloads the **PostGIS add-on** that this fcdev pins, checks its SHA-256,
and unpacks it into the embedded server's tree. The add-on is built from
source for Zonky's binaries by this repository's own release workflow
(`.github/workflows/release-postgis-addon.yml`, from `scripts/postgis-addon`)
and published as assets of a release `postgis-addon/v<postgis>-pg<major>-<build>`
of this repository.

| Platform                              | Add-on                                                    |
|---------------------------------------|-----------------------------------------------------------|
| macOS Apple silicon (arm64)           | `darwin-arm64`, built for macOS 13 or later (as Zonky's server) |
| macOS Intel (amd64)                   | `darwin-amd64`, built for macOS 13 or later               |
| Linux x86_64, glibc                   | `linux-amd64`, glibc 2.27 or later (as Zonky's server)    |
| Linux arm64, glibc                    | `linux-arm64`, glibc 2.27 or later                        |
| Linux musl (Alpine), Windows, others  | none: [by hand](#by-hand-platforms-with-no-add-on)        |

What is in it: **PostGIS 3.6.4, the core `postgis` extension only** (no raster,
topology, SFCGAL, tiger geocoder or address standardizer), for **PostgreSQL 18**,
with everything it needs: GEOS, PROJ (and PROJ's `proj.db`), SQLite, json-c,
libxml2 and protobuf-c. Nothing else on the machine is used: no Homebrew, apt
packages or PostgreSQL of your own. On Linux they are linked into the module
itself; on macOS they are libraries in the add-on's own directory. Licence
texts are in `postgis/share/licenses` in the tree, and each release carries the
exact sources of everything in it.

What it does, in order:

1. Takes a lock, so two installs never run at once.
2. If the embedded server's tree does not exist yet (fcdev has never started on
   this machine), fetches it first, exactly as `fcdev start` would.
3. Refuses if the tree's server is not the PostgreSQL major the add-on is for.
4. If the pinned add-on is already installed, does nothing and says so.
5. Downloads the asset over HTTPS from the GitHub release (with a time and a
   size limit) and checks it against the SHA-256 compiled into fcdev. A
   mismatch installs nothing.
6. Unpacks it into a staging directory, accepting regular files and directories
   only, under the paths an add-on may use (`lib/postgresql`,
   `share/postgresql/extension`, its own `postgis/` and `lib/share/proj/`;
   never `bin/` or the rest of Zonky's files).
7. Moves the files into the tree and writes `postgis-addon.json` there last: its
   presence marks a complete install. If anything fails part way, the tree is
   marked incomplete, `fcdev start` says so, and running the command again
   finishes the job.

The tree is the one fcdev's embedded server runs from:

| OS      | Tree |
|---------|------|
| macOS   | `~/Library/Caches/flowcatalyst/embedded-pg/bin` |
| Linux   | `~/.cache/flowcatalyst/embedded-pg/bin` (or `$XDG_CACHE_HOME/flowcatalyst/embedded-pg/bin`) |

**While fcdev is running.** Installing is fine: PostgreSQL reads an extension's
files when `CREATE EXTENSION` runs, so a first install works at once. When the
command replaces a PostGIS that the running server may already have loaded (an
earlier add-on, or one copied in by hand), it tells you to restart fcdev
(`fcdev stop`, then `fcdev start`); after a version change, run
`ALTER EXTENSION postgis UPDATE;` in each database that has it. Every fcdev on
the machine (and anything else run from that tree) uses the same tree.

**Repeats.** Running it again when that add-on is installed does nothing.

**A local build.** `fcdev install postgis --archive <tarball>` installs an
add-on you built yourself (`scripts/postgis-addon/README.md`). It must name
your platform and fcdev's PostgreSQL major; there is no pinned checksum to
check it against.

**No release pinned yet.** An fcdev built before the first add-on release has
no tag or checksums in `cmd/fcdev/postgis_addon_pins.go`. The command then says
that this fcdev has no add-on release pinned, and the manual method below
applies.

### What survives what

- **Data wipes** (`fcdev start --embedded-db-reset`, `fcdev db upgrade`,
  `fcdev fresh`) touch the *data* only, never the tree. PostGIS stays
  installed; a fresh database needs `CREATE EXTENSION postgis;` again.
- **Deleting the cache** (the tree above) makes the next `fcdev start` unpack a
  vanilla server, so the warning comes back: run `fcdev install postgis` again.
- **A new fcdev** that pins a newer add-on: `fcdev start` says so; run
  `fcdev install postgis` to update.
- **A PostgreSQL major change** in fcdev: the command refuses an add-on built
  for another major.

### Gotchas

- **`PROJ_DATA` / `PROJ_LIB`.** fcdev's server inherits fcdev's environment.
  If either is set, PROJ looks there for `proj.db` before the add-on's copy,
  and a `proj.db` of another PROJ version can fail transformations. The same
  goes for a `proj.db` in PROJ's user directory (`~/.local/share/proj`, or
  `~/Library/Application Support/proj` on macOS). Unset the variable for fcdev,
  or remove that file.
- **The Intel fcdev on Apple silicon** (under Rosetta) installs the
  `darwin-amd64` add-on; that combination is not tested. Use the arm64 fcdev.

## By hand (platforms with no add-on)

PostGIS is "just files": a loadable module plus its extension SQL and control
files. So you can **transplant** a matching PostGIS build into the embedded
tree, then `CREATE EXTENSION postgis;`. This is the method for Windows,
Alpine/musl Linux and anything else without an add-on, and for an fcdev with no
add-on release pinned. `fcdev start` sees a hand-copied PostGIS (by its
`postgis.control`) and does not warn; `fcdev install postgis` replaces it.

> If your app's database needs PostGIS in anger, you can also run your own
> Postgres (e.g. the `postgis/postgis` Docker image) and point the outbox
> poller at it (`fcdev outbox --source-db-url=…`).

### The version that must match

The embedded server is **PostgreSQL 18** (pinned in `cmd/fcdev/embedded.go` as
`embeddedPGVersion = embeddedpostgres.V18`). **Everything you install must be
PostGIS built for PostgreSQL 18** on your OS, CPU and C library. Mixing majors
fails to load with `undefined symbol` / `incompatible library`.

1. **Match the major: PostgreSQL 18.** Check the embedded binary, not your
   system Postgres (Step 0).
2. **Match OS + arch + libc** (the exact build the loader will `dlopen`).
3. **Keep PostGIS's runtime dependencies installed** (GEOS, PROJ, GDAL,
   libxml2, protobuf-c, …): a hand-copied module links against them at load time.
4. **Do it after the first `fcdev start`** (so the tree exists), then restart
   fcdev and run `CREATE EXTENSION postgis;`.

| Your platform              | Embedded PG build it runs     | PostGIS to copy in |
|----------------------------|-------------------------------|--------------------|
| Linux x86_64 (musl/Alpine) | `linux-amd64-alpine`          | Alpine **amd64**   |
| Windows x64                | `windows-amd64`               | Windows **x64**    |
| Windows on ARM             | no Zonky build: run the x64 fcdev (emulated) | Windows **x64** |
| macOS, Linux with glibc    | use `fcdev install postgis`   | the same platform's PostGIS for PG18 |

> **Windows ARM:** Zonky publishes no `windows-arm64` binary, so a *native*
> arm64 `fcdev` cannot start the embedded DB at all. Run the **x64** `fcdev`
> under emulation; it pulls `windows-amd64` Postgres, and you install **x64**
> PostGIS.

### Step 0: locate and verify the embedded tree

| OS      | Tree |
|---------|------|
| macOS   | `~/Library/Caches/flowcatalyst/embedded-pg/bin` |
| Linux   | `~/.cache/flowcatalyst/embedded-pg/bin` (or `$XDG_CACHE_HOME/flowcatalyst/embedded-pg/bin`) |
| Windows | `%LOCALAPPDATA%\flowcatalyst\embedded-pg\bin` |

**Linux / macOS:**

```bash
EMBED=~/.cache/flowcatalyst/embedded-pg/bin             # macOS: ~/Library/Caches/flowcatalyst/embedded-pg/bin
"$EMBED/bin/postgres" --version                          # must say: postgres (PostgreSQL) 18.x
MODDIR=$(dirname "$(find "$EMBED/lib" -name 'plpgsql.*' | head -1)")          # loadable modules
EXTDIR=$(dirname "$(find "$EMBED/share" -name 'plpgsql.control' | head -1)")  # extension files
echo "modules -> $MODDIR"; echo "extensions -> $EXTDIR"
```

**Windows (PowerShell):**

```powershell
$Embed = "$env:LOCALAPPDATA\flowcatalyst\embedded-pg\bin"
& "$Embed\bin\postgres.exe" --version    # postgres (PostgreSQL) 18.x
$ModDir = "$Embed\lib"                    # modules: lib\*.dll
$ExtDir = (Get-ChildItem "$Embed\share" -Recurse -Filter plpgsql.control).Directory.FullName
"modules -> $ModDir"; "extensions -> $ExtDir"
```

### Windows (x64)

There's no apt/yum, so take the files from a PG18 PostGIS bundle:

1. Install **PostgreSQL 18 (x64)** from EDB, then run **Stack Builder** →
   *Spatial Extensions* → **PostGIS for PG18**. (Or take the matching PostGIS
   zip from the OSGeo/PostGIS Windows downloads.)
2. From that install (`C:\Program Files\PostgreSQL\18\`), copy into the
   embedded tree (`%LOCALAPPDATA%\flowcatalyst\embedded-pg\bin\`):

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

**Windows on ARM:** run the **x64** `fcdev` (emulated) and follow the x64 steps
above; there is no native arm64 embedded build.

### Alpine (musl)

On Alpine the embedded build is `linux-amd64-alpine`. Install PostGIS from
Alpine's repository built against PostgreSQL 18 and copy its module
(`postgis-3.so`) into `$MODDIR` and the `postgis*` files of its `extension`
directory into `$EXTDIR` (Step 0). Do **not** copy a glibc PostGIS into a musl
build.

### macOS and glibc Linux, with no add-on pinned

Use the platform's packages of PostGIS for **PostgreSQL 18** and copy the same
files (use `find … -exec cp {} … \;`, not bare globs: zsh aborts a command
whose glob matches nothing):

- **macOS (Homebrew):** `brew install postgis`, then copy
  `$(brew --prefix)/lib/postgresql@18/postgis-3.dylib` into `$MODDIR` and
  `$(brew --prefix)/share/postgresql@18/extension/postgis*` into `$EXTDIR`. The
  module refers to Homebrew's GEOS/PROJ by their keg paths, so it stops loading
  if you `brew uninstall` them.
- **Debian/Ubuntu (PGDG apt repository):** `apt-get install postgresql-18-postgis-3`,
  then copy `/usr/lib/postgresql/18/lib/postgis-3.so` and
  `/usr/share/postgresql/18/extension/postgis*`.
- **RHEL/Rocky/AlmaLinux (PGDG yum repository):** `dnf install postgis<version>_18`
  (whichever PostGIS 3 package PGDG has for 18), then copy
  `/usr/pgsql-18/lib/postgis-3.so` and `/usr/pgsql-18/share/extension/postgis*`.

### Enable and verify

The embedded tree ships no `psql`. Use any client against the embedded database
(default port **15432**, database `flowcatalyst`, `postgres`/`postgres`; the
server listens on loopback only. The default password stays the well-known one
because developers' own applications connect with that URL; to choose another,
set `--embedded-db-password` / `FC_EMBEDDED_DB_PASSWORD` before the cluster is
first created, as it cannot be changed on an existing data directory):

```bash
psql "postgresql://postgres:postgres@localhost:15432/flowcatalyst" \
  -c "CREATE EXTENSION IF NOT EXISTS postgis;" \
  -c "SELECT postgis_full_version();"
```

```powershell
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" `
  "postgresql://postgres:postgres@localhost:15432/flowcatalyst" `
  -c "CREATE EXTENSION IF NOT EXISTS postgis;" -c "SELECT postgis_full_version();"
```

Restart `fcdev` first if it was running while you copied files.

### Manual method: lifecycle and errors

- **Restart required.** Copy files, then restart `fcdev` so the server sees
  the new module.
- **Data wipes** (`--embedded-db-reset`, `fcdev db upgrade`) remove the data
  directory, not the tree: the files survive; run `CREATE EXTENSION postgis;`
  on the fresh database.
- **Deleting the cache** makes fcdev unpack the *vanilla* binaries again,
  removing your transplant. Copy again.
- **`undefined symbol` / `incompatible library version`** on `CREATE EXTENSION`
  → the PostGIS you copied is for the wrong PG major (not 18) or architecture.
- **`Library/Image not loaded`** → a runtime dependency (GEOS/PROJ/GDAL) is not
  where the module expects it; keep those packages installed (Linux/macOS) or
  copy the named DLL next to `postgres.exe` (Windows).

[`fergusstrange/embedded-postgres`]: https://github.com/fergusstrange/embedded-postgres
