"""``fc.sync``: SDK self-registration. M1 ships roles only (the other categories are M3)."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from urllib.parse import quote

from flowcatalyst.http import Transport
from flowcatalyst.models.sync import SyncResult, SyncRole

__all__ = ["ROLES_SYNC_OPERATION", "SyncResource"]


@dataclass(frozen=True, slots=True)
class Operation:
    method: str
    path: str
    operation_id: str
    query_param_remove_unlisted: str = "removeUnlisted"


#: Pinned by the contract test against api/openapi.lock.json.
ROLES_SYNC_OPERATION = Operation("POST", "/api/applications/{appCode}/roles/sync", "syncRoles")


class SyncResource:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def roles(
        self, app_code: str, roles: Sequence[SyncRole], *, remove_unlisted: bool = False
    ) -> SyncResult:
        """Register ``roles`` for ``app_code``. The whole set goes in one call.

        ``remove_unlisted`` deletes SDK-sourced roles missing from ``roles``; off by default
        because other roles may legitimately hang off the application. Safe to repeat, so
        the call is retried after transport errors.
        """
        path = ROLES_SYNC_OPERATION.path.replace("{appCode}", quote(app_code, safe=""))
        response = await self._transport.request(
            "POST",
            path,
            params={ROLES_SYNC_OPERATION.query_param_remove_unlisted: "true"}
            if remove_unlisted
            else None,
            json={"roles": [r.model_dump(by_alias=True, exclude_none=True) for r in roles]},
            idempotent=True,
        )
        return SyncResult.model_validate(response.json())
