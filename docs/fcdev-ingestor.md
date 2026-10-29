# The ingestor runtime under fcdev

`fcdev start` runs the ingestor runtime (the Java program built from `./ingestor`) beside the dev platform,
with no setup. This page says what happens, where things live, and how to change it. For what the runtime
does, see `ingestor/README.md`; for the variables, `docs/environment-variables.md` (section 12).

## What happens on start

It happens only when ingestion is enabled (`FC_INGESTION_ENABLED`, on in fcdev) and the ingestor is
(`--ingestor` / `FC_INGESTOR_ENABLED`, on by default).

1. **Credential.** fcdev creates the OAuth client `fcdev-ingestor` if it is missing, holding exactly the role
   `platform:ingestion-runtime`. On every start its secret is re-minted (kept only in the child's environment,
   never logged or written to disk) and its role is reconciled to that one role, like `fcdev-router` and
   `fcdev-functions`.
2. **Jar.** In order: `FC_INGESTOR_JAR`; a built jar in the checkout fcdev is run from
   (`ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar`, the newest); the pinned release, downloaded
   and SHA-256 checked. No runtime release exists yet, so when the first two find nothing fcdev logs one INFO
   line and starts without the ingestor.
3. **Java.** fcdev never uses the machine's Java. It installs its own Eclipse Temurin JRE, of the major the
   runtime needs, on first use (a one-time download of about 55 MB from `api.adoptium.net`, checksum verified
   before unpacking, installed atomically).
4. **Working directory.** fcdev writes `config/application.yaml` (the jar will not start without it) in the
   per-instance state directory and starts `java -jar` there with the credential and ports in the environment.
5. **Ready.** When `/health/ready` answers, fcdev logs `ingestor runtime ready on :8091`. The runtime then
   polls the platform every 15 seconds (dev setting) and shows as a live runtime.

The runtime's output goes to `ingestor.log` in the state directory. If the JVM exits unexpectedly fcdev
restarts it, waiting 2 s, 4 s, 8 s ... (up to a minute) between attempts, and gives up after 5 consecutive
failures (a run of 5 minutes or more resets the count); each restart is a WARN line.

A missing ingestor never stops fcdev. Offline, no jar, port taken, or a Java that cannot be installed each log
one INFO/WARN line and fcdev carries on.

## Where things are

| What | Where |
|---|---|
| fcdev's Java | `<user-cache-dir>/flowcatalyst/fcdev-jre/jre-<major>/` (macOS: `~/Library/Caches/...`; Linux: `~/.cache/...`; Windows: `%LocalAppData%`). `fcdev-jre.json` there records the major and the full version. Override the root with `FC_INGESTOR_JRE_DIR`. |
| Downloaded release jars | `<user-cache-dir>/flowcatalyst/fcdev-ingestor/<version>/` (`<FC_INGESTOR_JRE_DIR>/ingestor-jars/` when that is set) |
| Working directory, log, pid | `<user-data-dir>/flowcatalyst/fcdev-ingestor/api-<api-port>/` (override `FC_INGESTOR_STATE_DIR`) |

`fcdev ingestor status [--api-port N] [--ingestor-port N]` prints the Java, the jar, the process and the health.

## Raising the Java version

The required major is the constant `ingestorJavaMajor` in `cmd/fcdev/ingestor_jre.go` (kept equal to
`maven.compiler.release` in `ingestor/pom.xml`; a test checks it). When a new fcdev raises it, the next start
logs one line, for example

    ingestor runtime needs Java 26; installing it (the Java 25 install stays until you remove it)

installs the new major beside the old and uses it. Old installs are never deleted by fcdev; remove
`jre-25/` yourself when you no longer want it. A JRE is not updated within a major: to pick up a newer 25.x,
delete `jre-25/` and restart.

## Using a local jar

From a checkout, build and restart fcdev:

    ingestor/mvnw -f ingestor/pom.xml -DskipTests package     # needs a JDK 25 to build; running needs none

fcdev finds `ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar` from its working directory or the
directory of the fcdev binary upward. To use a jar from anywhere: `FC_INGESTOR_JAR=/path/to/jar fcdev start`.
Extension jars are loaded from `~/.flowcatalyst/ingestor-extensions` (created on start), where an
extension's build drops its jar; `FC_INGESTOR_EXTENSIONS_DIR=/path/to/dir` names another folder. The runtime
reads the folder when it starts, so restart fcdev after rebuilding an extension.

## Turning it off

`fcdev start --ingestor=false`, or `FC_INGESTOR_ENABLED=false`. (`FC_INGESTION_ENABLED=false` turns off the
platform's ingestion API as well.) The `fcdev-ingestor` credential is then not touched.

## Stopping and crashes

On a normal stop (`fcdev stop`, Ctrl-C, SIGTERM) fcdev sends the JVM SIGTERM and kills it after 15 s if it has
not exited. On Linux the kernel also kills the JVM if fcdev dies without cleanup. On macOS there is no such
facility: if fcdev is killed with SIGKILL the runtime keeps running and holds its port; the next start logs that
the port is in use and skips the ingestor. Stop the leftover (its pid is in `ingestor.pid`) and restart. On
Windows the JVM is ended outright (no graceful signal) on a normal stop.

## Isolated instances

An isolated fcdev needs its own `--ingestor-port`, `FC_INGESTOR_STATE_DIR` and `FC_INGESTOR_JRE_DIR` (or it
shares the Java cache, which is safe, and collides on the port and state if they are left at the defaults of
the same API port).
