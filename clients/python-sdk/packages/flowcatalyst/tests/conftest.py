from __future__ import annotations

from collections.abc import Callable

import pytest

BASE = "https://fc.test"


class Clock:
    """Controllable monotonic clock."""

    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now

    def advance(self, seconds: float) -> None:
        self.now += seconds


class SleepRecorder:
    def __init__(self) -> None:
        self.delays: list[float] = []

    async def __call__(self, delay: float) -> None:
        self.delays.append(delay)


@pytest.fixture
def clock() -> Clock:
    return Clock()


@pytest.fixture
def sleeps() -> SleepRecorder:
    return SleepRecorder()


@pytest.fixture
def full_jitter() -> Callable[[], float]:
    """rng that always draws the top of the jitter window (deterministic ceiling)."""
    return lambda: 1.0


def token_body(token: str = "tok-1", expires_in: int = 3600) -> dict[str, object]:
    return {"access_token": token, "token_type": "Bearer", "expires_in": expires_in}
