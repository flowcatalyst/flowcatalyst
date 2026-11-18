from __future__ import annotations

import httpx
import pytest

from flowcatalyst import errors


def resp(status: int, body: object = None, headers: dict[str, str] | None = None) -> httpx.Response:
    return httpx.Response(status, json=body, headers=headers or {})


@pytest.mark.parametrize(
    ("status", "cls"),
    [
        (401, errors.AuthError),
        (403, errors.ForbiddenError),
        (404, errors.NotFoundError),
        (409, errors.ConflictError),
        (400, errors.ValidationError),
        (422, errors.ValidationError),
        (429, errors.RateLimitedError),
        (500, errors.ServerError),
        (503, errors.ServerError),
        (418, errors.FlowCatalystError),
    ],
)
def test_status_maps_to_taxonomy(status: int, cls: type[errors.FlowCatalystError]) -> None:
    err = errors.from_response(resp(status, {"error": "X_CODE", "message": "boom"}))
    assert type(err) is cls
    assert err.status == status
    assert err.code == "X_CODE"
    assert err.message == "boom"
    assert "boom" in str(err)


def test_every_error_is_a_flowcatalyst_error() -> None:
    for cls in (
        errors.NetworkError,
        errors.RequestTimeoutError,
        errors.AuthError,
        errors.ForbiddenError,
        errors.NotFoundError,
        errors.ConflictError,
        errors.ValidationError,
        errors.RateLimitedError,
        errors.ServerError,
        errors.PartialError,
        errors.OutcomeUnknownError,
    ):
        assert issubclass(cls, errors.FlowCatalystError)


def test_outcome_unknown_from_a_gateway_status_is_also_a_server_error() -> None:
    err = errors.outcome_unknown_from_response(
        httpx.Response(502, text="bad gateway", headers={"X-Request-Id": "req-1"}),
        "POST",
        "/api/x",
    )
    assert isinstance(err, errors.OutcomeUnknownError)
    assert isinstance(err, errors.ServerError)
    assert (err.method, err.path, err.status, err.body) == ("POST", "/api/x", 502, "bad gateway")
    assert err.request_id == "req-1"
    assert str(err) == (
        "POST /api/x: outcome unknown (HTTP 502): "
        "the request may have been applied and was not repeated"
    )
    assert repr(err).startswith("OutcomeUnknownError(method='POST'")


@pytest.mark.parametrize(
    ("timeout", "old"), [(True, errors.RequestTimeoutError), (False, errors.NetworkError)]
)
def test_outcome_unknown_from_transport_keeps_the_old_class(
    timeout: bool, old: type[errors.FlowCatalystError]
) -> None:
    cause = httpx.ReadError("reset")
    err = errors.outcome_unknown_from_transport(cause, "POST", "/api/x", timeout=timeout)
    assert isinstance(err, errors.OutcomeUnknownError)
    assert isinstance(err, old)
    assert not isinstance(err, errors.ServerError)
    assert err.status is None
    assert err.cause is cause
    assert "may have been applied and was not repeated" in str(err)


def test_request_id_and_details() -> None:
    err = errors.from_response(
        resp(
            422,
            {"error": "VALIDATION", "message": "bad", "details": {"name": "required"}},
            {"X-Request-Id": "req-9"},
        )
    )
    assert isinstance(err, errors.ValidationError)
    assert err.request_id == "req-9"
    assert err.fields == {"name": "required"}


def test_rate_limited_carries_retry_after() -> None:
    err = errors.from_response(
        resp(429, {"error": "RATE", "message": "slow"}, {"Retry-After": "12"})
    )
    assert isinstance(err, errors.RateLimitedError)
    assert err.retry_after == 12.0


def test_non_json_body_degrades_gracefully() -> None:
    err = errors.from_response(httpx.Response(502, text="<html>bad gateway</html>"))
    assert isinstance(err, errors.ServerError)
    assert err.code is None
    assert "502" in err.message


def test_json_but_not_an_error_model() -> None:
    err = errors.from_response(resp(403, ["nope"]))
    assert isinstance(err, errors.ForbiddenError)
    assert err.code is None
