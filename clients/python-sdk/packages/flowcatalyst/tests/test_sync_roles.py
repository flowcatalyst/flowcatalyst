from __future__ import annotations

import json

import httpx
import pytest
import respx
from conftest import BASE, token_body
from pydantic import ValidationError as PydanticValidationError

from flowcatalyst import ClientCredentials, FlowCatalyst, SyncRole
from flowcatalyst.errors import ForbiddenError

ROLES_URL = f"{BASE}/api/applications/agent-planner/roles/sync"
RESULT = {
    "applicationCode": "agent-planner",
    "created": 1,
    "updated": 2,
    "deleted": 0,
    "syncedCodes": ["agent-planner:planner"],
}


def client() -> FlowCatalyst:
    return FlowCatalyst(BASE, credentials=ClientCredentials("sa", "secret"))


@respx.mock
async def test_roles_sync_wire_shape() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.post(ROLES_URL).respond(200, json=RESULT)
    async with client() as fc:
        result = await fc.sync.roles(
            "agent-planner",
            [
                SyncRole(
                    name="planner",
                    display_name="Planner",
                    description="d",
                    permissions=["a:b:c:d"],
                    client_managed=False,
                )
            ],
        )
    req = route.calls.last.request
    assert req.url.query == b""  # removeUnlisted omitted unless requested
    assert json.loads(req.content) == {
        "roles": [
            {
                "name": "planner",
                "displayName": "Planner",
                "description": "d",
                "permissions": ["a:b:c:d"],
                "clientManaged": False,
            }
        ]
    }
    assert result.created == 1 and result.updated == 2 and result.deleted == 0
    assert result.synced_codes == ["agent-planner:planner"]
    assert result.skipped_roles == [] and result.password_hash_ignored == []


@respx.mock
async def test_unset_optional_fields_are_omitted() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.post(ROLES_URL).respond(200, json=RESULT)
    async with client() as fc:
        await fc.sync.roles("agent-planner", [SyncRole(name="planner")])
    assert json.loads(route.calls.last.request.content) == {"roles": [{"name": "planner"}]}


@respx.mock
async def test_remove_unlisted_sent_only_when_true() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.post(ROLES_URL).respond(200, json=RESULT)
    async with client() as fc:
        await fc.sync.roles("agent-planner", [], remove_unlisted=True)
    assert route.calls.last.request.url.params["removeUnlisted"] == "true"


@respx.mock
async def test_role_sync_retries_transport_error_because_it_is_idempotent() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.post(ROLES_URL).mock(
        side_effect=[httpx.ReadError("reset"), httpx.Response(200, json=RESULT)]
    )
    async with FlowCatalyst(
        BASE, credentials=ClientCredentials("sa", "secret"), sleep=_nosleep
    ) as fc:
        await fc.sync.roles("agent-planner", [])
    assert route.call_count == 2


async def _nosleep(_: float) -> None:
    return None


@respx.mock
async def test_403_names_the_missing_permission() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.post(ROLES_URL).respond(
        403, json={"error": "FORBIDDEN", "message": "requires platform:admin:role:write"}
    )
    async with client() as fc:
        with pytest.raises(ForbiddenError, match="platform:admin:role:write"):
            await fc.sync.roles("agent-planner", [])


@respx.mock
async def test_app_code_is_path_encoded() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    route = respx.post(f"{BASE}/api/applications/we%2Fird/roles/sync").respond(200, json=RESULT)
    async with client() as fc:
        await fc.sync.roles("we/ird", [])
    assert route.called


def test_role_name_must_be_unprefixed() -> None:
    with pytest.raises(PydanticValidationError, match="unprefixed"):
        SyncRole(name="agent-planner:planner")


def test_role_forbids_unknown_fields() -> None:
    with pytest.raises(PydanticValidationError):
        SyncRole(name="x", nope=1)  # type: ignore[call-arg]


@respx.mock
async def test_result_tolerates_additive_fields_and_reports_partial_fields() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())
    respx.post(ROLES_URL).respond(
        200, json={**RESULT, "skippedRoles": ["x"], "passwordHashIgnored": ["a@b"], "brandNew": 1}
    )
    async with client() as fc:
        r = await fc.sync.roles("agent-planner", [])
    assert r.skipped_roles == ["x"] and r.password_hash_ignored == ["a@b"]
