"""``FlowCatalyst`` (async) and ``FlowCatalystSync`` (blocking facade)."""

from __future__ import annotations

import asyncio
import threading
from collections.abc import Awaitable, Callable, Coroutine, Sequence
from concurrent.futures import Future
from types import TracebackType
from typing import Any, TypeVar

import httpx

from flowcatalyst.auth.tokens import ClientCredentials, ClientCredentialsTokenProvider
from flowcatalyst.config import Settings
from flowcatalyst.http import Transport
from flowcatalyst.models.platform_config import ConfigProperty, SecretResponse
from flowcatalyst.models.sync import SyncResult, SyncRole
from flowcatalyst.resources.platform_config import PlatformConfigResource
from flowcatalyst.resources.sync import SyncResource
from flowcatalyst.retry import RetryPolicy, Timeouts

__all__ = ["FlowCatalyst", "FlowCatalystSync"]

T = TypeVar("T")


class FlowCatalyst:
    """Async client. Use as ``async with FlowCatalyst(...) as fc``.

    ``http_client`` lets an application share its own ``httpx.AsyncClient`` (it is then
    not closed by :meth:`aclose`). ``sleep`` / ``rng`` exist for deterministic tests.
    """

    def __init__(
        self,
        base_url: str,
        *,
        credentials: ClientCredentials | None = None,
        app_code: str | None = None,
        timeouts: Timeouts | None = None,
        retry: RetryPolicy | None = None,
        http_client: httpx.AsyncClient | None = None,
        user_agent: str = "flowcatalyst-python-sdk",
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
        rng: Callable[[], float] | None = None,
    ) -> None:
        self.app_code = app_code
        self._owns_client = http_client is None
        self._client = http_client or httpx.AsyncClient()
        extra: dict[str, Any] = {} if rng is None else {"rng": rng}
        common: dict[str, Any] = {
            "retry": retry,
            "timeouts": timeouts,
            "user_agent": user_agent,
            "sleep": sleep,
            **extra,
        }
        self._tokens: ClientCredentialsTokenProvider | None = None
        auth = None
        if credentials is not None:
            # the token endpoint is called through an unauthenticated transport
            raw = Transport(self._client, base_url, **common)
            self._tokens = ClientCredentialsTokenProvider(
                raw, client_id=credentials.client_id, client_secret=credentials.client_secret
            )
            auth = self._tokens
        self._transport = Transport(self._client, base_url, auth=auth, **common)
        self.sync = SyncResource(self._transport)
        self.platform_config = PlatformConfigResource(self._transport)

    @classmethod
    def from_settings(cls, settings: Settings, **kwargs: Any) -> FlowCatalyst:
        return cls(
            settings.base_url,
            credentials=ClientCredentials(settings.client_id, settings.client_secret),
            app_code=settings.app_code,
            **kwargs,
        )

    @property
    def tokens(self) -> ClientCredentialsTokenProvider:
        if self._tokens is None:
            raise RuntimeError("this client was created without credentials")
        return self._tokens

    async def aclose(self) -> None:
        if self._owns_client:
            await self._client.aclose()

    async def __aenter__(self) -> FlowCatalyst:
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        await self.aclose()


class _LoopThread:
    """A private event loop on a daemon thread; blocking callers submit coroutines to it."""

    def __init__(self) -> None:
        self._loop = asyncio.new_event_loop()
        self._ready = threading.Event()
        self._closed = False
        self._thread = threading.Thread(
            target=self._run, name="flowcatalyst-sync-loop", daemon=True
        )
        self._thread.start()
        self._ready.wait()

    def _run(self) -> None:
        asyncio.set_event_loop(self._loop)
        self._loop.call_soon(self._ready.set)
        self._loop.run_forever()
        self._loop.close()

    @property
    def closed(self) -> bool:
        return self._closed

    def run(self, coro: Coroutine[Any, Any, T]) -> T:
        try:
            asyncio.get_running_loop()
        except RuntimeError:
            pass
        else:
            coro.close()
            raise RuntimeError(
                "FlowCatalystSync cannot be used inside a running event loop; use FlowCatalyst"
            )
        if self._closed:
            coro.close()
            raise RuntimeError("FlowCatalystSync is closed")
        future: Future[T] = asyncio.run_coroutine_threadsafe(coro, self._loop)
        try:
            return future.result()
        except BaseException:
            future.cancel()  # Ctrl-C while waiting must not leave the request running
            raise

    def stop(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._loop.call_soon_threadsafe(self._loop.stop)
        self._thread.join(timeout=5)


class _SyncTokens:
    def __init__(
        self, runner: _LoopThread, tokens: Callable[[], ClientCredentialsTokenProvider]
    ) -> None:
        self._runner = runner
        self._tokens = tokens

    def get(self, *, force: bool = False) -> str:
        return self._runner.run(self._tokens().get(force=force))


class _SyncSyncResource:
    def __init__(self, runner: _LoopThread, inner: SyncResource) -> None:
        self._runner = runner
        self._inner = inner

    def roles(
        self, app_code: str, roles: Sequence[SyncRole], *, remove_unlisted: bool = False
    ) -> SyncResult:
        return self._runner.run(self._inner.roles(app_code, roles, remove_unlisted=remove_unlisted))


class _SyncPlatformConfigResource:
    def __init__(self, runner: _LoopThread, inner: PlatformConfigResource) -> None:
        self._runner = runner
        self._inner = inner

    def list(self, app: str) -> list[ConfigProperty]:
        return self._runner.run(self._inner.list(app))

    def get(
        self, app: str, section: str, property: str, *, client_id: str | None = None
    ) -> ConfigProperty:
        return self._runner.run(self._inner.get(app, section, property, client_id=client_id))

    def set(
        self,
        app: str,
        section: str,
        property: str,
        value: str,
        *,
        value_type: str | None = None,
        description: str | None = None,
        client_id: str | None = None,
    ) -> ConfigProperty:
        return self._runner.run(
            self._inner.set(
                app,
                section,
                property,
                value,
                value_type=value_type,
                description=description,
                client_id=client_id,
            )
        )

    def delete(
        self, app: str, section: str, property: str, *, client_id: str | None = None
    ) -> None:
        return self._runner.run(self._inner.delete(app, section, property, client_id=client_id))

    def reveal_secret(
        self, app: str, section: str, property: str, *, client_id: str | None = None
    ) -> SecretResponse:
        return self._runner.run(
            self._inner.reveal_secret(app, section, property, client_id=client_id)
        )


class FlowCatalystSync:
    """Blocking facade over :class:`FlowCatalyst` for code that runs outside an event loop
    (boot scripts, CLIs, sync-psycopg code). One implementation: calls run on a private
    loop thread, so it is safe to share across threads."""

    def __init__(self, base_url: str, **kwargs: Any) -> None:
        self._runner = _LoopThread()
        try:
            # the async client (and its httpx client / locks) must be built on its own loop
            self._client: FlowCatalyst = self._runner.run(_build(base_url, kwargs))
        except BaseException:
            self._runner.stop()
            raise
        self.sync = _SyncSyncResource(self._runner, self._client.sync)
        self.platform_config = _SyncPlatformConfigResource(
            self._runner, self._client.platform_config
        )
        self.tokens = _SyncTokens(self._runner, lambda: self._client.tokens)

    def close(self) -> None:
        if self._runner.closed:
            return
        try:
            self._runner.run(self._client.aclose())
        finally:
            self._runner.stop()

    def __enter__(self) -> FlowCatalystSync:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        self.close()


async def _build(base_url: str, kwargs: dict[str, Any]) -> FlowCatalyst:
    return FlowCatalyst(base_url, **kwargs)
