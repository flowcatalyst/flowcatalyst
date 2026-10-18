# ingest-webhook-rust

A public endpoint for a third party that signs what it sends. It verifies an HMAC-SHA256 signature over the raw
body, then creates one dispatch job (`ctx.dispatch`) that delivers the payload, byte for byte, to a webhook function of
the same application. The Go and Rust versions behave identically.

```
sender  POST /fn/<address>/ingest   X-Signature: sha256=<hex>
  -> ingest-webhook -- ctx.dispatch --> DISPATCH JOB (declared pool + connection) --> <target> POST /deliver
```

| Piece | Value |
|---|---|
| Endpoint | `POST /ingest`, auth `none` (open): the signature is the authentication; `GET /healthz` is open too |
| Signature | header `X-Signature: sha256=` + hex(HMAC-SHA256(secret, raw body)), compared in constant time; the secret is the declared secret `INGEST_SIGNING_SECRET` |
| Result | `202 {"jobId"}`; `401` bad or missing signature (nothing dispatched); `400` not a JSON object or array, or `X-Message-Group` over 128 chars; `500` secret not set (never an open door); `502` dispatch refused; `429` + `Retry-After: 5` platform trouble |
| Job | payload = the request body, `messageGroup` = `X-Message-Group` or the default (jobs of one group are delivered in order), metadata `source=webhook` plus `deliveryId` from `X-Delivery-Id`, correlation id from `X-Correlation-Id` |

The signature covers the body only, so it does not stop a replay of a captured request; dispatch is not
de-duplicated. If the sender can supply a delivery id, de-duplicate on `deliveryId` in the target.

## Use it

1. Edit `src/names.rs`, replacing every `changeme` (application, target function, pool, connection, message group).
   The pool and connection must exist on the platform; the target must be a function of the same application and
   client with a webhook endpoint at `DELIVER_PATH` (for example `payload-logger` from `../dispatch-demo-rust`).
2. `cargo test`
3. Try it locally; `fn dev` signs nothing for an open endpoint, so sign the sample yourself:

   ```
   SIG=$(openssl dgst -sha256 -hmac dev-secret < sample.json | awk '{print $NF}')
   fcdev fn dev . --secret INGEST_SIGNING_SECRET=dev-secret --send sample.json --path /ingest \
     --header "X-Signature: sha256=$SIG" --header 'X-Message-Group: trip-1'
   ```

   It prints the `202` and `would create job: target=... pool=... connection=... group=trip-1 payload={...}`.
4. Deploy the target first, then this function with the same application and `--client`; set the secret with
   `fcdev fn set <address> --secret INGEST_SIGNING_SECRET=...`:

   ```
   fcdev fn deploy <app>.<name> . --create --client <client code> --dry-run
   ```

One-time: `rustup target add wasm32-unknown-unknown`. The HMAC comes from the `hmac` and `sha2` crates (pure Rust, so they
build for `wasm32-unknown-unknown`); `verify_slice` is the constant-time compare. A `wasm32-unknown-unknown` guest has no
clock, so a timestamp-window check (a common replay defence) is not possible in Rust without the sender's help.
