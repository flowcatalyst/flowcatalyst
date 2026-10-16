"""HTTP transport: retry / back-off / timeouts / Retry-After / 401 refresh / error mapping."""

from __future__ import annotations

import asyncio
import random
from collections.abc import Awaitable, Callable, Mapping
from typing import Any, Protocol

import httpx

from flowcatalyst.errors import (
    FlowCatalystError,
    NetworkError,
    RequestTimeoutError,
    from_response,
)
from flowcatalyst.retry import (
    RetryPolicy,
    Timeouts,
    backoff_ceiling,
    parse_retry_after,
    should_retry_status,
)

__all__ = ["BearerProvider", "Transport"]

IDEMPOTENT_METHODS = frozenset({"GET", "HEAD", "OPTIONS", "PUT", "DELETE"})
#: Failures where the request provably never reached the server, so any method may retry.
_NOT_SENT = (httpx.ConnectError, httpx.ConnectTimeout, httpx.PoolTimeout)


class BearerProvider(Protocol):
    async def get(self, *, force: bool = False) -> str: ...

    async def refresh(self, *, rejected: str) -> str: ...


class Transport:
    def __init__(
        self,
        client: httpx.AsyncClient,
        base_url: str,
        *,
        auth: BearerProvider | None = None,
        retry: RetryPolicy | None = None,
        timeouts: Timeouts | None = None,
        user_agent: str = "flowcatalyst-python-sdk",
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
        rng: Callable[[], float] = random.random,
    ) -> None:
        self._client = client
        self._base = base_url.rstrip("/")
        self._auth = auth
        self._retry = retry or RetryPolicy()
        self._timeouts = timeouts or Timeouts()
        self._user_agent = user_agent
        self._sleep = sleep
        self._rng = rng

    async def request(
        self,
        method: str,
        path: str,
        *,
        params: Mapping[str, str] | None = None,
        json: Any = None,
        data: Mapping[str, str] | None = None,
        idempotent: bool | None = None,
        timeout: Timeouts | None = None,  # noqa: ASYNC109 - per-call override, not asyncio's
    ) -> httpx.Response:
        """Send a request; return the 2xx response or raise a :class:`FlowCatalystError`.

        ``idempotent`` overrides the method default (use it for POSTs that are safe to
        repeat, e.g. sync endpoints). It only affects retry after a request was sent.
        """
        timeouts = timeout or self._timeouts
        method = method.upper()
        is_idempotent = method in IDEMPOTENT_METHODS if idempotent is None else idempotent
        try:
            async with asyncio.timeout(timeouts.total):
                return await self._send_loop(
                    method, path, params, json, data, is_idempotent, timeouts
                )
        except TimeoutError as exc:
            raise RequestTimeoutError(
                f"{method} {path} exceeded the overall {timeouts.total:g}s deadline"
            ) from exc

    async def _send_loop(
        self,
        method: str,
        path: str,
        params: Mapping[str, str] | None,
        json: Any,
        data: Mapping[str, str] | None,
        idempotent: bool,
        timeouts: Timeouts,
    ) -> httpx.Response:
        attempt = 0
        refreshed = False
        url = f"{self._base}{path}"
        while True:
            attempt += 1
            headers = {"User-Agent": self._user_agent, "Accept": "application/json"}
            token: str | None = None
            if self._auth is not None:
                token = await self._auth.get()
                headers["Authorization"] = f"Bearer {token}"
            try:
                response = await self._client.request(
                    method,
                    url,
                    params=params,
                    json=json,
                    data=data,
                    headers=headers,
                    timeout=timeouts.to_httpx(),
                )
            except httpx.RequestError as exc:
                error = _map_transport_error(exc, method, path)
                if attempt < self._retry.max_attempts and (
                    idempotent or isinstance(exc, _NOT_SENT)
                ):
                    await self._sleep(self._delay(attempt, None))
                    continue
                raise error from exc

            if response.is_success:
                return response
            if response.status_code == 401:
                if token is not None and self._auth is not None and not refreshed:
                    refreshed = True
                    await self._auth.refresh(rejected=token)
                    attempt -= 1  # the refresh retry does not spend the retry budget
                    continue
                raise from_response(response)
            if attempt < self._retry.max_attempts and should_retry_status(
                self._retry, response.status_code, idempotent=idempotent
            ):
                await self._sleep(
                    self._delay(attempt, parse_retry_after(response.headers.get("Retry-After")))
                )
                continue
            raise from_response(response)

    def _delay(self, attempt: int, retry_after: float | None) -> float:
        jittered = self._rng() * backoff_ceiling(self._retry, attempt)
        if retry_after is None:
            return jittered
        return max(jittered, min(retry_after, self._retry.retry_after_cap))


def _map_transport_error(exc: httpx.RequestError, method: str, path: str) -> FlowCatalystError:
    if isinstance(exc, httpx.TimeoutException):
        return RequestTimeoutError(f"{method} {path} timed out ({type(exc).__name__})")
    return NetworkError(f"{method} {path} failed: {type(exc).__name__}: {exc}")
