from __future__ import annotations

import httpx
import pytest
import respx
from conftest import BASE, token_body

from flowcatalyst import (
    ClientCredentials,
    FlowCatalyst,
    FlowCatalystSync,
    ForbiddenError,
    NotFoundError,
    ValidationError,
)

PLAIN = {
    "id": "c1",
    "applicationCode": "app",
    "section": "smtp",
    "property": "host",
    "scope": "GLOBAL",
    "valueType": "SECRET",
    "value": "***",
    "createdAt": "2026-01-01T00:00:00Z",
    "updatedAt": "2026-01-02T00:00:00Z",
}
SECRET = {
    "applicationCode": "app",
    "section": "smtp",
    "property": "password",
    "value": "hunter2-plaintext",
    "updatedAt": "2026-01-02T00:00:00Z",
}


def client() -> FlowCatalyst:
    return FlowCatalyst(BASE, credentials=ClientCredentials("sa", "secret"))


@respx.mock
async def test_reveal_secret_success_path_and_no_query_without_client_id() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.get(f"{BASE}/api/config/app/smtp/password/secret").respond(
        200, json=SECRET, headers={"Cache-Control": "no-store"}
    )
    async with client() as fc:
        result = await fc.platform_config.reveal_secret("app", "smtp", "password")
    req = route.calls.last.request
    assert req.method == "GET" and req.url.query == b""
    assert "cache-control" not in req.headers and "if-none-match" not in req.headers
    assert result.value.get_secret_value() == "hunter2-plaintext"
    assert result.client_id is None and result.property == "password"


@respx.mock
async def test_reveal_secret_url_encodes_segments_and_sends_client_id() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.get(url__startswith=f"{BASE}/api/config/").respond(200, json=SECRET)
    async with client() as fc:
        await fc.platform_config.reveal_secret("a/b", "s ec", "p?x", client_id="clt_1")
    req = route.calls.last.request
    assert req.url.raw_path.startswith(b"/api/config/a%2Fb/s%20ec/p%3Fx/secret")
    assert req.url.params["clientId"] == "clt_1"


@respx.mock
async def test_reveal_is_not_cached_each_call_hits_the_platform() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.get(f"{BASE}/api/config/app/smtp/password/secret").respond(200, json=SECRET)
    async with client() as fc:
        await fc.platform_config.reveal_secret("app", "smtp", "password")
        await fc.platform_config.reveal_secret("app", "smtp", "password")
    assert route.call_count == 2


@pytest.mark.parametrize(
    ("status", "body", "exc", "code"),
    [
        (
            403,
            {"error": "FORBIDDEN", "message": "requires platform:admin:config:reveal"},
            ForbiddenError,
            "FORBIDDEN",
        ),
        (404, {"error": "NOT_FOUND", "message": "no such property"}, NotFoundError, "NOT_FOUND"),
        (
            400,
            {"error": "NOT_A_SECRET", "message": "property is PLAIN"},
            ValidationError,
            "NOT_A_SECRET",
        ),
    ],
)
@respx.mock
async def test_reveal_error_statuses(
    status: int, body: dict[str, str], exc: type[Exception], code: str
) -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.get(f"{BASE}/api/config/app/smtp/password/secret").respond(status, json=body)
    async with client() as fc:
        with pytest.raises(exc) as info:
            await fc.platform_config.reveal_secret("app", "smtp", "password")
    assert getattr(info.value, "code", None) == code
    assert "hunter2" not in str(info.value)


@respx.mock
async def test_value_is_redacted_in_repr_str_and_json() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.get(f"{BASE}/api/config/app/smtp/password/secret").respond(200, json=SECRET)
    async with client() as fc:
        result = await fc.platform_config.reveal_secret("app", "smtp", "password")
    for text in (repr(result), str(result), result.model_dump_json(), str(result.model_dump())):
        assert "hunter2-plaintext" not in text


@respx.mock
async def test_get_returns_masked_value_and_sends_client_id() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.get(f"{BASE}/api/config/app/smtp/host").respond(200, json=PLAIN)
    async with client() as fc:
        prop = await fc.platform_config.get("app", "smtp", "host", client_id="c")
    assert prop.value == "***" and prop.value_type == "SECRET"
    assert route.calls.last.request.url.params["clientId"] == "c"


@respx.mock
async def test_list_set_delete_wire_shapes() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    lst = respx.get(f"{BASE}/api/platform-config/app").respond(200, json={"items": [PLAIN]})
    put = respx.put(f"{BASE}/api/config/app/smtp/host").respond(200, json=PLAIN)
    dele = respx.delete(f"{BASE}/api/config/app/smtp/host").respond(204)
    async with client() as fc:
        assert len(await fc.platform_config.list("app")) == 1
        await fc.platform_config.set(
            "app", "smtp", "host", "v", value_type="SECRET", description="d"
        )
        await fc.platform_config.delete("app", "smtp", "host")
    assert lst.called and dele.called
    import json

    assert json.loads(put.calls.last.request.content) == {
        "value": "v",
        "valueType": "SECRET",
        "description": "d",
    }


def test_sync_facade_reveal_secret() -> None:
    with respx.mock:
        respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
        respx.get(f"{BASE}/api/config/app/smtp/password/secret").mock(
            return_value=httpx.Response(200, json=SECRET)
        )
        with FlowCatalystSync(BASE, credentials=ClientCredentials("sa", "secret")) as fc:
            assert (
                fc.platform_config.reveal_secret("app", "smtp", "password").value.get_secret_value()
                == "hunter2-plaintext"
            )


@respx.mock
async def test_set_delete_and_reveal_send_on_behalf_of_only_when_given() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    put = respx.put(f"{BASE}/api/config/app/smtp/host").respond(200, json=PLAIN)
    dele = respx.delete(f"{BASE}/api/config/app/smtp/host").respond(204)
    reveal = respx.get(f"{BASE}/api/config/app/smtp/password/secret").respond(200, json=SECRET)
    async with client() as fc:
        pc = fc.platform_config
        await pc.set("app", "smtp", "host", "v", on_behalf_of="usr:alice")
        await pc.set("app", "smtp", "host", "v")
        await pc.delete("app", "smtp", "host", client_id="clt_1", on_behalf_of="bob")
        await pc.delete("app", "smtp", "host", on_behalf_of="  ")
        await pc.reveal_secret("app", "smtp", "password", on_behalf_of="carol")
    assert _req(put, 0).headers["x-on-behalf-of"] == "usr:alice"
    assert "x-on-behalf-of" not in _req(put, 1).headers
    assert _req(dele, 0).headers["x-on-behalf-of"] == "bob"
    assert _req(dele, 0).url.params["clientId"] == "clt_1"
    assert "x-on-behalf-of" not in _req(dele, 1).headers
    assert reveal.calls.last.request.headers["x-on-behalf-of"] == "carol"
    assert reveal.calls.last.request.headers["authorization"].startswith("Bearer ")


@respx.mock
async def test_list_rows_carry_the_client_identifier() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    row = PLAIN | {"scope": "CLIENT", "clientId": "clt_1", "clientIdentifier": "acme"}
    respx.get(f"{BASE}/api/platform-config/app").respond(200, json={"items": [row]})
    async with client() as fc:
        (prop,) = await fc.platform_config.list("app")
    assert prop.client_id == "clt_1" and prop.client_identifier == "acme"


def _req(route: respx.Route, index: int) -> httpx.Request:
    request: httpx.Request = route.calls[index].request  # pyright: ignore[reportUnknownMemberType, reportUnknownVariableType]
    return request  # pyright: ignore[reportUnknownVariableType]
