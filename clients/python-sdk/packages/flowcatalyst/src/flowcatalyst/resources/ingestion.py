"""``fc.ingestions`` and ``fc.ingestion_sets``: ingestions, their versions, connection tests
and sets. The runtime operations (``/api/ingestion-runtime/v1/*``) are the ingestion runtime's
own client and are deliberately not here."""

from __future__ import annotations

import builtins
from collections.abc import AsyncIterator, Sequence
from dataclasses import dataclass
from typing import Any
from urllib.parse import quote

from flowcatalyst.http import Transport
from flowcatalyst.models.ingestion import (
    ApplyIngestionSetItem,
    Ingestion,
    IngestionDefinition,
    IngestionFailure,
    IngestionFailurePage,
    IngestionHistoryPage,
    IngestionPage,
    IngestionSet,
    IngestionSetApply,
    IngestionSetArchive,
    IngestionSetPlan,
    IngestionSetRelease,
    IngestionSourceKind,
    IngestionTest,
    IngestionVersion,
    SetFile,
)

__all__ = ["IngestionSetsResource", "IngestionsResource"]


@dataclass(frozen=True, slots=True)
class Operation:
    method: str
    path: str
    operation_id: str


#: Pinned by the contract test against api/openapi.lock.json.
OPERATIONS = (
    Operation("GET", "/api/ingestions", "listIngestions"),
    Operation("POST", "/api/ingestions", "createIngestion"),
    Operation("GET", "/api/ingestions/{id}", "getIngestion"),
    Operation("PUT", "/api/ingestions/{id}", "updateIngestion"),
    Operation("DELETE", "/api/ingestions/{id}", "deleteIngestion"),
    Operation("POST", "/api/ingestions/{id}/pause", "pauseIngestion"),
    Operation("POST", "/api/ingestions/{id}/resume", "resumeIngestion"),
    Operation("GET", "/api/ingestions/{id}/versions", "listIngestionVersions"),
    Operation("POST", "/api/ingestions/{id}/versions", "saveIngestionVersion"),
    Operation("POST", "/api/ingestions/{id}/versions/{version}/live", "makeIngestionVersionLive"),
    Operation("GET", "/api/ingestions/{id}/definition", "getIngestionDefinition"),
    Operation("GET", "/api/ingestions/{id}/history", "listIngestionHistory"),
    Operation("GET", "/api/ingestions/{id}/failures", "listIngestionFailures"),
    Operation("GET", "/api/ingestion-source-kinds", "listIngestionSourceKinds"),
    Operation("POST", "/api/ingestion-tests", "testIngestionConnection"),
    Operation("GET", "/api/ingestion-tests/{id}", "getIngestionTest"),
    Operation("PUT", "/api/ingestion-sets/{name}", "applyIngestionSet"),
    Operation("GET", "/api/ingestion-sets", "listIngestionSets"),
    Operation("GET", "/api/ingestion-sets/{name}/applies", "listIngestionSetApplies"),
    Operation("GET", "/api/ingestion-sets/{name}/applies/{applyId}", "getIngestionSetApply"),
    Operation(
        "POST", "/api/ingestion-sets/{name}/applies/{applyId}/reapply", "reapplyIngestionSet"
    ),
    Operation("POST", "/api/ingestion-sets/{name}/release", "releaseIngestionSet"),
)
_BY_ID = {o.operation_id: o for o in OPERATIONS}


def _path(operation_id: str, **segments: str | int) -> str:
    path = _BY_ID[operation_id].path
    for key, value in segments.items():
        path = path.replace("{" + key + "}", quote(str(value), safe=""))
    return path


def _query(**values: str | int | bool | None) -> dict[str, str] | None:
    """Only the parameters that were given; a bool goes as ``true``/``false``."""
    out = {
        k: str(v).lower() if isinstance(v, bool) else str(v)
        for k, v in values.items()
        if v is not None
    }
    return out or None


def _body(definition: IngestionDefinition, **extra: Any) -> dict[str, Any]:
    body = definition.model_dump(by_alias=True, exclude_none=True, mode="json")
    body.update({k: v for k, v in extra.items() if v is not None})
    return body


class IngestionsResource:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def list(
        self,
        *,
        application_code: str | None = None,
        client_id: str | None = None,
        q: str | None = None,
        status: str | None = None,
        kind: str | None = None,
        after: str | None = None,
        limit: int | None = None,
    ) -> IngestionPage:
        """One page of ingestions, filtered by whatever is given. ``client_id`` is an id or an
        identifier; ``limit`` is 1 to 200. Pass ``next_cursor`` back as ``after``."""
        response = await self._transport.request(
            "GET",
            _path("listIngestions"),
            params=_query(
                applicationCode=application_code,
                clientId=client_id,
                q=q,
                status=status,
                kind=kind,
                after=after,
                limit=limit,
            ),
        )
        return IngestionPage.model_validate(response.json())

    async def iter(
        self,
        *,
        application_code: str | None = None,
        client_id: str | None = None,
        q: str | None = None,
        status: str | None = None,
        kind: str | None = None,
        limit: int | None = None,
    ) -> AsyncIterator[Ingestion]:
        """Every matching ingestion, following the cursor page by page."""
        after: str | None = None
        while True:
            page = await self.list(
                application_code=application_code,
                client_id=client_id,
                q=q,
                status=status,
                kind=kind,
                after=after,
                limit=limit,
            )
            for ingestion in page.ingestions:
                yield ingestion
            if not page.has_more or page.next_cursor is None:
                return
            after = page.next_cursor

    async def create(
        self,
        *,
        application: str,
        client: str,
        code: str,
        definition: IngestionDefinition,
        live: bool | None = None,
        note: str | None = None,
    ) -> Ingestion:
        """Create an ingestion. ``application`` is the code of an INTEGRATION application,
        ``client`` a client id or identifier, ``code`` lowercase letters, digits and hyphens,
        unique within the two. ``live=False`` saves the first version without running it.
        A taken code is a :class:`ConflictError` with ``code == "INGESTION_CODE_EXISTS"``."""
        response = await self._transport.request(
            "POST",
            _path("createIngestion"),
            json=_body(
                definition, application=application, client=client, code=code, live=live, note=note
            ),
        )
        return Ingestion.model_validate(response.json())

    async def get(self, id: str) -> Ingestion:
        response = await self._transport.request("GET", _path("getIngestion", id=id))
        return Ingestion.model_validate(response.json())

    async def update(
        self,
        id: str,
        definition: IngestionDefinition,
        *,
        expected_version: int,
        note: str | None = None,
    ) -> Ingestion:
        """Save ``definition`` as the next live version. ``expected_version`` is the live
        version the caller read (0 when none is live); a different current one is a
        :class:`ConflictError`. An unchanged definition writes nothing."""
        response = await self._transport.request(
            "PUT",
            _path("updateIngestion", id=id),
            json=_body(definition, expectedVersion=expected_version, note=note),
            idempotent=True,
        )
        return Ingestion.model_validate(response.json())

    async def delete(self, id: str) -> None:
        await self._transport.request("DELETE", _path("deleteIngestion", id=id))

    async def pause(self, id: str) -> Ingestion:
        response = await self._transport.request(
            "POST", _path("pauseIngestion", id=id), idempotent=True
        )
        return Ingestion.model_validate(response.json())

    async def resume(self, id: str) -> Ingestion:
        response = await self._transport.request(
            "POST", _path("resumeIngestion", id=id), idempotent=True
        )
        return Ingestion.model_validate(response.json())

    async def versions(self, id: str) -> builtins.list[IngestionVersion]:
        response = await self._transport.request("GET", _path("listIngestionVersions", id=id))
        return [IngestionVersion.model_validate(v) for v in response.json()["versions"]]

    async def save_version(
        self,
        id: str,
        definition: IngestionDefinition,
        *,
        live: bool | None = None,
        note: str | None = None,
    ) -> IngestionVersion:
        """Save a version without running it; ``live=True`` also makes it live (needs the
        promote permission)."""
        response = await self._transport.request(
            "POST",
            _path("saveIngestionVersion", id=id),
            json=_body(definition, live=live, note=note),
        )
        return IngestionVersion.model_validate(response.json())

    async def make_version_live(self, id: str, version: int) -> Ingestion:
        response = await self._transport.request(
            "POST",
            _path("makeIngestionVersionLive", id=id, version=version),
            idempotent=True,
        )
        return Ingestion.model_validate(response.json())

    async def definition(self, id: str) -> str:
        """The definition as the YAML file ``fcdev ingest apply`` reads back."""
        response = await self._transport.request(
            "GET", _path("getIngestionDefinition", id=id), accept="application/yaml"
        )
        return response.text

    async def history(
        self, id: str, *, limit: int | None = None, offset: int | None = None
    ) -> IngestionHistoryPage:
        response = await self._transport.request(
            "GET",
            _path("listIngestionHistory", id=id),
            params=_query(limit=limit, offset=offset),
        )
        return IngestionHistoryPage.model_validate(response.json())

    async def failures(
        self, id: str, *, after: str | None = None, limit: int | None = None
    ) -> IngestionFailurePage:
        """One page of failures; pass ``next_cursor`` back as ``after``."""
        response = await self._transport.request(
            "GET",
            _path("listIngestionFailures", id=id),
            params=_query(after=after, limit=limit),
        )
        return IngestionFailurePage.model_validate(response.json())

    async def iter_failures(
        self, id: str, *, limit: int | None = None
    ) -> AsyncIterator[IngestionFailure]:
        """Every failure of an ingestion, following the cursor page by page."""
        after: str | None = None
        while True:
            page = await self.failures(id, after=after, limit=limit)
            for failure in page.failures:
                yield failure
            if not page.has_more or page.next_cursor is None:
                return
            after = page.next_cursor

    async def source_kinds(self) -> builtins.list[IngestionSourceKind]:
        """The built-in kinds plus the kinds the live runtimes report."""
        response = await self._transport.request("GET", _path("listIngestionSourceKinds"))
        return [IngestionSourceKind.model_validate(k) for k in response.json()["kinds"]]

    async def test_connection(
        self,
        *,
        ingestion_id: str | None = None,
        definition: IngestionDefinition | None = None,
        application: str | None = None,
        client: str | None = None,
    ) -> IngestionTest:
        """Queue a test of a saved ingestion's live definition (``ingestion_id``) or of an
        unsaved ``definition`` (with its ``application`` and ``client``). A runtime takes it;
        read the result with :meth:`get_test`."""
        body: dict[str, Any] = {}
        if definition is not None:
            body["definition"] = _body(definition)
        body.update(
            {
                k: v
                for k, v in {
                    "ingestionId": ingestion_id,
                    "application": application,
                    "client": client,
                }.items()
                if v is not None
            }
        )
        response = await self._transport.request(
            "POST", _path("testIngestionConnection"), json=body
        )
        return IngestionTest.model_validate(response.json())

    async def get_test(self, id: str) -> IngestionTest:
        response = await self._transport.request("GET", _path("getIngestionTest", id=id))
        return IngestionTest.model_validate(response.json())


class IngestionSetsResource:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def list(self) -> builtins.list[IngestionSet]:
        response = await self._transport.request("GET", _path("listIngestionSets"))
        return [IngestionSet.model_validate(s) for s in response.json()["sets"]]

    async def apply(
        self,
        name: str,
        items: Sequence[ApplyIngestionSetItem],
        *,
        files: Sequence[SetFile] | None = None,
        dry_run: bool = False,
    ) -> IngestionSetPlan:
        """Make the set ``name`` be exactly ``items``, in one transaction: create, change or
        adopt each, and delete what the set owns but no longer lists. ``dry_run`` reports the
        changes and writes nothing. An ingestion owned by another set is a
        :class:`ConflictError` (``INGESTION_SET_OWNED``) and nothing is written."""
        body: dict[str, Any] = {
            "items": [i.model_dump(by_alias=True, exclude_none=True, mode="json") for i in items]
        }
        if files is not None:
            body["files"] = [f.model_dump(by_alias=True, mode="json") for f in files]
        response = await self._transport.request(
            "PUT",
            _path("applyIngestionSet", name=name),
            params=_query(dryRun=True if dry_run else None),
            json=body,
            idempotent=True,
        )
        return IngestionSetPlan.model_validate(response.json())

    async def applies(self, name: str) -> builtins.list[IngestionSetApply]:
        response = await self._transport.request("GET", _path("listIngestionSetApplies", name=name))
        return [IngestionSetApply.model_validate(a) for a in response.json()["applies"]]

    async def get_apply(self, name: str, apply_id: str) -> IngestionSetArchive:
        """An apply with the items and files it was given."""
        response = await self._transport.request(
            "GET", _path("getIngestionSetApply", name=name, applyId=apply_id)
        )
        return IngestionSetArchive.model_validate(response.json())

    async def reapply(self, name: str, apply_id: str, *, dry_run: bool = False) -> IngestionSetPlan:
        """Apply an archived apply's items again, as a new apply."""
        response = await self._transport.request(
            "POST",
            _path("reapplyIngestionSet", name=name, applyId=apply_id),
            params=_query(dryRun=True if dry_run else None),
        )
        return IngestionSetPlan.model_validate(response.json())

    async def release(self, name: str) -> IngestionSetRelease:
        """End the set's ownership of its ingestions; nothing else changes."""
        response = await self._transport.request(
            "POST", _path("releaseIngestionSet", name=name), idempotent=True
        )
        return IngestionSetRelease.model_validate(response.json())
