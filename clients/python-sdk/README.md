# FlowCatalyst Python SDK

Async-first Python client for the FlowCatalyst platform. Design and plan:
`docs/sdk-audit-and-python-sdk.md` (Part B). **Not published**; nothing here is released or tagged.

Two distributions in a uv workspace:

| Distribution | Import | State |
|---|---|---|
| `flowcatalyst` | `flowcatalyst` | M0 built (below) |
| `flowcatalyst-fastapi` | `flowcatalyst_fastapi` | placeholder (pyproject + empty package); arrives with M4 |

## What M0 covers

The core transport and auth layer (the plan's M1 content, built first because the platform
prerequisites in the plan's table were assigned to another agent).

* `FlowCatalyst` (async, `httpx.AsyncClient`, pydantic v2) and `FlowCatalystSync` (blocking facade
  that runs the same async client on a private loop thread).
* Behaviour contract (plan A.9): retry on 408/425/429/502/503/504 and transport errors, **not** 500
  (opt-in via `RetryPolicy(retry_server_error=True)`, idempotent requests only); 3 attempts;
  full-jitter backoff (base 100 ms, cap 5 s); `Retry-After` (seconds or HTTP date, capped at 30 s);
  transport errors after the request was sent are retried only for idempotent methods or calls
  marked `idempotent=True`; timeouts connect 5 s / read 30 s / overall 60 s, overridable per call.
* One error taxonomy (`FlowCatalystError` and subclasses); no `httpx` exception escapes.
* `ClientCredentialsTokenProvider`: cached, single-flight, refreshes 60 s early; one
  refresh-and-retry on 401, then `AuthError`.
* `metadata_to_wire()`: array of `{key, value}` on the wire, accepts a mapping.
* `Settings.from_env()` with an explicit prefix and variable names.
* One operation, to exercise the stack and the contract test: `fc.sync.roles(app_code, roles,
  remove_unlisted=False)` (`POST /api/applications/{appCode}/roles/sync`).
* No use-case envelope, no admin client, no generated client.

```python
from flowcatalyst import FlowCatalyst, ClientCredentials, SyncRole

async with FlowCatalyst(
    "https://platform.example", credentials=ClientCredentials(id_, secret)
) as fc:
    result = await fc.sync.roles("agent-planner", [SyncRole(name="planner", permissions=[...])])
```

Blocking code: `with FlowCatalystSync(url, credentials=...) as fc: fc.sync.roles(...)`.

## Develop

Everything runs in a venv inside this directory (git-ignored). No network is needed to run tests.

```sh
cd clients/python-sdk
python3.14 -m venv .venv && .venv/bin/pip install uv      # python >= 3.11 works
.venv/bin/uv pip install --python .venv/bin/python -e packages/flowcatalyst \
    httpx "pydantic>=2.7" pytest pytest-asyncio pytest-cov respx mypy pyright ruff
.venv/bin/python -m pytest                 # 150+ tests, all faked with respx
.venv/bin/ruff check . && .venv/bin/ruff format --check .
.venv/bin/mypy && .venv/bin/pyright        # both strict
.venv/bin/python tools/mutate.py           # manual mutation check; every mutant must be KILLED
```

`tests/test_contract_lock.py` reads `api/openapi.lock.json` from the repository at test time (never
copied) and checks the operation, parameters and model fields the SDK depends on; additive lock
changes pass. It skips when the lock is absent.

Tests never touch a real platform: all HTTP is faked with `respx`.
