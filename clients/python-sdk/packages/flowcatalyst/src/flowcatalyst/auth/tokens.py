"""OAuth2 client-credentials token provider: cached, single-flight, early refresh."""

from __future__ import annotations

import asyncio
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, cast

from flowcatalyst.errors import AuthError, FlowCatalystError

if TYPE_CHECKING:
    from flowcatalyst.http import Transport

__all__ = ["ClientCredentials", "ClientCredentialsTokenProvider"]

DEFAULT_MARGIN = 60.0
DEFAULT_LIFETIME = 300.0


@dataclass(frozen=True, slots=True)
class ClientCredentials:
    """A service-account id/secret pair (a plain OAuth client is rejected by the platform)."""

    client_id: str
    client_secret: str = field(repr=False)


class ClientCredentialsTokenProvider:
    """Mints and caches a bearer token via ``POST /oauth/token`` (grant client_credentials).

    * A cached token is reused until ``margin`` seconds before it expires. When the
      token lives less than twice the margin the margin is halved, so a short-lived token
      is still cached instead of being re-minted on every call.
    * Concurrent callers share one fetch (``asyncio.Lock``).
    * :meth:`refresh` replaces a token the platform rejected (401); if another caller
      already replaced it, the newer token is returned without a second fetch.
    * A failed fetch is never cached. 4xx from the token endpoint is :class:`AuthError`;
      network / 5xx failures keep their own type (the platform being down is not "bad
      credentials").
    """

    def __init__(
        self,
        transport: Transport,
        *,
        client_id: str,
        client_secret: str,
        margin: float = DEFAULT_MARGIN,
        token_path: str = "/oauth/token",  # noqa: S107 - URL path, not a secret
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._transport = transport
        self._client_id = client_id
        self._client_secret = client_secret
        self._margin = margin
        self._path = token_path
        self._clock = clock
        self._lock = asyncio.Lock()
        self._token: str | None = None
        self._refresh_at = 0.0

    def __repr__(self) -> str:
        return f"ClientCredentialsTokenProvider(client_id={self._client_id!r})"

    def _fresh(self) -> bool:
        return self._token is not None and self._clock() < self._refresh_at

    async def get(self, *, force: bool = False) -> str:
        if not force and self._token is not None and self._fresh():
            return self._token
        async with self._lock:
            if not force and self._token is not None and self._fresh():
                return self._token
            return await self._fetch()

    async def refresh(self, *, rejected: str) -> str:
        async with self._lock:
            if self._token is not None and self._token != rejected and self._fresh():
                return self._token
            return await self._fetch()

    async def _fetch(self) -> str:
        try:
            response = await self._transport.request(
                "POST",
                self._path,
                data={
                    "grant_type": "client_credentials",
                    "client_id": self._client_id,
                    "client_secret": self._client_secret,
                },
                idempotent=True,
            )
        except AuthError:
            raise
        except FlowCatalystError as exc:
            if exc.status is not None and 400 <= exc.status < 500:
                raise AuthError(
                    f"token request rejected: {exc.message}",
                    status=exc.status,
                    code=exc.code,
                    request_id=exc.request_id,
                ) from exc
            raise
        token, lifetime = _parse_token(response.json())
        margin = min(self._margin, lifetime / 2)
        self._token = token
        self._refresh_at = self._clock() + lifetime - margin
        return token


def _parse_token(body: object) -> tuple[str, float]:
    if not isinstance(body, dict):
        raise AuthError("token response was not a JSON object")
    fields = cast("dict[str, Any]", body)
    token = fields.get("access_token")
    if not isinstance(token, str) or not token:
        raise AuthError("token response had no access_token")
    expires = fields.get("expires_in", DEFAULT_LIFETIME)
    if isinstance(expires, bool) or not isinstance(expires, int | float):
        raise AuthError("token response had a non-numeric expires_in")
    return token, float(expires) if expires > 0 else DEFAULT_LIFETIME
