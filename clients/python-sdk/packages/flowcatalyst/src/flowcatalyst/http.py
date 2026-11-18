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
    outcome_unknown_from_response,
    outcome_unknown_from_transport,
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
_NOT_SENT = (httpx.ConnectError, httpx.ConnectTimeout, httpx.PoolTimeout, httpx.UnsupportedProtocol)
#: Gateway statuses: the platform may have processed the request and the answer was lost.
_GATEWAY = frozenset({502, 503, 504})


class _Attempts:
    """What the send loop knows about the call so far (read when the deadline expires)."""

    __slots__ = ("earlier_unknown", "in_flight")

    def __init__(self) -> None:
        #: An attempt is on the wire: the deadline expiring now leaves the outcome unknown.
        self.in_flight = False
        #: An earlier attempt may have been applied (a gateway status or a failure after
        #: sending) and was retried.
        self.earlier_unknown = False


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
        accept: str = "application/json",
        idempotent: bool | None = None,
        timeout: Timeouts | None = None,  # noqa: ASYNC109 - per-call override, not asyncio's
        headers: Mapping[str, str] | None = None,
    ) -> httpx.Response:
        """Send a request; return the 2xx response or raise a :class:`FlowCatalystError`.

        ``idempotent`` overrides the method default (GET, HEAD, OPTIONS, PUT and DELETE are
        idempotent). Mark a POST ``idempotent=True`` only when the platform treats a repeat as
        harmless (a declarative sync, a read, the token request). It decides what happens
        after the request may have reached the platform: a 502/503/504 or a transport failure
        after sending is retried for an idempotent request and raises
        :class:`~flowcatalyst.errors.OutcomeUnknownError` for any other. 408, 425, 429 and a
        failure to connect are retried for every request.

        A DELETE answered 404 on a retry after such an unknown outcome has succeeded (the
        first attempt most likely deleted it): an empty 204 response is returned.

        ``headers`` are sent besides the SDK's own (which they cannot replace).
        """
        timeouts = timeout or self._timeouts
        method = method.upper()
        is_idempotent = method in IDEMPOTENT_METHODS if idempotent is None else idempotent
        attempts = _Attempts()
        try:
            async with asyncio.timeout(timeouts.total):
                return await self._send_loop(
                    method,
                    path,
                    params,
                    json,
                    data,
                    accept,
                    is_idempotent,
                    timeouts,
                    headers,
                    attempts,
                )
        except TimeoutError as exc:
            if attempts.in_flight and not is_idempotent:
                raise outcome_unknown_from_transport(exc, method, path, timeout=True) from exc
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
        accept: str,
        idempotent: bool,
        timeouts: Timeouts,
        extra_headers: Mapping[str, str] | None,
        state: _Attempts,
    ) -> httpx.Response:
        attempt = 0
        refreshed = False
        url = f"{self._base}{path}"
        while True:
            attempt += 1
            headers = {**(extra_headers or {}), "User-Agent": self._user_agent, "Accept": accept}
            token: str | None = None
            if self._auth is not None:
                token = await self._auth.get()
                headers["Authorization"] = f"Bearer {token}"
            state.in_flight = True
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
                state.in_flight = False
                not_sent = isinstance(exc, _NOT_SENT)
                if not not_sent and not idempotent:
                    raise outcome_unknown_from_transport(
                        exc, method, path, timeout=isinstance(exc, httpx.TimeoutException)
                    ) from exc
                if attempt < self._retry.max_attempts:
                    if not not_sent:
                        state.earlier_unknown = True
                    await self._sleep(self._delay(attempt, None))
                    continue
                raise _map_transport_error(exc, method, path) from exc
            state.in_flight = False

            if response.is_success:
                return response
            if response.status_code == 404 and method == "DELETE" and state.earlier_unknown:
                # The resource is gone and an earlier attempt most likely removed it.
                return httpx.Response(204, request=response.request)
            if response.status_code == 401:
                if token is not None and self._auth is not None and not refreshed:
                    refreshed = True
                    await self._auth.refresh(rejected=token)
                    attempt -= 1  # the refresh retry does not spend the retry budget
                    continue
                raise from_response(response)
            status = response.status_code
            if status in _GATEWAY and not idempotent:
                raise outcome_unknown_from_response(response, method, path)
            if attempt < self._retry.max_attempts and should_retry_status(
                self._retry, status, idempotent=idempotent
            ):
                if status in _GATEWAY:
                    state.earlier_unknown = True
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
