"""Metadata is an array of ``{key, value}`` on the wire; the SDK also accepts a mapping."""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from typing import cast

__all__ = ["metadata_to_wire"]

MetadataInput = Mapping[str, str] | Iterable[Mapping[str, str] | tuple[str, str]] | None


def _pairs(
    metadata: Mapping[str, str] | Iterable[Mapping[str, str] | tuple[str, str]],
) -> list[tuple[str, str]]:
    if isinstance(metadata, Mapping):
        return list(cast("Mapping[str, str]", metadata).items())
    pairs: list[tuple[str, str]] = []
    for item in metadata:
        if isinstance(item, Mapping):
            pairs.append((item["key"], item["value"]))
        else:
            pairs.append((item[0], item[1]))
    return pairs


def metadata_to_wire(metadata: MetadataInput) -> list[dict[str, str]]:
    if metadata is None:
        return []
    out: list[dict[str, str]] = []
    for key, value in _pairs(metadata):
        # runtime guards: callers are not always type-checked
        if not key.strip():
            raise ValueError("metadata key must be a non-blank string")
        if type(value) is not str:
            raise TypeError(f"metadata value for {key!r} must be a string")
        out.append({"key": key, "value": value})
    return out
