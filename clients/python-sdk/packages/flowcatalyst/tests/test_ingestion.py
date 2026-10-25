from __future__ import annotations

from typing import Any

import httpx
import pytest
import respx
from conftest import BASE, token_body
from pydantic import ValidationError as PydanticValidationError

from flowcatalyst import (
    ApplyIngestionSetItem,
    ClientCredentials,
    ConflictError,
    CustomSource,
    FlowCatalyst,
    FlowCatalystSync,
    ForbiddenError,
    IngestionDefinition,
    NotFoundError,
    Target,
    ValidationError,
)
from flowcatalyst.models.ingestion import (
    ApiPollSource,
    BearerAuth,
    DatabasePollSource,
    HttpSource,
    Ingestion,
    JsonSplit,
    NoAuth,
    RecordResults,
    S3Source,
    SecretRef,
    SetFile,
    SqsSource,
    StatusTrack,
)

TARGET: dict[str, Any] = {
    "code": "orders",
    "targetUrl": "https://api.example.com/orders",
    "pool": "default",
    "mode": "IMMEDIATE",
    "batchSize": 10,
}
HTTP_SOURCE: dict[str, Any] = {
    "kind": "http",
    "auth": {"kind": "bearer", "token": {"name": "orders-token"}},
}
INGESTION: dict[str, Any] = {
    "id": "ing_1",
    "applicationId": "app_1",
    "applicationCode": "erp",
    "clientId": "clt_1",
    "clientIdentifier": "acme",
    "code": "orders",
    "status": "ACTIVE",
    "version": 2,
    "latestVersion": 3,
    "createdAt": "2026-01-01T00:00:00Z",
    "updatedAt": "2026-01-02T00:00:00Z",
    "name": "Orders",
    "source": HTTP_SOURCE,
    "split": {"kind": "json", "arrayPath": "$.orders"},
    "target": TARGET,
}
VERSION: dict[str, Any] = {
    "id": "ver_1",
    "version": 3,
    "live": False,
    "createdAt": "2026-01-02T00:00:00Z",
    "name": "Orders",
    "source": HTTP_SOURCE,
    "split": {"kind": "none"},
    "target": TARGET,
}
PLAN: dict[str, Any] = {
    "name": "erp",
    "dryRun": True,
    "changes": [
        {
            "application": "erp",
            "client": "acme",
            "code": "orders",
            "action": "CREATE",
            "changed": [],
        }
    ],
}


def definition() -> IngestionDefinition:
    return IngestionDefinition(
        name="Orders",
        source=HttpSource(auth=BearerAuth(token=SecretRef(name="orders-token"))),
        split=JsonSplit(array_path="$.orders"),
        target=Target(
            code="orders",
            target_url="https://api.example.com/orders",
            pool="default",
            mode="IMMEDIATE",
            batch_size=10,
        ),
    )


def client() -> FlowCatalyst:
    return FlowCatalyst(BASE, credentials=ClientCredentials("sa", "secret"))


def token() -> None:
    respx.post(f"{BASE}/oauth/token").respond(200, json=token_body())


@respx.mock
async def test_list_sends_only_the_filters_given_and_follows_the_cursor() -> None:
    token()
    route = respx.get(f"{BASE}/api/ingestions")
    route.side_effect = [
        httpx.Response(200, json={"ingestions": [INGESTION], "hasMore": True, "nextCursor": "c1"}),
        httpx.Response(200, json={"ingestions": [INGESTION], "hasMore": True, "nextCursor": "c1"}),
        httpx.Response(200, json={"ingestions": [INGESTION], "hasMore": False}),
    ]
    async with client() as fc:
        first = await fc.ingestions.list()
        assert _req(route).url.query == b""
        assert first.has_more and first.next_cursor == "c1"
        seen = [i.id async for i in fc.ingestions.iter(client_id="acme", limit=1)]
    assert seen == ["ing_1", "ing_1"]
    second, third = _req(route, 1), _req(route, 2)
    assert dict(second.url.params) == {"clientId": "acme", "limit": "1"}
    assert dict(third.url.params) == {"clientId": "acme", "limit": "1", "after": "c1"}


@respx.mock
async def test_list_sends_every_filter() -> None:
    token()
    route = respx.get(f"{BASE}/api/ingestions").respond(
        200, json={"ingestions": [], "hasMore": False}
    )
    async with client() as fc:
        await fc.ingestions.list(
            application_code="erp", client_id="clt_1", q="ord", status="PAUSED", kind="sftp"
        )
    assert dict(_req(route).url.params) == {
        "applicationCode": "erp",
        "clientId": "clt_1",
        "q": "ord",
        "status": "PAUSED",
        "kind": "sftp",
    }


@respx.mock
async def test_create_body_and_response() -> None:
    token()
    route = respx.post(f"{BASE}/api/ingestions").respond(201, json=INGESTION)
    async with client() as fc:
        created = await fc.ingestions.create(
            application="erp", client="acme", code="orders", definition=definition()
        )
    body = _json(route)
    assert body == {
        "application": "erp",
        "client": "acme",
        "code": "orders",
        "name": "Orders",
        "source": HTTP_SOURCE,
        "split": {"kind": "json", "arrayPath": "$.orders"},
        "target": TARGET,
    }
    assert isinstance(created.source, HttpSource) and created.version == 2


@respx.mock
async def test_create_sends_live_false_and_note() -> None:
    token()
    route = respx.post(f"{BASE}/api/ingestions").respond(201, json=INGESTION)
    async with client() as fc:
        await fc.ingestions.create(
            application="erp",
            client="acme",
            code="orders",
            definition=definition(),
            live=False,
            note="first",
        )
    assert _json(route)["live"] is False and _json(route)["note"] == "first"


@respx.mock
async def test_get_url_encodes_the_id() -> None:
    token()
    route = respx.get(url__startswith=f"{BASE}/api/ingestions/").respond(200, json=INGESTION)
    async with client() as fc:
        await fc.ingestions.get("a/b c")
    assert _req(route).url.raw_path == b"/api/ingestions/a%2Fb%20c"


@respx.mock
async def test_update_sends_expected_version() -> None:
    token()
    route = respx.put(f"{BASE}/api/ingestions/ing_1").respond(200, json=INGESTION)
    async with client() as fc:
        await fc.ingestions.update("ing_1", definition(), expected_version=2, note="n")
    body = _json(route)
    assert body["expectedVersion"] == 2 and body["note"] == "n" and body["name"] == "Orders"
    assert "application" not in body and "code" not in body


@respx.mock
async def test_delete_pause_resume_and_make_live_paths() -> None:
    token()
    delete = respx.delete(f"{BASE}/api/ingestions/ing_1").respond(204)
    pause = respx.post(f"{BASE}/api/ingestions/ing_1/pause").respond(200, json=INGESTION)
    resume = respx.post(f"{BASE}/api/ingestions/ing_1/resume").respond(200, json=INGESTION)
    live = respx.post(f"{BASE}/api/ingestions/ing_1/versions/3/live").respond(200, json=INGESTION)
    async with client() as fc:
        await fc.ingestions.delete("ing_1")
        await fc.ingestions.pause("ing_1")
        await fc.ingestions.resume("ing_1")
        await fc.ingestions.make_version_live("ing_1", 3)
    assert delete.called and pause.called and resume.called and live.called
    assert _req(live).content == b""


@respx.mock
async def test_versions_list_and_save() -> None:
    token()
    respx.get(f"{BASE}/api/ingestions/ing_1/versions").respond(200, json={"versions": [VERSION]})
    save = respx.post(f"{BASE}/api/ingestions/ing_1/versions").respond(201, json=VERSION)
    async with client() as fc:
        versions = await fc.ingestions.versions("ing_1")
        saved = await fc.ingestions.save_version("ing_1", definition(), live=True, note="x")
    assert [v.version for v in versions] == [3] and saved.live is False
    assert _json(save)["live"] is True and _json(save)["note"] == "x"


@respx.mock
async def test_definition_asks_for_yaml_and_returns_text() -> None:
    token()
    route = respx.get(f"{BASE}/api/ingestions/ing_1/definition").respond(
        200, text="name: Orders\n", headers={"Content-Type": "application/yaml"}
    )
    async with client() as fc:
        text = await fc.ingestions.definition("ing_1")
    assert text == "name: Orders\n"
    assert _req(route).headers["accept"] == "application/yaml"


@respx.mock
async def test_history_and_failures_with_cursor() -> None:
    token()
    history = respx.get(f"{BASE}/api/ingestions/ing_1/history").respond(
        200,
        json={
            "entries": [
                {"id": "a1", "operation": "PauseIngestion", "performedAt": "2026-01-01T00:00:00Z"}
            ],
            "hasMore": False,
        },
    )
    failure = {
        "id": "f1",
        "kind": "sftp",
        "message": "boom",
        "afterRecords": 0,
        "occurredAt": "2026-01-01T00:00:00Z",
    }
    failures = respx.get(f"{BASE}/api/ingestions/ing_1/failures")
    failures.side_effect = [
        httpx.Response(200, json={"failures": [failure], "hasMore": True, "nextCursor": "f1"}),
        httpx.Response(200, json={"failures": [failure | {"id": "f0"}], "hasMore": False}),
    ]
    async with client() as fc:
        page = await fc.ingestions.history("ing_1", limit=5, offset=10)
        ids = [f.id async for f in fc.ingestions.iter_failures("ing_1")]
    assert dict(_req(history).url.params) == {"limit": "5", "offset": "10"}
    assert page.entries[0].operation == "PauseIngestion"
    assert ids == ["f1", "f0"]
    assert _req(failures, 0).url.query == b""
    assert _req(failures, 1).url.params["after"] == "f1"


@respx.mock
async def test_source_kinds() -> None:
    token()
    respx.get(f"{BASE}/api/ingestion-source-kinds").respond(
        200,
        json={
            "kinds": [
                {"kind": "http", "builtIn": True, "singleOwner": False},
                {
                    "kind": "camel-route",
                    "builtIn": False,
                    "singleOwner": True,
                    "requiredPermission": "platform:ingestion:ingestion:route",
                    "settings": {"yaml": "string"},
                },
            ]
        },
    )
    async with client() as fc:
        kinds = await fc.ingestions.source_kinds()
    assert kinds[1].required_permission == "platform:ingestion:ingestion:route"


@respx.mock
async def test_test_connection_by_id_and_by_definition() -> None:
    token()
    test = {
        "id": "tst_1",
        "kind": "http",
        "status": "PENDING",
        "createdAt": "2026-01-01T00:00:00Z",
        "expiresAt": "2026-01-01T00:05:00Z",
        "notice": "no runtime carries the kind",
    }
    post = respx.post(f"{BASE}/api/ingestion-tests").respond(202, json=test)
    get = respx.get(f"{BASE}/api/ingestion-tests/tst_1").respond(
        200, json=test | {"status": "DONE", "outcome": "RESULTS", "records": 3, "steps": ["a"]}
    )
    async with client() as fc:
        queued = await fc.ingestions.test_connection(ingestion_id="ing_1")
        assert _json(post) == {"ingestionId": "ing_1"}
        await fc.ingestions.test_connection(
            definition=definition(), application="erp", client="acme"
        )
        done = await fc.ingestions.get_test(queued.id)
    body = _json(post)
    assert body["application"] == "erp" and body["client"] == "acme"
    assert body["definition"]["name"] == "Orders" and "ingestionId" not in body
    assert queued.notice and done.outcome == "RESULTS" and done.records == 3
    assert get.called


@respx.mock
async def test_set_apply_dry_run_flag_only_when_asked() -> None:
    token()
    route = respx.put(f"{BASE}/api/ingestion-sets/my%20set").respond(200, json=PLAN)
    item = ApplyIngestionSetItem(
        application="erp",
        client="acme",
        code="orders",
        name="Orders",
        source=definition().source,
        split=definition().split,
        target=definition().target,
        active=True,
    )
    async with client() as fc:
        await fc.ingestion_sets.apply("my set", [item])
        assert _req(route).url.query == b""
        plan = await fc.ingestion_sets.apply(
            "my set", [item], files=[SetFile(path="a.yaml", content="x")], dry_run=True
        )
    request = _req(route)
    assert request.url.params["dryRun"] == "true"
    body = _json(route)
    assert body["items"][0]["active"] is True and body["files"] == [
        {"path": "a.yaml", "content": "x"}
    ]
    assert plan.dry_run and plan.changes[0].action == "CREATE" and plan.apply_id is None


@respx.mock
async def test_set_listing_applies_archive_reapply_release() -> None:
    token()
    respx.get(f"{BASE}/api/ingestion-sets").respond(
        200,
        json={
            "sets": [
                {
                    "id": "s1",
                    "name": "erp",
                    "ingestions": 2,
                    "createdAt": "2026-01-01T00:00:00Z",
                    "updatedAt": "2026-01-01T00:00:00Z",
                }
            ]
        },
    )
    apply_json = {
        "id": "ap_1",
        "set": "erp",
        "appliedAt": "2026-01-01T00:00:00Z",
        "items": 1,
        "files": 0,
        "changes": PLAN["changes"],
    }
    respx.get(f"{BASE}/api/ingestion-sets/erp/applies").respond(200, json={"applies": [apply_json]})
    respx.get(f"{BASE}/api/ingestion-sets/erp/applies/ap_1").respond(
        200,
        json={
            "set": "erp",
            "applyId": "ap_1",
            "appliedAt": "2026-01-01T00:00:00Z",
            "appliedBy": "prn_1",
            "items": [
                {
                    "application": "erp",
                    "client": "acme",
                    "code": "orders",
                    "name": "Orders",
                    "source": HTTP_SOURCE,
                    "split": {"kind": "none"},
                    "target": TARGET,
                }
            ],
            "files": [{"path": "a.yaml", "content": "x"}],
        },
    )
    reapply = respx.post(f"{BASE}/api/ingestion-sets/erp/applies/ap_1/reapply").respond(
        200, json=PLAN | {"applyId": "ap_2"}
    )
    release = respx.post(f"{BASE}/api/ingestion-sets/erp/release").respond(
        200, json={"name": "erp", "released": 2}
    )
    async with client() as fc:
        assert (await fc.ingestion_sets.list())[0].ingestions == 2
        assert (await fc.ingestion_sets.applies("erp"))[0].id == "ap_1"
        archive = await fc.ingestion_sets.get_apply("erp", "ap_1")
        plan = await fc.ingestion_sets.reapply("erp", "ap_1")
        assert _req(reapply).url.query == b""
        await fc.ingestion_sets.reapply("erp", "ap_1", dry_run=True)
        released = await fc.ingestion_sets.release("erp")
    assert archive.items[0].code == "orders" and archive.files[0].path == "a.yaml"
    assert plan.apply_id == "ap_2" and _req(reapply).url.params["dryRun"] == "true"
    assert released.released == 2 and release.called


@pytest.mark.parametrize(
    ("status", "body", "exc", "code"),
    [
        (400, {"error": "INGESTION_TARGET_INVALID", "message": "pool"}, ValidationError, None),
        (403, {"error": "FORBIDDEN", "message": "no"}, ForbiddenError, None),
        (404, {"error": "INGESTION_NOT_FOUND", "message": "none"}, NotFoundError, None),
        (409, {"error": "INGESTION_CODE_EXISTS", "message": "taken"}, ConflictError, None),
    ],
)
@respx.mock
async def test_create_error_statuses(
    status: int, body: dict[str, str], exc: type[Exception], code: str | None
) -> None:
    token()
    respx.post(f"{BASE}/api/ingestions").respond(status, json=body)
    async with client() as fc:
        with pytest.raises(exc) as info:
            await fc.ingestions.create(
                application="erp", client="acme", code="orders", definition=definition()
            )
    assert getattr(info.value, "code", None) == body["error"]


@respx.mock
async def test_set_owned_is_a_conflict_with_its_code() -> None:
    token()
    respx.put(f"{BASE}/api/ingestion-sets/erp").respond(
        409, json={"error": "INGESTION_SET_OWNED", "message": "owned by another set"}
    )
    async with client() as fc:
        with pytest.raises(ConflictError) as info:
            await fc.ingestion_sets.apply("erp", [])
    assert info.value.code == "INGESTION_SET_OWNED"


@respx.mock
async def test_update_version_conflict_is_not_retried() -> None:
    token()
    route = respx.put(f"{BASE}/api/ingestions/ing_1").respond(
        409, json={"error": "VERSION_CONFLICT", "message": "moved"}
    )
    async with client() as fc:
        with pytest.raises(ConflictError):
            await fc.ingestions.update("ing_1", definition(), expected_version=1)
    assert route.call_count == 1


# --- the typed form --------------------------------------------------------------------------


def test_every_source_kind_round_trips_through_the_wire_form() -> None:
    wires: list[dict[str, Any]] = [
        HTTP_SOURCE | {"rateLimit": {"requests": 10, "per": "PT1S"}},
        {
            "kind": "sftp",
            "host": "h",
            "port": 22,
            "directory": "/in",
            "username": "u",
            "credential": {"name": "sftp-key"},
            "include": "*.csv",
            "every": "PT5M",
        },
        {
            "kind": "s3",
            "region": "eu-west-1",
            "bucket": "b",
            "every": "PT1M",
            "credential": {"accessKeyId": "AK", "secretAccessKey": {"name": "aws-secret"}},
        },
        {"kind": "sqs", "region": "eu-west-1", "queue": "q"},
        {
            "kind": "rabbitmq",
            "uri": "amqps://mq",
            "queue": "q",
            "username": "u",
            "password": {"name": "mq-pass"},
        },
        {
            "kind": "database-poll",
            "jdbcUrl": "jdbc:postgresql://db/x",
            "username": "u",
            "password": {"name": "db-pass"},
            "table": "t",
            "track": {
                "kind": "status",
                "column": "s",
                "keyColumn": "id",
                "pending": "N",
                "processed": "Y",
            },
            "maxRows": 100,
            "every": "PT30S",
        },
        {
            "kind": "api-poll",
            "url": "https://api/x",
            "method": "GET",
            "headers": [{"name": "Accept", "value": "application/json"}],
            "auth": {
                "kind": "oauth-client-credentials",
                "tokenUrl": "https://idp/token",
                "clientId": "c",
                "clientSecret": {"name": "oauth"},
                "credentialsIn": "BODY",
            },
            "results": {"kind": "json-path", "path": "$.items"},
            "cursor": {"from": "$.next", "param": "after"},
            "every": "PT1M",
            "idleDelay": "PT5M",
        },
        {"kind": "custom", "type": "camel-route", "settings": {"yaml": "- from: x"}},
    ]
    for wire in wires:
        target = {
            "name": "N",
            "source": wire,
            "split": {"kind": "csv", "header": True},
            "target": TARGET,
            "keepFailedPayloads": True,
        }
        parsed = IngestionDefinition.model_validate(target)
        assert parsed.model_dump(by_alias=True, exclude_none=True, mode="json") == target


def test_custom_source_is_the_escape_hatch() -> None:
    source = CustomSource(type="mix", settings={"queue": "orders"})
    assert source.model_dump(by_alias=True, exclude_none=True) == {
        "kind": "custom",
        "type": "mix",
        "settings": {"queue": "orders"},
    }


def test_typed_variants_construct_and_unknown_fields_are_refused() -> None:
    DatabasePollSource(
        jdbc_url="jdbc:x",
        username="u",
        password=SecretRef(name="p"),
        table="t",
        track=StatusTrack(column="s", key_column="id", pending="N", processed="Y"),
        max_rows=1,
        every="PT1S",
    )
    ApiPollSource(
        url="u",
        method="GET",
        auth=NoAuth(),
        results=RecordResults(),
        every="PT1S",
        idle_delay="PT1S",
    )
    SqsSource(region="r", queue="q")
    S3Source(region="r", bucket="b", every="PT1S")
    with pytest.raises(PydanticValidationError):
        SqsSource.model_validate({"kind": "sqs", "region": "r", "queue": "q", "secret": "x"})
    with pytest.raises(PydanticValidationError):
        Target.model_validate({**TARGET, "mode": "EVENTUALLY"})
    with pytest.raises(PydanticValidationError):
        IngestionDefinition.model_validate(
            {"name": "n", "source": {"kind": "ftp"}, "split": {"kind": "none"}, "target": TARGET}
        )


def test_ingestion_definition_property_round_trips() -> None:
    ingestion = Ingestion.model_validate(INGESTION)
    again = ingestion.definition
    assert again.name == "Orders" and again.target.batch_size == 10
    assert again.model_dump(by_alias=True, exclude_none=True, mode="json")["source"] == HTTP_SOURCE


def test_sync_facade_covers_ingestions() -> None:
    with respx.mock:
        token()
        respx.get(f"{BASE}/api/ingestions/ing_1").respond(200, json=INGESTION)
        respx.put(f"{BASE}/api/ingestion-sets/erp").respond(200, json=PLAN)
        respx.get(f"{BASE}/api/ingestions/ing_1/secrets").respond(200, json={"secrets": []})
        respx.post(f"{BASE}/api/ingestions/ing_1/failures/f1/dismiss").respond(200, json=FAILURE)
        with FlowCatalystSync(BASE, credentials=ClientCredentials("sa", "secret")) as fc:
            assert fc.ingestions.get("ing_1").code == "orders"
            assert fc.ingestion_sets.apply("erp", [], dry_run=True).name == "erp"
            assert fc.ingestions.secrets("ing_1").secrets == []
            assert fc.ingestions.dismiss_failure("ing_1", "f1").category == "rejected"


# --- drafts, notices, secrets ------------------------------------------------------------------

FAILURE: dict[str, Any] = {
    "id": "f1",
    "kind": "JOBS_REJECTED",
    "category": "rejected",
    "message": "refused",
    "afterRecords": 0,
    "occurredAt": "2026-10-25T10:00:00Z",
    "createdAt": "2026-10-25T10:00:01Z",
    "dismissedAt": "2026-10-25T11:00:00Z",
    "dismissedBy": "prn_1",
}


@respx.mock
async def test_failures_send_category_and_undismissed_only_when_given() -> None:
    token()
    route = respx.get(f"{BASE}/api/ingestions/ing_1/failures").respond(
        200, json={"failures": [], "hasMore": False}
    )
    async with client() as fc:
        await fc.ingestions.failures("ing_1")
        await fc.ingestions.failures("ing_1", category="rejected", undismissed=True)
        await fc.ingestions.failures("ing_1", category="payload", undismissed=False)
    assert _req(route, 0).url.query == b""
    assert dict(_req(route, 1).url.params) == {"category": "rejected", "undismissed": "true"}
    assert dict(_req(route, 2).url.params) == {"category": "payload"}


@respx.mock
async def test_dismiss_failure_posts_and_returns_the_failure() -> None:
    token()
    route = respx.post(f"{BASE}/api/ingestions/ing_1/failures/f%2F1/dismiss").respond(
        200, json=FAILURE
    )
    async with client() as fc:
        failure = await fc.ingestions.dismiss_failure("ing_1", "f/1")
    assert route.called and _req(route).content == b""
    assert failure.category == "rejected" and failure.dismissed_by == "prn_1"
    assert failure.dismissed_at is not None and failure.created_at is not None


@respx.mock
async def test_secrets_are_names_and_fetches() -> None:
    token()
    respx.get(f"{BASE}/api/ingestions/ing_1/secrets").respond(
        200,
        json={
            "secrets": [
                {
                    "name": "orders-token",
                    "bound": True,
                    "fetches": [
                        {
                            "runtimeId": "rt-1",
                            "version": 3,
                            "principalId": "prn_rt",
                            "lastFetchedAt": "2026-10-25T10:00:00Z",
                            "fetchCount": 7,
                        }
                    ],
                }
            ]
        },
    )
    async with client() as fc:
        listing = await fc.ingestions.secrets("ing_1")
    secret = listing.secrets[0]
    assert secret.name == "orders-token" and secret.bound
    assert secret.fetches[0].fetch_count == 7 and secret.fetches[0].runtime_id == "rt-1"


@respx.mock
async def test_a_create_without_promote_answers_a_draft() -> None:
    token()
    answer = INGESTION | {
        "notices": 2,
        "latestNotice": "jobs refused",
        "latestNoticeAt": "2026-10-25T10:00:00Z",
        "warnings": ["no host key"],
        "save": {
            "outcome": "DRAFT",
            "version": 1,
            "awaitingPromote": True,
            "message": "saved as a draft",
        },
    }
    respx.post(f"{BASE}/api/ingestions").respond(201, json=answer)
    respx.get(f"{BASE}/api/ingestions/ing_1/versions").respond(
        200, json={"versions": [VERSION | {"draft": True, "digest": "ab12"}]}
    )
    async with client() as fc:
        created = await fc.ingestions.create(
            application="erp", client="acme", code="orders", definition=definition()
        )
        versions = await fc.ingestions.versions("ing_1")
    assert created.save is not None and created.save.outcome == "DRAFT"
    assert created.save.awaiting_promote and created.notices == 2
    assert created.latest_notice == "jobs refused" and created.warnings == ["no host key"]
    assert versions[0].draft and versions[0].digest == "ab12"
    assert Ingestion.model_validate(INGESTION).notices == 0  # an older answer still reads


def test_sftp_accept_any_host_key_round_trips() -> None:
    wire: dict[str, Any] = {
        "kind": "sftp",
        "host": "h",
        "port": 22,
        "directory": "/in",
        "username": "u",
        "credential": {"name": "sftp-key"},
        "acceptAnyHostKey": True,
        "every": "PT5M",
    }
    body = {"name": "N", "source": wire, "split": {"kind": "none"}, "target": TARGET}
    parsed = IngestionDefinition.model_validate(body)
    assert parsed.model_dump(by_alias=True, exclude_none=True, mode="json") == body


def _req(route: respx.Route, index: int = -1) -> httpx.Request:
    request: httpx.Request = route.calls[index].request  # pyright: ignore[reportUnknownMemberType, reportUnknownVariableType]
    return request  # pyright: ignore[reportUnknownVariableType]


def _json(route: respx.Route) -> dict[str, Any]:
    import json

    out: dict[str, Any] = json.loads(_req(route).content)
    return out
