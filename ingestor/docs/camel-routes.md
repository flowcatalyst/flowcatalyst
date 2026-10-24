# Camel routes

A `camel-route` ingestion is a route written in [Camel's YAML DSL](https://camel.apache.org/components/latest/others/yaml-dsl.html).
Its `from` and steps make the payload; the runtime then splits it and creates the jobs exactly as for any
other source, and reports its exchanges and failures the same way. Use it for what the built-in kinds do
not cover: an unusual HTTP exchange, a body to reshape before it is split, a component the other kinds do
not use.

Prefer a built-in kind when one fits. They keep a payload until its jobs exist (a file moved to `.done/`
only then, a message acknowledged only then), put unreadable ones aside, keep watermarks and cursors, and
read the ingestion's secrets for its client. A route does what its own steps say and nothing more.

## Switching it on

The runtime reports the `camel-route` kind, and runs its ingestions, only when `INGEST_CAMEL_ROUTES_ENABLED`
is `true` (default `false`). Without it the kind does not exist for the platform: no form offers it, and an
ingestion of the kind is shown as "no runtime carries this kind".

A route runs with the runtime's own identity, network and environment, so it can reach anything the runtime
can, other clients' secrets in its environment included. The platform therefore refuses to create or change
a camel-route ingestion unless the caller holds `platform:ingestion:ingestion:route` (role
`platform:ingestion-route-author`); removing one needs only `delete`. The runtime sends this requirement with
the kind. The ingestion's own secrets are not handed to a route.

## The shape

On the platform the source is `{kind: custom, type: camel-route, settings: {yaml: "..."}}`; a definition file
writes `kind: camel-route` and `yaml:` beside it. The YAML holds one route, and nothing else:

```yaml
from:
  uri: timer:acme-fuel-prices        # where it starts
  parameters:                        # optional: the component's options
    period: 900000
  steps:                             # optional: what happens to the exchange, in order
    - to:
        uri: https://prices.example.org/v2/latest.csv
```

- At the top: `from:`, or `route:` holding `from:` (and optionally `id` and `description`).
- In `from`: `uri`, and optionally `parameters`, `steps`, `id`, `description`.
- Not allowed: `beans`, `routeConfiguration`, `rest`, templates, or a second route.

The platform stores the YAML without reading it. The runtime checks it when it takes the ingestion (a route
it cannot use is reported `FAILED` with why) and at "test connection", which checks the YAML and reports
`NOT_SUPPORTED` for a valid route: a route is not run for a test, because it would act on its own sources.

The body the last step leaves is the payload, handed to the ingestion's split. A step that throws is a
`SOURCE` failure: the exchange fails, the route shows `FAILING` until a payload gets through, and a `from`
component that can offer the message again does. A route runs on one runtime at a time, under a lease.

## What it can use

The components and languages the runtime is built with: `timer`, `http` and `https`, `log`, `direct`, `sftp`
and `ftp`, `aws2-sqs`, `aws2-s3`, and Camel's core languages (`simple`, `constant`, `header`, ...). JSONPath,
jq, Groovy and others are not in the build; a business that needs one builds its own runtime image on this
one (a dependency of its `ingestor-runtime` assembly).

## Examples

Make a body without any source, for a heartbeat or a demo (the runtime's tests run it as written):

```yaml
from:
  uri: timer:acme-heartbeat
  parameters:
    period: 60000
    includeMetadata: true
  steps:
    - setBody:
        simple: '[{"beat": ${header.CamelTimerCounter}, "at": "${date:now:yyyy-MM-dd''T''HH:mm:ssXXX}"}]'
```

with `split: {kind: json, arrayPath: $}`.

Dispatch only some answers. A `filter`'s own `steps` run when it matches, and the steps after it run either
way; to drop an exchange, `stop` it inside a filter that matches what is not wanted:

```yaml
from:
  uri: timer:acme-status
  parameters:
    period: 300000
  steps:
    - to:
        uri: https://status.acme.example/api/incidents
    - filter:
        simple: "${body} not contains 'OPEN'"
        steps:
          - stop: {}
```

A stopped exchange is not dispatched and is not a failure; the rest go on to the split.

## Java

There is no Java route per ingestion. A source that needs Java is an [extension](extensions.md): a jar that
adds a kind of source, which any ingestion then uses with its own settings.
