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
