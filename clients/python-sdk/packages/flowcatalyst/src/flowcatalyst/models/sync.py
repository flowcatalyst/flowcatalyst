"""Role-sync models. Requests forbid unknown fields (the platform rejects them);
responses ignore them (forward compatible)."""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field, field_validator
from pydantic.alias_generators import to_camel

__all__ = ["SyncResult", "SyncRole"]


class SyncRole(BaseModel):
    """One role to register. ``name`` is unprefixed: the platform adds ``{app}:`` itself."""

    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="forbid")

    name: str
    display_name: str | None = None
    description: str | None = None
    permissions: list[str] | None = None
    client_managed: bool | None = None

    @field_validator("name")
    @classmethod
    def _unprefixed(cls, value: str) -> str:
        if ":" in value:
            raise ValueError(
                "role name must be unprefixed (the platform adds the application prefix)"
            )
        return value


class SyncResult(BaseModel):
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="ignore")

    application_code: str
    created: int
    updated: int
    deleted: int
    synced_codes: list[str]
    skipped_roles: list[str] = Field(default_factory=list)
    password_hash_ignored: list[str] = Field(default_factory=list)
