from __future__ import annotations

import pytest
import respx
from conftest import BASE, token_body

import flowcatalyst
from flowcatalyst import ClientCredentials, FlowCatalyst, Settings

EXPECTED_API = sorted(
    [
        "AuthError",
        "ClientCredentials",
        "ClientCredentialsTokenProvider",
        "ConfigProperty",
        "ConflictError",
        "FlowCatalyst",
        "FlowCatalystError",
        "FlowCatalystSync",
        "ForbiddenError",
        "NetworkError",
        "NotFoundError",
        "PartialError",
        "RateLimitedError",
        "RequestTimeoutError",
        "RetryPolicy",
        "SecretResponse",
        "ServerError",
        "Settings",
        "SyncResult",
        "SyncRole",
        "Timeouts",
        "ValidationError",
        "metadata_to_wire",
    ]
)


def test_public_api_exact() -> None:
    assert sorted(flowcatalyst.__all__) == EXPECTED_API
    for name in flowcatalyst.__all__:
        assert hasattr(flowcatalyst, name)


def test_settings_from_env_default_prefix() -> None:
    env = {
        "FLOWCATALYST_BASE_URL": "https://p.example/",
        "FLOWCATALYST_CLIENT_ID": "id",
        "FLOWCATALYST_CLIENT_SECRET": "sec",
        "FLOWCATALYST_APP_CODE": "app",
    }
    s = Settings.from_env(environ=env)
    assert (s.base_url, s.client_id, s.client_secret, s.app_code) == (
        "https://p.example",
        "id",
        "sec",
        "app",
    )
    assert "sec" not in repr(s)


def test_settings_agentplanner_names() -> None:
    env = {
        "CENTRAL_AGENT_FC_BASE_URL": "https://p.example",
        "CENTRAL_AGENT_FC_SERVICE_CLIENT_ID": "svc",
        "CENTRAL_AGENT_FC_SERVICE_CLIENT_SECRET": "svc-sec",
        "CENTRAL_AGENT_FC_CLIENT_ID": "login-client",  # must NOT be picked up
    }
    s = Settings.from_env(
        prefix="CENTRAL_AGENT_FC_",
        client_id_var="SERVICE_CLIENT_ID",
        client_secret_var="SERVICE_CLIENT_SECRET",
        environ=env,
    )
    assert s.client_id == "svc" and s.client_secret == "svc-sec" and s.app_code is None


def test_settings_missing_names_the_variable() -> None:
    with pytest.raises(KeyError, match="FLOWCATALYST_BASE_URL"):
        Settings.from_env(environ={})


@respx.mock
async def test_from_settings_and_tokens_property() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body("abc"))
    s = Settings(base_url=BASE, client_id="i", client_secret="s")
    async with FlowCatalyst.from_settings(s) as fc:
        assert await fc.tokens.get() == "abc"


async def test_requires_credentials_for_tokens() -> None:
    async with FlowCatalyst(BASE) as fc:
        with pytest.raises(RuntimeError, match="credentials"):
            _ = fc.tokens


def test_credentials_repr_hides_secret() -> None:
    assert "topsecret" not in repr(ClientCredentials("id", "topsecret"))


async def test_closed_client_owned_http_closed() -> None:
    fc = FlowCatalyst(BASE, credentials=ClientCredentials("i", "s"))
    await fc.aclose()
    await fc.aclose()  # idempotent
