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

Two ways, which can be mixed:

1. **A jar in `INGEST_EXTENSIONS_DIR`.** The runtime loads every `*.jar` in the directory at start, each with
   a class loader of its own whose parent is the runtime's, so the jar sees the contract, the extension API
   and Camel, and not other extension jars. The jar must hold what it needs beyond those (shade it) and must
   **not** bundle `ingestor-contract` or `ingestor-extension`.
2. **On the class path**, in a build that depends on `ingestor-runtime` and the extension module.

Either way the extension is found with `ServiceLoader` through
`META-INF/services/io.flowcatalyst.ingestor.extension.IngestExtension`, one line: the extension's class name.

The runtime refuses to start, naming the jar, when: the jar declares no extension API, or one with another
major version than the runtime's; the jar lists no extension; a type is taken twice (the message names both
places); a kind is malformed; a default does not satisfy its own field.

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
| `http()` | the runtime's HTTP clients: register the extension's own (`registerScoped`) so their requests are measured, and close the registration when the endpoint stops |
| `clientName(prefix)` | a name for such a client, unique among the runtime's |
| `clock()` | the time, which tests set |

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
throws is reported (`AUTH` for a refused login, tried again every five minutes, else `SOURCE`) and the
ingestion is `FAILING` until one gets through. `SeenKeys` remembers what a poll dispatched lately, by key. A
`ScheduledPollEndpoint` fits too (the generator's). The endpoint never produces (`createProducer` throws).

## Testing

An extension's own tests run its endpoint in a `DefaultCamelContext` with an `ExtensionContext` of their own
and, for the vendor, a small server that answers as the vendor does. The runtime's own `ExtensionLoadingTest`
shows how to compile and load a jar from a directory.
