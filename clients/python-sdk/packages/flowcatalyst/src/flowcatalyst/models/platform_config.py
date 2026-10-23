"""Platform-config models. Requests forbid unknown fields; responses ignore them."""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, SecretStr
from pydantic.alias_generators import to_camel

__all__ = ["ConfigProperty", "SecretResponse"]

_RESPONSE = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="ignore")


class ConfigProperty(BaseModel):
    """One platform-config property.

    ``value`` is the literal ``"***"`` for every ``value_type == "SECRET"`` property, for every
    caller. Read the plaintext with :meth:`PlatformConfigResource.reveal_secret`.
    """

    model_config = _RESPONSE

    id: str
    application_code: str
    section: str
    property: str
    scope: str
    value_type: Literal["PLAIN", "SECRET"] | str
    value: str
    created_at: datetime
    updated_at: datetime
    client_id: str | None = None
    description: str | None = None


class SecretResponse(BaseModel):
    """A revealed secret. ``value`` is a :class:`pydantic.SecretStr`: ``repr``, ``str`` and
    ``model_dump_json`` show ``**********``; call ``value.get_secret_value()`` for the plaintext.
    ``updated_at`` is the secret's version (changes whenever it is set)."""

    model_config = _RESPONSE

    application_code: str
    section: str
    property: str
    value: SecretStr
    updated_at: datetime
    client_id: str | None = None
