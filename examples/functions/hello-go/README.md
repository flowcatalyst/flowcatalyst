# hello-go

The smallest useful Go function, mirroring `clients/fn-ts/examples/hello` and `hello-rust`:

| Endpoint | Auth | Does |
|---|---|---|
| `GET /healthz` | open | `{"status":"ok"}` |
| `GET /hello/{name}` | platform | reads the declared config `GREETING` and the caller's principal, returns `{"message","caller"}` |
| `POST /events/greeting` | webhook | emits `hello:greeting:greeting:sent`, deduplicated on the delivered event's id; 429 on platform trouble, 422 (reject) on a refusal |
| `POST /dispatch` | open | `fn.Dispatch` of `{"hello":"world"}` to `hello.logger` `/deliver` (declared with connection `hello-conn`, pool `hello-pool`) |

## Test, run, build

```
go test ./...                                  # handlers against a fake host, on the host OS
fcdev fn dev . --config GREETING=Kia           # build, serve on the real runner at 127.0.0.1:8099, hot-reload
curl 127.0.0.1:8099/fn/local.hello-go/hello/Aroha -H 'Authorization: Bearer dev'
fcdev fn build .                               # function.wasm (standard Go, GOOS=wasip1), prints the describe document
```

Tests use `fn.InvokeForTest` (request through the real frame, route and handler path), `fn.SetHost(&fn.FakeHost{...})`
to script the platform side, and `fn.DescribeForTest`. See `docs/fcdev-fn-dev.md` for the dev loop and
`docs/function-examples-go-rust.md` for why this builds with standard Go, not TinyGo.

The module `replace`s the SDK with `../../../clients/fn-go`; a project of your own `require`s the published SDK.
