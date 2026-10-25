"""``fc.platform_config``: application config properties and the secret-reveal call."""

from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import quote

from flowcatalyst.http import Transport
from flowcatalyst.models.platform_config import ConfigProperty, SecretResponse

__all__ = [
    "DELETE_OPERATION",
    "GET_OPERATION",
    "LIST_OPERATION",
    "ON_BEHALF_OF_HEADER",
    "REVEAL_OPERATION",
    "SET_OPERATION",
    "PlatformConfigResource",
]


@dataclass(frozen=True, slots=True)
class Operation:
    method: str
    path: str
    operation_id: str


#: Pinned by the contract test against api/openapi.lock.json.
LIST_OPERATION = Operation("GET", "/api/platform-config/{app}", "listPlatformConfigProperties")
GET_OPERATION = Operation(
    "GET", "/api/config/{app}/{section}/{property}", "getPlatformConfigProperty"
)
SET_OPERATION = Operation(
    "PUT", "/api/config/{app}/{section}/{property}", "setPlatformConfigProperty"
)
DELETE_OPERATION = Operation(
    "DELETE", "/api/config/{app}/{section}/{property}", "deletePlatformConfigProperty"
)
REVEAL_OPERATION = Operation(
    "GET", "/api/config/{app}/{section}/{property}/secret", "revealPlatformConfigSecret"
)


def _path(template: str, app: str, section: str, property_: str) -> str:
    return (
        template.replace("{app}", quote(app, safe=""))
        .replace("{section}", quote(section, safe=""))
        .replace("{property}", quote(property_, safe=""))
    )


def _client_params(client_id: str | None) -> dict[str, str] | None:
    return {"clientId": client_id} if client_id is not None else None


#: Who a calling service account acts for (for example the application's signed-in user) on
#: set, delete and reveal; recorded in the platform's audit row beside the service account.
#: Honoured only from a service account and ignored from anyone else. At most 256 printable
#: characters (no control characters), else ``400 INVALID_ON_BEHALF_OF``.
ON_BEHALF_OF_HEADER = "X-On-Behalf-Of"


def _actor(on_behalf_of: str | None) -> dict[str, str] | None:
    who = on_behalf_of.strip() if on_behalf_of is not None else ""
    return {ON_BEHALF_OF_HEADER: who} if who else None


class PlatformConfigResource:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def list(self, app: str) -> list[ConfigProperty]:
        """All properties of ``app``. SECRET values come back as ``"***"``."""
        path = LIST_OPERATION.path.replace("{app}", quote(app, safe=""))
        response = await self._transport.request("GET", path)
        return [ConfigProperty.model_validate(i) for i in response.json()["items"]]

    async def get(
        self, app: str, section: str, property: str, *, client_id: str | None = None
    ) -> ConfigProperty:
        """One property. A SECRET property's ``value`` is always ``"***"``; use
        :meth:`reveal_secret` to read the plaintext."""
        response = await self._transport.request(
            "GET",
            _path(GET_OPERATION.path, app, section, property),
            params=_client_params(client_id),
        )
        return ConfigProperty.model_validate(response.json())

    async def set(
        self,
        app: str,
        section: str,
        property: str,
        value: str,
        *,
        value_type: str | None = None,
        description: str | None = None,
        client_id: str | None = None,
        on_behalf_of: str | None = None,
    ) -> ConfigProperty:
        """Create or replace a property (``value_type`` is ``"PLAIN"`` or ``"SECRET"``).
        The response masks a SECRET value as ``"***"``.

        A SECRET's value is required and the mask is not a value: ``"***"`` is refused
        (:class:`ValidationError`, ``code == "SECRET_VALUE_MASKED"``), as is an empty secret
        (``"SECRET_VALUE_REQUIRED"``). To leave a secret unchanged, do not set it.
        ``on_behalf_of`` is sent as :data:`ON_BEHALF_OF_HEADER`."""
        body: dict[str, str] = {"value": value}
        if value_type is not None:
            body["valueType"] = value_type
        if description is not None:
            body["description"] = description
        response = await self._transport.request(
            "PUT",
            _path(SET_OPERATION.path, app, section, property),
            params=_client_params(client_id),
            json=body,
            idempotent=True,
            headers=_actor(on_behalf_of),
        )
        return ConfigProperty.model_validate(response.json())

    async def delete(
        self,
        app: str,
        section: str,
        property: str,
        *,
        client_id: str | None = None,
        on_behalf_of: str | None = None,
    ) -> None:
        """Delete a property (audited; one that is not there is not an error).
        ``on_behalf_of`` is sent as :data:`ON_BEHALF_OF_HEADER`."""
        await self._transport.request(
            "DELETE",
            _path(DELETE_OPERATION.path, app, section, property),
            params=_client_params(client_id),
            headers=_actor(on_behalf_of),
        )

    async def reveal_secret(
        self,
        app: str,
        section: str,
        property: str,
        *,
        client_id: str | None = None,
        on_behalf_of: str | None = None,
    ) -> SecretResponse:
        """Read a SECRET property's plaintext. The only way to do so; every call is audited.

        Needs a reveal permission (else :class:`ForbiddenError`): ``platform:admin:config:reveal``
        (a platform administrator), or ``platform:application-service:config:reveal`` (an
        application's service account, which the platform confines to its own applications and to
        the clients it is linked to: its explicit clients, else the clients its application is
        enabled for, plus the application-wide value). Raises :class:`NotFoundError` for an
        unknown property and :class:`ValidationError` (``code == "NOT_A_SECRET"``) for a PLAIN
        one (use :meth:`get`). The value is a ``SecretStr`` and is never logged or put in an
        error message by the SDK. It is deliberately **not cached** here; wrap the call in your
        own TTL cache if you need one. ``on_behalf_of`` (who a service account acts for) is sent
        as :data:`ON_BEHALF_OF_HEADER` and recorded in the reveal's audit row.
        """
        response = await self._transport.request(
            "GET",
            _path(REVEAL_OPERATION.path, app, section, property),
            params=_client_params(client_id),
            headers=_actor(on_behalf_of),
        )
        return SecretResponse.model_validate(response.json())
