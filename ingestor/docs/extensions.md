# Extensions: a kind of source in Java

An extension adds a kind of source beyond the built-in ones. It says what an ingestion of its kind is
configured with (which the runtime reports to the platform, and the platform's pages draw the form from),
and it turns one ingestion's settings into the Camel endpoint that ingestion's route consumes from.
Everything after the endpoint is the runtime's, as for a built-in kind: the split, the jobs, the counts, the
failures, `FAILING`, and the lease that keeps it on one runtime at a time.

On the platform an extension's source is `{kind: custom, type: <type>, settings: {...}}`; a definition file
writes `kind: <type>` with the settings beside it, a secret setting as the secret's name:

```yaml
source:
  kind: generator
  every: PT10S
  count: 3
```

Write an extension when a source needs Java: a vendor's API with state of its own (a feed token, a call per
asset), a client's SDK, a protocol. For something a few Camel steps can do, a [Camel route](camel-routes.md)
is lighter. The worked example is `ingestor-extensions/generator`, which makes up records on a schedule.

## How the runtime gets it

Three ways. The class path mixes with either of the others; the directory and the manifest are exclusive:

1. **A jar in `INGEST_EXTENSIONS_DIR`.** The runtime loads every `*.jar` in the directory at start, each with
   a class loader of its own whose parent is the runtime's, so the jar sees the contract, the extension API
   and Camel, and not other extension jars. The jar must hold what it needs beyond those (shade it) and must
   **not** bundle `ingestor-contract` or `ingestor-extension`.
2. **A pinned manifest in S3** (`INGEST_EXTENSIONS_MANIFEST`), for deployed runtimes: see below.
3. **On the class path**, in a build that depends on `ingestor-runtime` and the extension module.

Either way the extension is found with `ServiceLoader` through
`META-INF/services/io.flowcatalyst.ingestor.extension.IngestExtension`, one line: the extension's class name.

The runtime refuses to start, naming the jar, when: the jar declares no extension API, or one with another
major version than the runtime's; the jar lists no extension; a type is taken twice (the message names both
places); a kind is malformed; a default does not satisfy its own field.

### Deployed: loading from a pinned manifest in S3

A deployed runtime cannot write its extensions directory, so it loads its jars from S3, pinned per
environment by a manifest. Set `INGEST_EXTENSIONS_MANIFEST=s3://<bucket>/<key>` (for example
`s3://example-ingestor-extensions-np/manifest.json`). Unset, nothing changes.

```json
{"version": 1, "environment": "np", "publishedAt": "2026-10-29T12:00:00Z",
 "source": {"repository": "example-org/extensions-repo", "commit": "<40-hex sha>"},
 "extensions": [{"name": "ingest-extension-mix", "version": "0.1.0-SNAPSHOT",
                 "key": "jars/ingest-extension-mix-0.1.0-SNAPSHOT-<sha12>.jar", "sha256": "<64 lowercase hex>"}]}
```

`key` is an object key in the same bucket as the manifest. Names are unique and match `[a-z0-9][a-z0-9.-]*`.
Unknown fields are ignored, so the publisher can add to the manifest; a `version` other than 1 is refused.

At start the runtime fetches the manifest, downloads each jar into a fresh directory only its process can
read (streaming, computing SHA-256), and loads exactly those jars with the same checks as a directory
(extension API, service discovery, taken types). It logs each jar's name, version, first 12 hex of its hash
and the manifest's source commit. Credentials come from the default AWS provider chain (the task role) and
the region from `AWS_REGION`; `INGEST_AWS_ENDPOINT` points it at a stand-in such as LocalStack.

The runtime refuses to start, naming the manifest (or the jar), when:

- `INGEST_EXTENSIONS_DIR` also holds a `*.jar`: extensions come from the manifest or the directory, not
  both (an empty or absent directory is fine, the image's default is an empty one);
- the manifest cannot be read (access denied, the network after the S3 client's bounded retries), is not
  valid JSON, has another `version`, or has a bad entry (name form, duplicate name, missing field, a
  `sha256` that is not 64 lowercase hex);
- a listed jar cannot be fetched, or its SHA-256 differs from the manifest's (the message gives the
  expected and the actual hash).

If the manifest object **does not exist** (NoSuchKey / 404: nothing has been published to the environment
yet), the runtime logs a WARN naming it and starts with no extensions. The platform then never leases an
extension kind to this runtime, which is the safe failure.

IAM: the task role needs `s3:GetObject` on the bucket's objects. Without `s3:ListBucket` too, S3 answers a
missing key with 403 rather than 404, so a not-yet-published manifest would refuse the start instead of
starting with no extensions; grant both to get the behaviour above.

### The extension API version

`ExtensionApi.MAJOR` (in `ingestor-extension`) is the major version of the extension API: the `extension`
package, `ExtensionKind` and `SourceKind`, and what the runtime gives an extension. It changes only when an
existing extension would stop working. A jar declares the major it was built against in its manifest:

```xml
<plugin>
  <artifactId>maven-jar-plugin</artifactId>
  <configuration>
    <archive>
      <manifestEntries>
        <Ingestor-Extension-Api>1</Ingestor-Extension-Api>
      </manifestEntries>
    </archive>
  </configuration>
</plugin>
```

## The pieces

```
<name>/
  pom.xml                                          ingestor-contract and ingestor-extension as provided
  src/main/java/.../<Name>Kind.java                implements ExtensionKind: the fields and the rules
  src/main/java/.../<Name>Extension.java           implements IngestExtension: the endpoint, a test
  src/main/java/.../<Name>Endpoint.java            usually a PolledSource
  src/main/resources/META-INF/services/
    io.flowcatalyst.ingestor.extension.IngestExtension     one line: the extension's class name
```

## `ExtensionKind`: what an ingestion of it is configured with

```java
public final class GeneratorKind implements ExtensionKind {

  private static final SourceKind KIND =
      new SourceKind(
          "generator",                    // the type: lowercase words joined by '-', not a built-in kind's
          "Generator",                    // for people, in the kind list
          "Made-up records on a schedule, for trying an ingestion out. Split it as JSON with the path $.",
          List.of(
              SourceKind.field("every", "Every", Type.DURATION).description("How often a payload is made.")
                  .required().defaultValue("PT10S").between("PT1S", null).rule("must be a second or more").build(),
              SourceKind.field("count", "Records", Type.INTEGER).defaultValue("1").between("1", "1000").build(),
              SourceKind.field("template", "Template", Type.MULTILINE).defaultValue("{\"n\": ${n}}").build()));

  @Override
  public SourceKind kind() {
    return KIND;
  }
}
```

A field has a name (a letter then letters and digits, not `kind`), a label, a description, a type, whether it
is required, a default, and, by type, choices, a pattern, bounds and a rule. The runtime sends these fields
as the kind's `settings` in every poll.

| `type` | In the settings | Checked | Read with |
|---|---|---|---|
| `TEXT` | a string | `pattern`, if any; blank is not given | `settings().text(name)`, `requireText(name)` |
| `MULTILINE` | a string | | `text(name)` |
| `CHOICE` | one of `choices` | | `text(name)` |
| `INTEGER` | a number | `between(min, max)` | `integer(name)` |
| `BOOLEAN` | true or false | | `flag(name)` |
| `DURATION` | ISO 8601, `PT30S` | `between("PT30S", "PT1H")` | `duration(name)` |
| `URL` | an http or https address | | `url(name)` |
| `SECRET` | a secret's **name** | not blank | `secret(name)`, `requireSecret(name)`: the `Secret`, revealed for this ingestion |
| `LIST` | an array of strings | `pattern` on each item, `between` on the count | `list(name)` |

Rules a field cannot say go in `problems(settings, split)`: between two settings, an address that must be
https, a split the payload does not suit. The platform does not know the kind's fields, so the **runtime**
checks the settings before it builds the endpoint and reports a failed route with every problem at once. A
setting that is a secret's name is read from the platform for the ingestion that names it. **A setting of an
extension kind that holds text counts as a secret name to the platform's reveal call**, because the platform
cannot tell which settings are secrets: an extension kind's secret is a `SECRET` field.

## `IngestExtension`: the runtime's part

```java
public interface IngestExtension {
  ExtensionKind kind();
  Endpoint endpoint(ExtensionContext context) throws Exception;
  default @Nullable Tested test(ExtensionContext context, List<String> steps) throws Exception { return null; }
}
```

The implementation has a public no-argument constructor and keeps no state of its own: the runtime makes one
instance and asks it for a new endpoint per ingestion and version.

`ExtensionContext` gives:

| | |
|---|---|
| `camel()` | the `CamelContext` the route is added to |
| `ingestion()` | the ingestion: id, version, client, application, code |
| `settings()` | its settings, read by the kind's fields |
| `state()` | small values kept between polls (`get(key)`, `put(key, value)`), such as a watermark or a feed token. The platform keeps them, so they follow the ingestion to whichever runtime runs it next; `put` is refused once this runtime no longer holds the lease |
| `http()` | the runtime's HTTP clients: register the extension's own (`registerScoped`) so their requests are measured, and close the registration when the endpoint stops. The runtime's checks of the addresses a definition names (loopback, link-local, metadata) do not cover an extension's own connections: an extension that connects to an address from its settings should refuse those itself |
| `clientName(prefix)` | a name for such a client, unique among the runtime's |
| `clock()` | the time, which tests set |
| `conceal(value)` | keeps a credential the extension obtained while running (a login's session, a token grant's access token) out of the ingestion's traces, wherever it appears: a URL's path or query, a header, a body. Call it each time one is obtained, whether or not tracing is on; an API that puts a session in its paths (`/Vehicles/{session}/...`) otherwise shows it to anyone who may read the ingestion's trace metadata. Secrets read through `settings()` need no call |

Throw `InvalidSettingsException` (with a message that holds no secret) when the settings cannot be used; the
ingestion is reported `FAILED` with it. Any other exception is tried again on the runtime's next turn.

### Test connection

The platform queues a test and hands it to a runtime that carries the kind in its poll; the runtime runs
`test(context, steps)` and answers in its next report. It tries the source once, as the endpoint would, and
dispatches nothing: log in, read a little, add a sentence to `steps` for each thing done, and answer
`Tested(outcome, records)` (`RESULTS` or `NO_RESULTS`). A failure is thrown: its message is the last step, and
a refused login (`PolledSource.refusedLogin`) is shown as one. The default answers null, and the pages say the
kind offers no test. A test never reads or writes the ingestion's state.

## The endpoint

Each exchange the endpoint produces is one payload. Its body is split by the ingestion's split and each
record becomes a dispatch job, created through the platform. When the jobs cannot be created the exchange
fails, so a source that can keep a payload and offer it again (a page read again from its token, a file not
moved) should do so on a failed exchange, and commit (store the token, move the file) only after a good one.

`PolledSource` fits a source polled with code of its own. `poll(payloads)` hands any number of payloads to the
route (`payloads.dispatch(body)` answers whether their jobs exist) and says when to poll again; a poll that
throws anything, an `Error` included, is reported (`AUTH` for a refused login, else `SOURCE`) and the next poll
still comes, after the wait described under [Failures are paced](#failures-are-paced); the ingestion is
`FAILING` until one gets through. `SeenKeys`
remembers what a poll dispatched lately, by key. A `ScheduledPollEndpoint` fits too (the generator's). The
endpoint never produces (`createProducer` throws).

When the lease is about to run out without a renewal, the runtime's watchdog first **suspends** the route's
consumer (Camel's `ServiceHelper.suspendService`: `suspend()` for a `Suspendable` consumer, `stop()` for any
other), without waiting for a round or for the Camel context, and stops and removes the route afterwards. A
route the platform moves, and every route at shutdown, is suspended the same way and then waited for. A
consumer must take nothing more from its source once suspended or stopped; the exchange in flight may finish.
`PolledSource` and `ScheduledPollEndpoint` consumers do this. A consumer whose start does not return holds up
the round that starts it, but not the watchdog.

### Stopping cleanly

A `PolledSource` is suspended, not stopped outright: no further poll is scheduled, and the poll in progress is
**not interrupted** but asked to stop. From then on:

- `payloads.dispatch(...)` returns `false` at once, without running the route (`dispatchTaken`, below, still
  runs it). This is not a failure: it is not
  reported, the ingestion does not show `FAILING`, and the next poll is not delayed; a `partFailed` or an
  exception from the poll while it is stopping is logged at DEBUG only.
- `payloads.stopping()` is `true`.

So a poll that stops moving on as soon as a dispatch returns `false` (do not store the cursor, the token or the
page past the payload that was not taken) already stops cleanly: it ends at its next dispatch, and whichever
runtime runs the ingestion next reads that payload again. The runtime waits for the poll to end, within the
stop's bound, and only then counts the route as stopped and gives the ingestion's lease back (`polling()` says
whether a poll is running). A poll still running at the end of the bound is interrupted, and the lease is left
to run out.

A poll with costly work before its first dispatch can ask `stopping()` first and skip it. For example, one that
asks a vendor for many assets on a pool before it dispatches anything:

```java
@Override
protected Duration poll(Payloads payloads) throws Exception {
  List<Future<Rows>> answers = new ArrayList<>();
  try (ExecutorService pool = Executors.newFixedThreadPool(concurrency, Thread.ofVirtual().factory())) {
    for (Asset asset : assets) {
      answers.add(pool.submit(() -> payloads.stopping() ? null : vendor.rows(asset)));  // skip the calls not yet made
    }
  }
  if (payloads.stopping()) {
    return every;  // nothing dispatched, nothing moved: the next runtime asks again
  }
  for (int i = 0; i < answers.size(); i++) {
    Rows rows = answers.get(i).get();
    if (!payloads.dispatch(rows.json(), rows.identity())) {
      break;  // not taken (or stopping): do not move past it
    }
    seen.add(rows.keys());
  }
  return every;
}
```

A source that takes a payload away as it reads it (a queue that empties as it serves, with no acknowledgement)
cannot read it again, so it must not lose what it has read:

- ask `stopping()` just before each read, and do not read when it is true;
- dispatch what has been read with `payloads.dispatchTaken(body, identity)`, not `dispatch`: it runs the route
  even while the source is stopping, so a page read before the stop came still makes its jobs. A `false` from
  it is the platform's answer (the jobs were not created; the route has reported why), never the stop's. The
  runtime waits for it, within the stop's bound, before it counts the route as stopped;
- then return.

```java
@Override
protected Duration poll(Payloads payloads) throws Exception {
  for (int page = 0; page < MOST_PAGES; page++) {
    if (payloads.stopping()) {
      return every;  // read nothing more: the queue would lose it
    }
    List<Position> positions = vendor.takePage(limit);  // gone from the vendor's queue once served
    if (positions.isEmpty()) {
      break;
    }
    String body = json(positions);
    if (!payloads.dispatchTaken(body, identity(body))) {
      held = body;  // the platform did not take it: offered again at the next poll
      return every;
    }
  }
  return every;
}
```

A page the platform did not take can only be kept in memory, as before; it is lost if the route goes away
before the next poll offers it again.

`stopping()`, `dispatchTaken(...)` and `polling()` are additions to extension API 1 (`Ingestor-Extension-Api: 1`
is unchanged): an extension built before them stops cleanly without a change. One that calls
`payloads.stopping()` or `payloads.dispatchTaken(...)` needs a runtime that has them (an older runtime's
`Payloads` has no such methods, and the call fails the poll with `NoSuchMethodError`): deploy the runtime first.

### Failures are paced

A failure that repeats is waited out and logged once in full, then summarised
(see [ingestor-failure-pacing](../../docs/ingestor-failure-pacing.md)). A `PolledSource` gets this without
doing anything:

- After a poll that succeeded, the wait it returned is used, but never less than 250 ms: a loop that returns
  `Duration.ZERO` still drains a backlog, and cannot spin.
- A poll that throws, reports a part of itself with `partFailed`, or has `payloads.dispatch(...)` return
  `false` at least once is a failure, whatever it returned. (Not while the source is stopping: see
  [Stopping cleanly](#stopping-cleanly).) Dispatching `false` is not an exception and has no
  stack trace in the log; the route has reported it. After `n` failures in a row the next poll waits
  `Backoff.FAILURES.delay(n, max(retryDelay(), 1 s))` (5 s doubling to 5 min, with jitter), and not less than
  the wait the poll returned. The first poll that succeeds ends the streak.
- A refused login (`PolledSource.refusedLogin`) waits `Backoff.REFUSED_LOGIN` (5 min doubling to 1 h), and the
  `AUTH` failure the platform shows says when it will try again. Saving the ingestion starts it at once.
- The first failure of a streak is logged with its stack trace, then at most one line a minute with a count.

An extension's own calls can use the same two classes from `io.flowcatalyst.ingestor.extension`: `Backoff`
(`Backoff.FAILURES`, or `new Backoff(initial, max)`; `delay(n)` or `delay(n, floor)` for the wait after `n`
failures in a row) and `FailureLog` (`FailureLog.warn(logger)`; `failed(key, message, cause)` for a failure that
can repeat, `recovered(key, message)` when it stops). Use a key that names what failed, for example the
ingestion's id and `":poll"`. Do not write a loop that retries at once, and do not log a failure that repeats
once per attempt.

### Saying where a payload came from (idempotency keys)

When the source can say where a payload came from, say it, and its jobs carry idempotency keys: the runtime
makes each record's `externalId` from the payload's identity and the record's position in it (the SHA-256 of
`<identity>#<index>`), and the platform answers a record sent again under the same key with the job it made
first. A payload read twice (a page asked for again after its jobs failed, a retry after a timeout, the
ingestion moving to another runtime) then makes its jobs once, and its jobs share one message group however
often it is read.

The identity names the payload and is the same every time that payload is read: an asset's id and version, the
page's token, a file's path, size and time. It must never be the content alone, and two different payloads must
never share one. Without one (the default) the jobs carry no key, as before.

```java
@Override
protected Duration poll(Payloads payloads) throws Exception {
  Page page = vendor.page(cursor);
  // The page asked for is what this payload is.
  if (!payloads.dispatch(page.json(), "acme-vendor:orders?page=" + cursor)) {
    // The platform did not take the jobs; the route has reported why. Ask for the same page again
    // later: returning ZERO here would ask for it again at once, over and over.
    return Duration.ofSeconds(30);
  }
  context.state().put("cursor", page.next());
  return page.hasMore() ? Duration.ZERO : Duration.ofMinutes(5);
}
```

- From a `PolledSource`: `payloads.dispatch(body, identity)` (null or blank for none).
- From any other endpoint: `PayloadIdentity.set(exchange, identity)` on the exchange it hands the route, which
  sets the exchange property `PayloadIdentity.PROPERTY` (`ingestPayloadIdentity`).

Both are additions to extension API 1 (`Ingestor-Extension-Api: 1` is unchanged): an extension built against
the old API keeps working and gives no identity, and one that gives an identity on an older runtime has it
ignored.

## Testing

An extension's own tests run its endpoint in a `DefaultCamelContext` with an `ExtensionContext` of their own
and, for the vendor, a small server that answers as the vendor does. The runtime's own `ExtensionLoadingTest`
shows how to compile and load a jar from a directory.
