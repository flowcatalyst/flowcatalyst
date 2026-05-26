# Parity tests

Contract tests that the Go server responds byte-compatibly with the Rust server. See [`docs/api-parity.md`](../../docs/api-parity.md) for the full strategy.

## Layout

```
tests/parity/
├── README.md
├── spec-exempt.txt        # OpenAPI paths exempt from the spec diff (shrinks to 0 by end of Phase 3)
└── requests/              # YAML request files for the replay harness (per subdomain)
    ├── event-types/
    ├── subscriptions/
    └── ...
```

## Running

```sh
# Spec diff (requires oasdiff)
oasdiff breaking <rust-spec.json> <go-spec.json> --fail-on ERR

# Replay harness
go run ./tools/parityharness -rust=http://localhost:3001 -go=http://localhost:3002 -dir=tests/parity/requests
```

Both are wired into CI in `.github/workflows/ci.yml`. The `parity-spec` job is `continue-on-error: true` until Phase 3 completes; remove that flag once the spec exempt list is empty.
