"""Settings read from the environment. Names are explicit so an app keeps its own prefix."""

from __future__ import annotations

import os
from collections.abc import Mapping
from dataclasses import dataclass, field

__all__ = ["Settings"]


@dataclass(frozen=True, slots=True)
class Settings:
    base_url: str
    client_id: str
    client_secret: str = field(repr=False)
    app_code: str | None = None

    @classmethod
    def from_env(
        cls,
        *,
        prefix: str = "FLOWCATALYST_",
        client_id_var: str = "CLIENT_ID",
        client_secret_var: str = "CLIENT_SECRET",  # noqa: S107 - variable NAME
        environ: Mapping[str, str] | None = None,
    ) -> Settings:
        """Read ``{prefix}BASE_URL``, ``{prefix}{client_id_var}``, ``{prefix}{client_secret_var}``
        and optional ``{prefix}APP_CODE``. A missing variable raises ``KeyError`` naming it.

        No fallback between id variables: silently using a login client where a service
        account is needed makes the platform's token endpoint fail with a confusing 500.
        """
        env = os.environ if environ is None else environ

        def need(name: str) -> str:
            full = f"{prefix}{name}"
            try:
                return env[full]
            except KeyError:
                raise KeyError(f"missing environment variable {full}") from None

        return cls(
            base_url=need("BASE_URL").rstrip("/"),
            client_id=need(client_id_var),
            client_secret=need(client_secret_var),
            app_code=env.get(f"{prefix}APP_CODE") or None,
        )
