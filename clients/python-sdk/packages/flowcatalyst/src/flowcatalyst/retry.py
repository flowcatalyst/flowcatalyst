"""Retry and timeout policy (behaviour contract A.9). Pure functions, no I/O."""

from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import UTC, datetime
from email.utils import parsedate_to_datetime

import httpx

__all__ = [
    "DEFAULT_RETRY_STATUSES",
    "RetryPolicy",
    "Timeouts",
    "backoff_ceiling",
    "parse_retry_after",
    "should_retry_status",
]

#: 500 is deliberately absent: a 500 may have done the work.
DEFAULT_RETRY_STATUSES: frozenset[int] = frozenset({408, 425, 429, 502, 503, 504})


@dataclass(frozen=True, slots=True)
class RetryPolicy:
    max_attempts: int = 3
    base_delay: float = 0.1
    max_delay: float = 5.0
    #: Upper bound on how long a ``Retry-After`` header can make us wait.
    retry_after_cap: float = 30.0
    retry_statuses: frozenset[int] = DEFAULT_RETRY_STATUSES
    #: Opt in to retrying 500 (idempotent requests only).
    retry_server_error: bool = False

    def __post_init__(self) -> None:
        if self.max_attempts < 1:
            raise ValueError("max_attempts must be >= 1")
        if self.base_delay < 0 or self.max_delay < 0 or self.retry_after_cap < 0:
            raise ValueError("delay values must be >= 0")


@dataclass(frozen=True, slots=True)
class Timeouts:
    connect: float = 5.0
    read: float = 30.0
    write: float = 30.0
    pool: float = 5.0
    #: Overall deadline for one call, retries and back-off included.
    total: float = 60.0

    def to_httpx(self) -> httpx.Timeout:
        return httpx.Timeout(connect=self.connect, read=self.read, write=self.write, pool=self.pool)


def should_retry_status(policy: RetryPolicy, status: int, *, idempotent: bool) -> bool:
    if status in policy.retry_statuses:
        return True
    return policy.retry_server_error and status == 500 and idempotent


def backoff_ceiling(policy: RetryPolicy, attempt: int) -> float:
    """Top of the full-jitter window after ``attempt`` (1-based) failed attempts."""
    exponent = min(attempt - 1, 60)  # avoid float overflow for absurd attempt counts
    return min(policy.max_delay, policy.base_delay * (2.0**exponent))


def parse_retry_after(value: str | None, *, now: datetime | None = None) -> float | None:
    """Seconds to wait from a ``Retry-After`` header (delta-seconds or HTTP date)."""
    if value is None:
        return None
    text = value.strip()
    if not text:
        return None
    try:
        seconds = float(text)
    except ValueError:
        try:
            when = parsedate_to_datetime(text)
        except (TypeError, ValueError):
            return None
        if when.tzinfo is None:
            when = when.replace(tzinfo=UTC)
        return max(0.0, (when - (now or datetime.now(UTC))).total_seconds())
    if not math.isfinite(seconds) or seconds < 0:
        return None
    return seconds
