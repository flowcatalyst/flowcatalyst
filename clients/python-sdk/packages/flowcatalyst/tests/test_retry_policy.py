from __future__ import annotations

from datetime import UTC, datetime

import pytest

from flowcatalyst.retry import (
    DEFAULT_RETRY_STATUSES,
    RetryPolicy,
    Timeouts,
    backoff_ceiling,
    parse_retry_after,
    should_retry_status,
)


def test_default_statuses_exclude_500() -> None:
    assert frozenset({408, 425, 429, 502, 503, 504}) == DEFAULT_RETRY_STATUSES
    assert 500 not in DEFAULT_RETRY_STATUSES


@pytest.mark.parametrize("status", [408, 425, 429, 502, 503, 504])
def test_retryable_statuses(status: int) -> None:
    assert should_retry_status(RetryPolicy(), status, idempotent=False)
    assert should_retry_status(RetryPolicy(), status, idempotent=True)


@pytest.mark.parametrize("status", [200, 400, 401, 403, 404, 409, 422, 500, 501, 505])
def test_non_retryable_statuses(status: int) -> None:
    assert not should_retry_status(RetryPolicy(), status, idempotent=True)


def test_500_is_opt_in_and_idempotent_only() -> None:
    policy = RetryPolicy(retry_server_error=True)
    assert should_retry_status(policy, 500, idempotent=True)
    assert not should_retry_status(policy, 500, idempotent=False)
    assert not should_retry_status(RetryPolicy(), 500, idempotent=True)


def test_defaults_match_the_contract() -> None:
    policy = RetryPolicy()
    assert (policy.max_attempts, policy.base_delay, policy.max_delay) == (3, 0.1, 5.0)
    t = Timeouts()
    assert (t.connect, t.read, t.total) == (5.0, 30.0, 60.0)


def test_backoff_ceiling_doubles_then_caps() -> None:
    p = RetryPolicy()
    assert backoff_ceiling(p, 1) == pytest.approx(0.1)
    assert backoff_ceiling(p, 2) == pytest.approx(0.2)
    assert backoff_ceiling(p, 3) == pytest.approx(0.4)
    assert backoff_ceiling(p, 6) == pytest.approx(3.2)
    assert backoff_ceiling(p, 7) == 5.0
    assert backoff_ceiling(p, 500) == 5.0
    assert backoff_ceiling(p, 5000) == 5.0  # no float overflow


def test_policy_validation() -> None:
    with pytest.raises(ValueError, match="max_attempts"):
        RetryPolicy(max_attempts=0)
    with pytest.raises(ValueError, match="delay"):
        RetryPolicy(base_delay=-1)


def test_retry_after_seconds() -> None:
    assert parse_retry_after("7") == 7.0
    assert parse_retry_after(" 0 ") == 0.0
    assert parse_retry_after("1.5") == 1.5


def test_retry_after_http_date() -> None:
    now = datetime(2026, 10, 16, 12, 0, 0, tzinfo=UTC)
    assert parse_retry_after("Fri, 16 Oct 2026 12:00:30 GMT", now=now) == 30.0
    assert parse_retry_after("Fri, 16 Oct 2026 11:59:00 GMT", now=now) == 0.0  # past -> 0


@pytest.mark.parametrize("bad", [None, "", "soon", "-3", "nan", "inf"])
def test_retry_after_garbage_is_none(bad: str | None) -> None:
    assert parse_retry_after(bad) is None


def test_retry_after_naive_http_date_treated_as_utc() -> None:
    now = datetime(2026, 10, 16, 12, 0, 0, tzinfo=UTC)
    # "-0000" is the RFC 5322 "UTC, local zone unknown" marker: parsed as a naive datetime
    assert parse_retry_after("Fri, 16 Oct 2026 12:00:30 -0000", now=now) == 30.0
