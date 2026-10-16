from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from typing import Any

import httpx
import pytest
import respx
from conftest import BASE, Clock, SleepRecorder, token_body

from flowcatalyst.auth.tokens import ClientCredentialsTokenProvider
from flowcatalyst.errors import (
    AuthError,
    ForbiddenError,
    NetworkError,
    NotFoundError,
    RateLimitedError,
    RequestTimeoutError,
    ServerError,
)
from flowcatalyst.http import Transport
from flowcatalyst.retry import RetryPolicy, Timeouts

URL = f"{BASE}/api/thing"


@pytest.fixture
async def http() -> AsyncIterator[httpx.AsyncClient]:
    async with httpx.AsyncClient() as c:
        yield c


def transport(http: httpx.AsyncClient, sleeps: SleepRecorder, **kw: object) -> Transport:
    return Transport(http, BASE, sleep=sleeps, rng=lambda: 1.0, **kw)  # type: ignore[arg-type]


@respx.mock
async def test_success_returns_response(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    respx.get(URL).respond(200, json={"ok": True})
    r = await transport(http, sleeps).request("GET", "/api/thing")
    assert r.json() == {"ok": True}


@pytest.mark.parametrize("status", [408, 425, 429, 502, 503, 504])
@respx.mock
async def test_retries_retryable_status_then_succeeds(
    http: httpx.AsyncClient, sleeps: SleepRecorder, status: int
) -> None:
    route = respx.get(URL).mock(side_effect=[httpx.Response(status), httpx.Response(200, json={})])
    await transport(http, sleeps).request("GET", "/api/thing")
    assert route.call_count == 2
    assert sleeps.delays == [pytest.approx(0.1)]


@pytest.mark.parametrize("status", [500, 400, 404, 409, 422, 501])
@respx.mock
async def test_non_retryable_statuses_not_retried(
    http: httpx.AsyncClient, sleeps: SleepRecorder, status: int
) -> None:
    route = respx.get(URL).respond(status, json={"error": "E", "message": "m"})
    with pytest.raises(Exception) as ei:
        await transport(http, sleeps).request("GET", "/api/thing")
    assert getattr(ei.value, "status", None) == status
    assert route.call_count == 1
    assert sleeps.delays == []


@respx.mock
async def test_three_attempts_then_raises_last_error(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).respond(503, json={"error": "UNAVAILABLE", "message": "down"})
    with pytest.raises(ServerError) as ei:
        await transport(http, sleeps).request("GET", "/api/thing")
    assert ei.value.status == 503
    assert route.call_count == 3
    assert sleeps.delays == [pytest.approx(0.1), pytest.approx(0.2)]


@respx.mock
async def test_500_retry_is_opt_in_for_idempotent(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).mock(side_effect=[httpx.Response(500), httpx.Response(200, json={})])
    t = transport(http, sleeps, retry=RetryPolicy(retry_server_error=True))
    await t.request("GET", "/api/thing")
    assert route.call_count == 2


@respx.mock
async def test_custom_max_attempts(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    route = respx.get(URL).respond(503)
    with pytest.raises(ServerError):
        await transport(http, sleeps, retry=RetryPolicy(max_attempts=5)).request(
            "GET", "/api/thing"
        )
    assert route.call_count == 5
    assert len(sleeps.delays) == 4


@respx.mock
async def test_max_attempts_one_disables_retry(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).respond(503)
    with pytest.raises(ServerError):
        await transport(http, sleeps, retry=RetryPolicy(max_attempts=1)).request(
            "GET", "/api/thing"
        )
    assert route.call_count == 1


@respx.mock
async def test_full_jitter_draws_below_ceiling(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    respx.get(URL).respond(503)
    t = Transport(http, BASE, sleep=sleeps, rng=lambda: 0.25)
    with pytest.raises(ServerError):
        await t.request("GET", "/api/thing")
    assert sleeps.delays == [pytest.approx(0.025), pytest.approx(0.05)]


@respx.mock
async def test_backoff_never_exceeds_cap(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    respx.get(URL).respond(503)
    policy = RetryPolicy(max_attempts=12)
    with pytest.raises(ServerError):
        await transport(http, sleeps, retry=policy).request("GET", "/api/thing")
    assert max(sleeps.delays) == 5.0
    assert all(d <= 5.0 for d in sleeps.delays)


@respx.mock
async def test_retry_after_seconds_honoured(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    respx.get(URL).mock(
        side_effect=[
            httpx.Response(429, headers={"Retry-After": "2"}),
            httpx.Response(200, json={}),
        ]
    )
    await transport(http, sleeps).request("GET", "/api/thing")
    assert sleeps.delays == [2.0]


@respx.mock
async def test_retry_after_capped(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    respx.get(URL).mock(
        side_effect=[
            httpx.Response(503, headers={"Retry-After": "3600"}),
            httpx.Response(200, json={}),
        ]
    )
    await transport(http, sleeps).request("GET", "/api/thing")
    assert sleeps.delays == [30.0]


@respx.mock
async def test_retry_after_smaller_than_backoff_does_not_shorten(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    respx.get(URL).mock(
        side_effect=[
            httpx.Response(503),
            httpx.Response(503, headers={"Retry-After": "0"}),
            httpx.Response(200, json={}),
        ]
    )
    await transport(http, sleeps).request("GET", "/api/thing")
    assert sleeps.delays == [pytest.approx(0.1), pytest.approx(0.2)]


@respx.mock
async def test_429_exhausted_is_rate_limited_with_retry_after(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    respx.get(URL).respond(
        429, headers={"Retry-After": "4"}, json={"error": "RATE", "message": "slow"}
    )
    with pytest.raises(RateLimitedError) as ei:
        await transport(http, sleeps).request("GET", "/api/thing")
    assert ei.value.retry_after == 4.0


# ---- transport errors and idempotency -----------------------------------------------------


@respx.mock
async def test_get_retries_read_error(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    route = respx.get(URL).mock(
        side_effect=[httpx.ReadError("reset"), httpx.Response(200, json={})]
    )
    await transport(http, sleeps).request("GET", "/api/thing")
    assert route.call_count == 2


@pytest.mark.parametrize("method", ["PUT", "DELETE"])
@respx.mock
async def test_put_delete_retry_read_error(
    http: httpx.AsyncClient, sleeps: SleepRecorder, method: str
) -> None:
    route = respx.route(method=method, url=URL).mock(
        side_effect=[httpx.ReadError("reset"), httpx.Response(200, json={})]
    )
    await transport(http, sleeps).request(method, "/api/thing")
    assert route.call_count == 2


@respx.mock
async def test_post_does_not_retry_after_request_sent(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.post(URL).mock(side_effect=httpx.ReadError("reset"))
    with pytest.raises(NetworkError):
        await transport(http, sleeps).request("POST", "/api/thing", json={})
    assert route.call_count == 1


@respx.mock
async def test_post_read_timeout_not_retried(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.post(URL).mock(side_effect=httpx.ReadTimeout("slow"))
    with pytest.raises(RequestTimeoutError):
        await transport(http, sleeps).request("POST", "/api/thing", json={})
    assert route.call_count == 1


@respx.mock
async def test_post_retries_connect_error_because_nothing_was_sent(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.post(URL).mock(
        side_effect=[httpx.ConnectError("refused"), httpx.Response(200, json={})]
    )
    await transport(http, sleeps).request("POST", "/api/thing", json={})
    assert route.call_count == 2


@respx.mock
async def test_post_with_idempotent_flag_retries_read_error(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.post(URL).mock(
        side_effect=[httpx.ReadError("reset"), httpx.Response(200, json={})]
    )
    await transport(http, sleeps).request("POST", "/api/thing", json={}, idempotent=True)
    assert route.call_count == 2


@respx.mock
async def test_post_503_is_retried_regardless(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.post(URL).mock(side_effect=[httpx.Response(503), httpx.Response(200, json={})])
    await transport(http, sleeps).request("POST", "/api/thing", json={})
    assert route.call_count == 2


@respx.mock
async def test_network_error_after_attempts(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    route = respx.get(URL).mock(side_effect=httpx.ConnectError("refused"))
    with pytest.raises(NetworkError) as ei:
        await transport(http, sleeps).request("GET", "/api/thing")
    assert route.call_count == 3
    assert isinstance(ei.value.__cause__, httpx.ConnectError)


@respx.mock
async def test_no_httpx_exception_escapes(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    respx.get(URL).mock(side_effect=httpx.DecodingError("garbled"))
    with pytest.raises(NetworkError):
        await transport(http, sleeps).request("GET", "/api/thing")


@respx.mock
async def test_timeouts_passed_to_httpx_and_per_call_override(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).respond(200, json={})
    t = transport(http, sleeps)
    await t.request("GET", "/api/thing")
    assert route.calls.last.request.extensions["timeout"] == {
        "connect": 5.0,
        "read": 30.0,
        "write": 30.0,
        "pool": 5.0,
    }
    await t.request("GET", "/api/thing", timeout=Timeouts(connect=1, read=2, write=3, pool=4))
    assert route.calls.last.request.extensions["timeout"] == {
        "connect": 1.0,
        "read": 2.0,
        "write": 3.0,
        "pool": 4.0,
    }


async def test_overall_deadline_enforced(http: httpx.AsyncClient) -> None:
    with respx.mock:

        async def hang(request: httpx.Request) -> httpx.Response:
            await asyncio.sleep(10)
            return httpx.Response(200)

        respx.get(URL).mock(side_effect=hang)
        t = Transport(http, BASE)
        with pytest.raises(RequestTimeoutError):
            await t.request("GET", "/api/thing", timeout=Timeouts(total=0.05))


# ---- auth / 401 -----------------------------------------------------------------------------


def authed(http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder) -> Transport:
    raw = Transport(http, BASE, sleep=sleeps, rng=lambda: 1.0)
    prov = ClientCredentialsTokenProvider(raw, client_id="i", client_secret="s", clock=clock)
    return Transport(http, BASE, auth=prov, sleep=sleeps, rng=lambda: 1.0)


@respx.mock
async def test_bearer_attached(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body("t1"))
    route = respx.get(URL).respond(200, json={})
    await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert route.calls.last.request.headers["authorization"] == "Bearer t1"


@respx.mock
async def test_401_refreshes_once_and_retries(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    tok = respx.post(f"{BASE}/oauth/token").mock(
        side_effect=[
            httpx.Response(200, json=token_body("t1")),
            httpx.Response(200, json=token_body("t2")),
        ]
    )
    route = respx.get(URL).mock(
        side_effect=[httpx.Response(401), httpx.Response(200, json={"ok": 1})]
    )
    r = await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert r.json() == {"ok": 1}
    assert tok.call_count == 2
    calls: list[Any] = list(route.calls)
    sent = [str(c.request.headers["authorization"]) for c in calls]
    assert sent == ["Bearer t1", "Bearer t2"]
    assert sleeps.delays == []  # the 401 retry is immediate


@respx.mock
async def test_401_twice_is_auth_error_and_only_one_refresh(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    tok = respx.post(f"{BASE}/oauth/token").mock(
        side_effect=[
            httpx.Response(200, json=token_body("t1")),
            httpx.Response(200, json=token_body("t2")),
        ]
    )
    route = respx.get(URL).respond(401, json={"error": "UNAUTHENTICATED", "message": "no"})
    with pytest.raises(AuthError) as ei:
        await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert ei.value.status == 401
    assert route.call_count == 2
    assert tok.call_count == 2


@respx.mock
async def test_401_without_credentials_is_auth_error(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).respond(401)
    with pytest.raises(AuthError):
        await transport(http, sleeps).request("GET", "/api/thing")
    assert route.call_count == 1


@respx.mock
async def test_401_refresh_does_not_consume_retry_budget(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    respx.post(f"{BASE}/oauth/token").mock(
        side_effect=[
            httpx.Response(200, json=token_body("t1")),
            httpx.Response(200, json=token_body("t2")),
        ]
    )
    route = respx.get(URL).mock(
        side_effect=[
            httpx.Response(401),
            httpx.Response(503),
            httpx.Response(503),
            httpx.Response(200, json={}),
        ]
    )
    await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert route.call_count == 4


@respx.mock
async def test_403_is_forbidden_not_refreshed(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    tok = respx.post(f"{BASE}/oauth/token").respond(200, json=token_body("t1"))
    respx.get(URL).respond(403, json={"error": "FORBIDDEN", "message": "missing permission x:y"})
    with pytest.raises(ForbiddenError) as ei:
        await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert "x:y" in ei.value.message
    assert tok.call_count == 1


@respx.mock
async def test_json_body_and_query_params(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    route = respx.post(URL).respond(200, json={})
    await transport(http, sleeps).request("POST", "/api/thing", json={"a": 1}, params={"q": "true"})
    req = route.calls.last.request
    assert req.url.query == b"q=true"
    assert req.content == b'{"a":1}' or req.content == b'{"a": 1}'


@respx.mock
async def test_default_headers(http: httpx.AsyncClient, sleeps: SleepRecorder) -> None:
    route = respx.get(URL).respond(200, json={})
    await transport(http, sleeps).request("GET", "/api/thing")
    h = route.calls.last.request.headers
    assert h["user-agent"] == "flowcatalyst-python-sdk"
    assert h["accept"] == "application/json"


@respx.mock
async def test_retry_after_header_is_case_insensitive_and_used(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    respx.get(URL).mock(
        side_effect=[
            httpx.Response(503, headers={"retry-after": "3"}),
            httpx.Response(200, json={}),
        ]
    )
    await transport(http, sleeps).request("GET", "/api/thing")
    assert sleeps.delays == [3.0]


@respx.mock
async def test_error_messages_name_the_request(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    respx.get(URL).mock(side_effect=httpx.ReadTimeout("slow"))
    with pytest.raises(RequestTimeoutError, match=r"GET /api/thing timed out \(ReadTimeout\)"):
        await transport(http, sleeps, retry=RetryPolicy(max_attempts=1)).request(
            "GET", "/api/thing"
        )
    respx.get(URL).mock(side_effect=httpx.ReadError("reset by peer"))
    with pytest.raises(NetworkError, match=r"GET /api/thing failed: ReadError: reset by peer"):
        await transport(http, sleeps, retry=RetryPolicy(max_attempts=1)).request(
            "GET", "/api/thing"
        )


async def test_deadline_message(http: httpx.AsyncClient) -> None:
    with respx.mock:

        async def hang(request: httpx.Request) -> httpx.Response:
            await asyncio.sleep(10)
            return httpx.Response(200)

        respx.get(URL).mock(side_effect=hang)
        with pytest.raises(
            RequestTimeoutError, match=r"GET /api/thing exceeded the overall 0\.05s deadline"
        ):
            await Transport(http, BASE).request("GET", "/api/thing", timeout=Timeouts(total=0.05))


@respx.mock
async def test_non_retryable_error_is_not_retried_even_with_budget_left(
    http: httpx.AsyncClient, sleeps: SleepRecorder
) -> None:
    route = respx.get(URL).respond(404, json={"error": "NF", "message": "m"})
    with pytest.raises(NotFoundError):
        await transport(http, sleeps).request("GET", "/api/thing")
    assert route.call_count == 1 and sleeps.delays == []


@respx.mock
async def test_401_budget_exact(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    """After a 401 refresh, the full retry budget (3 attempts) is still available."""
    respx.post(f"{BASE}/oauth/token").mock(
        side_effect=[
            httpx.Response(200, json=token_body("t1")),
            httpx.Response(200, json=token_body("t2")),
        ]
    )
    route = respx.get(URL).mock(side_effect=[httpx.Response(401), *[httpx.Response(503)] * 3])
    with pytest.raises(ServerError):
        await authed(http, clock, sleeps).request("GET", "/api/thing")
    assert route.call_count == 4  # the 401 plus three 503 attempts
