from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator

import httpx
import pytest
import respx
from conftest import BASE, Clock, SleepRecorder, token_body

from flowcatalyst.auth.tokens import ClientCredentialsTokenProvider
from flowcatalyst.errors import AuthError, NetworkError, ServerError
from flowcatalyst.http import Transport

TOKEN_URL = f"{BASE}/oauth/token"


@pytest.fixture
async def http() -> AsyncIterator[httpx.AsyncClient]:
    async with httpx.AsyncClient() as c:
        yield c


def make(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder, margin: float | None = None
) -> ClientCredentialsTokenProvider:
    transport = Transport(http, BASE, sleep=sleeps, rng=lambda: 1.0)
    if margin is None:
        return ClientCredentialsTokenProvider(
            transport, client_id="sa-id", client_secret="sa-secret", clock=clock
        )
    return ClientCredentialsTokenProvider(
        transport, client_id="sa-id", client_secret="sa-secret", clock=clock, margin=margin
    )


@respx.mock
async def test_fetches_with_form_client_credentials(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1"))
    assert await make(http, clock, sleeps).get() == "t1"
    req = route.calls.last.request
    assert req.headers["content-type"].startswith("application/x-www-form-urlencoded")
    assert (
        req.content.decode()
        == "grant_type=client_credentials&client_id=sa-id&client_secret=sa-secret"
    )
    assert "authorization" not in req.headers


@respx.mock
async def test_caches_until_margin(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1", expires_in=3600))
    p = make(http, clock, sleeps)
    await p.get()
    clock.advance(3600 - 61)  # 61 s left: still outside the 60 s margin
    await p.get()
    assert route.call_count == 1
    clock.advance(1)  # exactly 60 s left: inside the margin -> refresh
    await p.get()
    assert route.call_count == 2


@respx.mock
async def test_refresh_exactly_at_margin_boundary(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1", expires_in=200))
    p = make(http, clock, sleeps)
    await p.get()
    clock.advance(139)  # 61 s left
    await p.get()
    assert route.call_count == 1
    clock.advance(1)  # exactly 60 s left
    await p.get()
    assert route.call_count == 2


@respx.mock
async def test_short_lived_token_is_still_cached(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    """expires_in <= margin must not mean 'refresh on every call'."""
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1", expires_in=30))
    p = make(http, clock, sleeps)
    await p.get()
    await p.get()
    assert route.call_count == 1
    clock.advance(14)  # half-life margin = 15 s
    await p.get()
    assert route.call_count == 1
    clock.advance(1)
    await p.get()
    assert route.call_count == 2


@respx.mock
async def test_force_refreshes(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).mock(
        side_effect=[
            httpx.Response(200, json=token_body("t1")),
            httpx.Response(200, json=token_body("t2")),
        ]
    )
    p = make(http, clock, sleeps)
    assert await p.get() == "t1"
    assert await p.get(force=True) == "t2"
    assert await p.get() == "t2"
    assert route.call_count == 2


@respx.mock
async def test_concurrent_gets_single_flight(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    gate = asyncio.Event()

    async def slow(request: httpx.Request) -> httpx.Response:
        await gate.wait()
        return httpx.Response(200, json=token_body("t1"))

    route = respx.post(TOKEN_URL).mock(side_effect=slow)
    p = make(http, clock, sleeps)
    tasks = [asyncio.create_task(p.get()) for _ in range(10)]
    await asyncio.sleep(0.01)
    gate.set()
    assert set(await asyncio.gather(*tasks)) == {"t1"}
    assert route.call_count == 1


@respx.mock
async def test_refresh_rejected_token_single_flight(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    """Many callers saw 401 for the same token: only one new token is minted."""
    minted = iter(["t1", "t2", "t3"])

    async def slow(request: httpx.Request) -> httpx.Response:
        await asyncio.sleep(0.02)  # force the callers to interleave
        return httpx.Response(200, json=token_body(next(minted)))

    route = respx.post(TOKEN_URL).mock(side_effect=slow)
    p = make(http, clock, sleeps)
    old = await p.get()
    results = await asyncio.gather(*(p.refresh(rejected=old) for _ in range(5)))
    assert set(results) == {"t2"}
    assert route.call_count == 2


@respx.mock
async def test_refresh_when_cached_differs_from_rejected_reuses(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1"))
    p = make(http, clock, sleeps)
    await p.get()
    assert await p.refresh(rejected="some-older-token") == "t1"
    assert route.call_count == 1


@pytest.mark.parametrize("status", [400, 401])
@respx.mock
async def test_rejected_credentials_are_auth_error(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder, status: int
) -> None:
    respx.post(TOKEN_URL).respond(status, json={"error": "invalid_client", "message": "no"})
    with pytest.raises(AuthError) as ei:
        await make(http, clock, sleeps).get()
    assert ei.value.code == "invalid_client"
    assert sleeps.delays == []  # not retried


@respx.mock
async def test_failed_fetch_is_not_cached(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).mock(
        side_effect=[
            httpx.Response(401, json={"error": "x", "message": "m"}),
            httpx.Response(200, json=token_body("t1")),
        ]
    )
    p = make(http, clock, sleeps)
    with pytest.raises(AuthError):
        await p.get()
    assert await p.get() == "t1"
    assert route.call_count == 2


@respx.mock
async def test_server_error_on_token_endpoint_is_not_auth(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    """500 (e.g. 'Client not properly configured') is a ServerError, not 'bad credentials'."""
    respx.post(TOKEN_URL).respond(
        500, json={"error": "SERVER", "message": "Client not properly configured"}
    )
    with pytest.raises(ServerError) as ei:
        await make(http, clock, sleeps).get()
    assert "not properly configured" in ei.value.message


@respx.mock
async def test_transient_503_retried_on_token_fetch(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).mock(
        side_effect=[httpx.Response(503), httpx.Response(200, json=token_body("t1"))]
    )
    assert await make(http, clock, sleeps).get() == "t1"
    assert route.call_count == 2


@respx.mock
async def test_network_error_propagates(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    respx.post(TOKEN_URL).mock(side_effect=httpx.ConnectError("refused"))
    with pytest.raises(NetworkError):
        await make(http, clock, sleeps).get()


@pytest.mark.parametrize(
    "body",
    [{}, {"access_token": ""}, {"access_token": 5}, {"access_token": "t", "expires_in": "x"}],
)
@respx.mock
async def test_malformed_token_response(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder, body: dict[str, object]
) -> None:
    respx.post(TOKEN_URL).respond(200, json=body)
    with pytest.raises(AuthError):
        await make(http, clock, sleeps).get()


@respx.mock
async def test_missing_expires_in_uses_default(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json={"access_token": "t1"})
    p = make(http, clock, sleeps)
    await p.get()
    await p.get()
    assert route.call_count == 1


def test_secret_not_in_repr(http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder) -> None:
    assert "sa-secret" not in repr(make(http, clock, sleeps))


@respx.mock
async def test_token_fetch_is_retried_after_transport_error(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    """Minting a token is safe to repeat, so a read error mid-POST is retried."""
    route = respx.post(TOKEN_URL).mock(
        side_effect=[httpx.ReadError("reset"), httpx.Response(200, json=token_body("t1"))]
    )
    assert await make(http, clock, sleeps).get() == "t1"
    assert route.call_count == 2


@respx.mock
async def test_rejection_keeps_status_code_and_request_id(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    respx.post(TOKEN_URL).respond(
        400,
        json={"error": "invalid_grant", "message": "bad pair"},
        headers={"X-Request-Id": "req-7"},
    )
    with pytest.raises(AuthError) as ei:
        await make(http, clock, sleeps).get()
    assert (ei.value.status, ei.value.code, ei.value.request_id) == (400, "invalid_grant", "req-7")
    assert "bad pair" in ei.value.message


@pytest.mark.parametrize("expires_in", [0, -5])
@respx.mock
async def test_non_positive_expires_in_falls_back_to_default(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder, expires_in: int
) -> None:
    route = respx.post(TOKEN_URL).respond(200, json=token_body("t1", expires_in=expires_in))
    p = make(http, clock, sleeps)
    await p.get()
    clock.advance(100)  # default lifetime is 300 s
    await p.get()
    assert route.call_count == 1
    clock.advance(200)
    await p.get()
    assert route.call_count == 2


@pytest.mark.parametrize(
    ("body", "text"),
    [
        ({}, "no access_token"),
        ({"access_token": "t", "expires_in": "x"}, "non-numeric expires_in"),
        ({"access_token": "t", "expires_in": True}, "non-numeric expires_in"),
    ],
)
@respx.mock
async def test_malformed_response_messages(
    http: httpx.AsyncClient,
    clock: Clock,
    sleeps: SleepRecorder,
    body: dict[str, object],
    text: str,
) -> None:
    respx.post(TOKEN_URL).respond(200, json=body)
    with pytest.raises(AuthError, match=text):
        await make(http, clock, sleeps).get()


@respx.mock
async def test_non_object_token_response(
    http: httpx.AsyncClient, clock: Clock, sleeps: SleepRecorder
) -> None:
    respx.post(TOKEN_URL).respond(200, json=["x"])
    with pytest.raises(AuthError, match="not a JSON object"):
        await make(http, clock, sleeps).get()
