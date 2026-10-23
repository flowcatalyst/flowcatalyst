"""Drift guard: the operations the SDK calls must exist in api/openapi.lock.json.

The lock is read at test time (never copied). Additive changes are tolerated: we only
assert what the SDK depends on.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from flowcatalyst.models.sync import SyncResult, SyncRole
from flowcatalyst.resources.sync import ROLES_SYNC_OPERATION

LOCK = Path(__file__).resolve().parents[5] / "api" / "openapi.lock.json"


@pytest.fixture(scope="module")
def lock() -> dict[str, Any]:
    if not LOCK.exists():
        pytest.skip(f"{LOCK} not found (SDK checked out without the platform repo)")
    return json.loads(LOCK.read_text())  # type: ignore[no-any-return]


def resolve(lock: dict[str, Any], schema: dict[str, Any]) -> dict[str, Any]:
    ref = schema.get("$ref")
    if ref:
        out: dict[str, Any] = lock["components"]["schemas"][ref.rsplit("/", 1)[-1]]
        return out
    return schema


def test_lock_location_is_the_repo_lock() -> None:
    assert LOCK.name == "openapi.lock.json" and LOCK.parent.name == "api"


def test_roles_sync_operation_exists(lock: dict[str, Any]) -> None:
    op = lock["paths"][ROLES_SYNC_OPERATION.path][ROLES_SYNC_OPERATION.method.lower()]
    assert op["operationId"] == ROLES_SYNC_OPERATION.operation_id
    params = {(p["in"], p["name"]) for p in op["parameters"]}
    assert ("path", "appCode") in params
    assert ("query", ROLES_SYNC_OPERATION.query_param_remove_unlisted) in params


def test_request_model_is_subset_of_lock_request_schema(lock: dict[str, Any]) -> None:
    op = lock["paths"][ROLES_SYNC_OPERATION.path]["post"]
    body = resolve(lock, op["requestBody"]["content"]["application/json"]["schema"])
    role = resolve(lock, body["properties"]["roles"]["items"])
    sent = {f.alias or n for n, f in SyncRole.model_fields.items()}
    assert sent <= set(role["properties"]), sent - set(role["properties"])
    assert set(role.get("required", [])) <= {a for a in sent}  # we can always send required fields
    # the envelope key
    assert "roles" in body["required"]


def test_response_model_covers_required_response_fields(lock: dict[str, Any]) -> None:
    op = lock["paths"][ROLES_SYNC_OPERATION.path]["post"]
    resp = resolve(lock, op["responses"]["200"]["content"]["application/json"]["schema"])
    modeled = {f.alias or n for n, f in SyncResult.model_fields.items()}
    assert set(resp["required"]) <= modeled, set(resp["required"]) - modeled
    # fields we model must exist on the wire (catches renames); extras on the wire are fine
    assert modeled <= set(resp["properties"]) | {"$schema"}, modeled - set(resp["properties"])


def test_error_model_matches_our_parsing(lock: dict[str, Any]) -> None:
    err = lock["components"]["schemas"]["ErrorModel"]
    assert {"error", "message"} <= set(err["required"])
    assert "details" in err["properties"]


def test_platform_config_operations_exist(lock: dict[str, Any]) -> None:
    from flowcatalyst.resources import platform_config as pc

    for op in (
        pc.LIST_OPERATION,
        pc.GET_OPERATION,
        pc.SET_OPERATION,
        pc.DELETE_OPERATION,
        pc.REVEAL_OPERATION,
    ):
        assert lock["paths"][op.path][op.method.lower()]["operationId"] == op.operation_id
    reveal = lock["paths"][pc.REVEAL_OPERATION.path]["get"]
    assert ("query", "clientId") in {(p["in"], p["name"]) for p in reveal["parameters"]}
    secret = lock["components"]["schemas"]["SecretResponse"]
    from flowcatalyst.models.platform_config import SecretResponse

    sent = {f.alias or n for n, f in SecretResponse.model_fields.items()}
    assert set(secret["required"]) <= sent
