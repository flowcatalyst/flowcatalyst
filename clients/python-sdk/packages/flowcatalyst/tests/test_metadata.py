from __future__ import annotations

from typing import cast

import pytest

from flowcatalyst.metadata import metadata_to_wire


def test_mapping_becomes_key_value_array() -> None:
    assert metadata_to_wire({"a": "1", "b": "2"}) == [
        {"key": "a", "value": "1"},
        {"key": "b", "value": "2"},
    ]


def test_array_passes_through() -> None:
    arr = [{"key": "a", "value": "1"}]
    assert metadata_to_wire(arr) == arr


def test_pairs_accepted() -> None:
    assert metadata_to_wire([("a", "1")]) == [{"key": "a", "value": "1"}]


def test_none_is_empty() -> None:
    assert metadata_to_wire(None) == []


def test_non_string_values_rejected() -> None:
    with pytest.raises(TypeError):
        metadata_to_wire(cast("dict[str, str]", {"a": 1}))


def test_blank_key_rejected() -> None:
    with pytest.raises(ValueError, match="key"):
        metadata_to_wire({" ": "x"})
