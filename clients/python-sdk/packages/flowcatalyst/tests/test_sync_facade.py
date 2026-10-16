from __future__ import annotations

import asyncio
import threading

import httpx
import pytest
import respx
from conftest import BASE, token_body

from flowcatalyst import ClientCredentials, FlowCatalystSync, SyncRole
from flowcatalyst.errors import ForbiddenError

RESULT: dict[str, object] = {
    "applicationCode": "a",
    "created": 1,
    "updated": 0,
    "deleted": 0,
    "syncedCodes": [],
}


@respx.mock
def test_blocking_roles_sync() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.post(f"{BASE}/api/applications/a/roles/sync").respond(200, json=RESULT)
    with FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s")) as fc:
        assert fc.sync.roles("a", [SyncRole(name="r")]).created == 1
        assert fc.tokens.get() == "tok-1"


@respx.mock
def test_errors_propagate_with_taxonomy() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.post(f"{BASE}/api/applications/a/roles/sync").respond(
        403, json={"error": "F", "message": "m"}
    )
    with (
        FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s")) as fc,
        pytest.raises(ForbiddenError),
    ):
        fc.sync.roles("a", [])


@respx.mock
def test_runs_on_private_thread_and_close_stops_it() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    seen: list[str] = []

    def spy(request: httpx.Request) -> httpx.Response:
        seen.append(threading.current_thread().name)
        return httpx.Response(200, json=RESULT)

    respx.post(f"{BASE}/api/applications/a/roles/sync").mock(side_effect=spy)
    fc = FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s"))
    fc.sync.roles("a", [])
    assert seen and seen[0] != threading.current_thread().name
    thread_name = seen[0]
    fc.close()
    fc.close()  # idempotent
    assert all(t.name != thread_name for t in threading.enumerate())
    with pytest.raises(RuntimeError, match="closed"):
        fc.sync.roles("a", [])


@respx.mock
def test_usable_from_many_threads() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.post(f"{BASE}/api/applications/a/roles/sync").respond(200, json=RESULT)
    out: list[int] = []
    with FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s")) as fc:
        ts = [
            threading.Thread(target=lambda: out.append(fc.sync.roles("a", []).created))
            for _ in range(8)
        ]
        for t in ts:
            t.start()
        for t in ts:
            t.join()
    assert out == [1] * 8


async def test_refuses_to_block_a_running_event_loop() -> None:
    with pytest.raises(RuntimeError, match="event loop"):
        FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s"))
    await asyncio.sleep(0)


def test_refuses_blocking_call_from_loop_after_construction() -> None:
    fc = FlowCatalystSync(BASE, credentials=ClientCredentials("i", "s"))

    async def inside() -> None:
        with pytest.raises(RuntimeError, match="event loop"):
            fc.sync.roles("a", [])

    asyncio.run(inside())
    fc.close()
