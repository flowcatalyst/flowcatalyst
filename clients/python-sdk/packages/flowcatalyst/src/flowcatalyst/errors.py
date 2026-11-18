"""One error taxonomy for every SDK call (behaviour contract A.9).

No ``httpx`` exception escapes the SDK; callers catch :class:`FlowCatalystError` or one
of its subclasses. Each error carries the HTTP ``status`` (when there was a response), the
platform ``code`` and ``message``, and the ``request_id`` when the platform sent one.
"""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any, cast

import httpx

from flowcatalyst.retry import parse_retry_after

__all__ = [
    "AuthError",
    "ConflictError",
    "FlowCatalystError",
    "ForbiddenError",
    "NetworkError",
    "NotFoundError",
    "OutcomeUnknownError",
    "PartialError",
    "RateLimitedError",
    "RequestTimeoutError",
    "ServerError",
    "ValidationError",
    "from_response",
]


class FlowCatalystError(Exception):
    """Base class. ``str(err)`` is the platform message (it names a missing permission on 403)."""

    def __init__(
        self,
        message: str,
        *,
        status: int | None = None,
        code: str | None = None,
        request_id: str | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.message = message
        self.status = status
        self.code = code
        self.request_id = request_id
        self.details: dict[str, Any] = details or {}

    def __repr__(self) -> str:
        return (
            f"{type(self).__name__}(status={self.status!r}, code={self.code!r}, "
            f"message={self.message!r}, request_id={self.request_id!r})"
        )


class NetworkError(FlowCatalystError):
    """Connection-level failure; no usable HTTP response."""


class RequestTimeoutError(FlowCatalystError):
    """A connect/read/write timeout, or the overall per-call deadline, expired."""


class AuthError(FlowCatalystError):
    """Token fetch rejected, or 401 after the one refresh-and-retry."""


class ForbiddenError(FlowCatalystError):
    """403."""


class NotFoundError(FlowCatalystError):
    """404."""


class ConflictError(FlowCatalystError):
    """409."""


class ValidationError(FlowCatalystError):
    """400 / 422; ``fields`` is the platform's ``details`` map."""

    @property
    def fields(self) -> dict[str, Any]:
        return self.details


class RateLimitedError(FlowCatalystError):
    """429 after retries; ``retry_after`` is seconds when the platform said."""

    def __init__(self, message: str, *, retry_after: float | None = None, **kw: Any) -> None:
        super().__init__(message, **kw)
        self.retry_after = retry_after


class ServerError(FlowCatalystError):
    """5xx."""


class OutcomeUnknownError(FlowCatalystError):
    """A request that is not idempotent may have been applied, and was not repeated.

    Raised instead of retrying when a POST (not marked ``idempotent=True``) got 502, 503 or
    504 from the gateway, or failed in transport after it may have been sent (reset, read
    timeout, the overall deadline). The platform may have committed it; sending it again could
    duplicate it or be refused (a provisioning answers 409 and its one-time secrets are lost).
    Read the state before deciding to send it again.

    ``method`` and ``path`` name the request; ``status`` is the gateway status, ``None`` for a
    transport failure; ``body`` is the response text (empty without one); ``cause`` is the
    transport exception, also ``__cause__``.

    So that existing ``except`` clauses keep catching what they caught, the instance raised
    is also a :class:`ServerError` (gateway status), a :class:`RequestTimeoutError` (a
    timeout) or a :class:`NetworkError` (any other transport failure). Catch
    ``OutcomeUnknownError`` before those.
    """

    def __init__(
        self,
        message: str,
        *,
        method: str,
        path: str,
        body: str = "",
        cause: BaseException | None = None,
        **kw: Any,
    ) -> None:
        super().__init__(message, **kw)
        self.method = method
        self.path = path
        self.body = body
        self.cause = cause

    def __repr__(self) -> str:
        return (
            f"OutcomeUnknownError(method={self.method!r}, path={self.path!r}, "
            f"status={self.status!r}, cause={self.cause!r}, request_id={self.request_id!r})"
        )


class _GatewayOutcomeUnknownError(OutcomeUnknownError, ServerError):
    """A 502/503/504 on a request that is not idempotent."""


class _TimeoutOutcomeUnknownError(OutcomeUnknownError, RequestTimeoutError):
    """A timeout after a request that is not idempotent may have been sent."""


class _NetworkOutcomeUnknownError(OutcomeUnknownError, NetworkError):
    """A transport failure after a request that is not idempotent may have been sent."""


_UNKNOWN_TAIL = "the request may have been applied and was not repeated"


def outcome_unknown_from_response(
    response: httpx.Response, method: str, path: str
) -> OutcomeUnknownError:
    """The error for a gateway status (502/503/504) on a request that is not idempotent."""
    mapped = from_response(response)
    status = response.status_code
    return _GatewayOutcomeUnknownError(
        f"{method} {path}: outcome unknown (HTTP {status}): {_UNKNOWN_TAIL}",
        method=method,
        path=path,
        body=response.text,
        status=status,
        code=mapped.code,
        request_id=mapped.request_id,
        details=mapped.details,
    )


def outcome_unknown_from_transport(
    cause: BaseException, method: str, path: str, *, timeout: bool
) -> OutcomeUnknownError:
    """The error for a transport failure after a non-idempotent request may have been sent."""
    kind = "timed out" if timeout else f"failed: {type(cause).__name__}: {cause}"
    message = f"{method} {path}: outcome unknown ({kind}): {_UNKNOWN_TAIL}"
    cls = _TimeoutOutcomeUnknownError if timeout else _NetworkOutcomeUnknownError
    return cls(message, method=method, path=path, cause=cause)


class PartialError(FlowCatalystError):
    """A sync / batch where some items failed; ``results`` holds the per-item outcomes."""

    def __init__(self, message: str, *, results: Sequence[Any] = (), **kw: Any) -> None:
        super().__init__(message, **kw)
        self.results = tuple(results)


def from_response(response: httpx.Response) -> FlowCatalystError:
    """Map a non-2xx response to the taxonomy. Never raises."""
    status = response.status_code
    code: str | None = None
    message: str | None = None
    details: dict[str, Any] = {}
    try:
        body: Any = response.json()
    except ValueError:
        body = None
    if isinstance(body, dict):
        fields = cast("dict[str, Any]", body)
        raw_code, raw_msg, raw_details = (
            fields.get("error"),
            fields.get("message"),
            fields.get("details"),
        )
        code = raw_code if isinstance(raw_code, str) else None
        message = raw_msg if isinstance(raw_msg, str) else None
        if isinstance(raw_details, dict):
            details = cast("dict[str, Any]", raw_details)
    text = message or f"platform returned HTTP {status}"
    kw: dict[str, Any] = {
        "status": status,
        "code": code,
        "request_id": response.headers.get("X-Request-Id"),
        "details": details,
    }
    if status == 401:
        return AuthError(text, **kw)
    if status == 403:
        return ForbiddenError(text, **kw)
    if status == 404:
        return NotFoundError(text, **kw)
    if status == 409:
        return ConflictError(text, **kw)
    if status in (400, 422):
        return ValidationError(text, **kw)
    if status == 429:
        return RateLimitedError(
            text, retry_after=parse_retry_after(response.headers.get("Retry-After")), **kw
        )
    if status >= 500:
        return ServerError(text, **kw)
    return FlowCatalystError(text, **kw)
