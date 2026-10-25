"""Ingestion models.

An ingestion's ``source``, ``split`` and the unions inside a source are tagged unions on
``kind`` (``type`` for a database poll's ``track``), typed per variant. A runtime-extension
kind is :class:`CustomSource` (``kind="custom"``, the extension's name in ``type``, its
``settings`` an open mapping). Secrets are **names** (:class:`SecretRef`), never values; nothing
in this SDK handles a secret value for an ingestion. Durations are ISO 8601 strings
(``"PT30S"``). The definition parts forbid unknown fields, as the platform does.
"""

from __future__ import annotations

from datetime import datetime
from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel

__all__ = [
    "ApiBasicAuth",
    "ApiBearerAuth",
    "ApiKeyAuth",
    "ApiPollSource",
    "ApplyIngestionSetItem",
    "AwsCredential",
    "BasicAuth",
    "BearerAuth",
    "CsvSplit",
    "Cursor",
    "CustomSource",
    "DatabasePollSource",
    "FlowCatalystTokenAuth",
    "Header",
    "HeaderResults",
    "HttpSource",
    "Ingestion",
    "IngestionDefinition",
    "IngestionFailure",
    "IngestionFailurePage",
    "IngestionHistoryEntry",
    "IngestionHistoryPage",
    "IngestionPage",
    "IngestionSaveOutcome",
    "IngestionSecret",
    "IngestionSecretFetch",
    "IngestionSecretList",
    "IngestionSet",
    "IngestionSetApply",
    "IngestionSetArchive",
    "IngestionSetChange",
    "IngestionSetPlan",
    "IngestionSetRelease",
    "IngestionSourceKind",
    "IngestionTest",
    "IngestionVersion",
    "JsonPathResults",
    "JsonSplit",
    "LoginAuth",
    "NoAuth",
    "NoSplit",
    "OAuthClientCredentialsAuth",
    "RabbitMqSource",
    "RateLimit",
    "RecordResults",
    "S3Source",
    "SecretRef",
    "SetFile",
    "SftpSource",
    "Source",
    "Split",
    "SqsSource",
    "StatusTrack",
    "Target",
    "WatermarkTrack",
    "XmlSplit",
]

_DEFINITION = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="forbid")
_RESPONSE = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="ignore")

#: How a target hands each record on; ``IMMEDIATE`` | ``NEXT_ON_ERROR`` | ``BLOCK_ON_ERROR``.
TargetMode = Literal["IMMEDIATE", "NEXT_ON_ERROR", "BLOCK_ON_ERROR"]


class _Part(BaseModel):
    model_config = _DEFINITION


class SecretRef(_Part):
    """The name of a secret, resolved by the runtime against the ingestion's own client."""

    name: str


class RateLimit(_Part):
    requests: int
    per: str


class AwsCredential(_Part):
    access_key_id: str
    secret_access_key: SecretRef


class Header(_Part):
    """A header sent on every api-poll request; none of them is secret."""

    name: str
    value: str


class Cursor(_Part):
    from_: str = Field(alias="from")
    param: str
    initial: str | None = None


# --- http source authentication -------------------------------------------------------------


class FlowCatalystTokenAuth(_Part):
    kind: Literal["flowcatalyst-token"] = "flowcatalyst-token"
    application: str
    principal_id: str | None = None


class BasicAuth(_Part):
    kind: Literal["basic"] = "basic"
    username: str
    password: SecretRef


class BearerAuth(_Part):
    kind: Literal["bearer"] = "bearer"
    token: SecretRef


HttpAuth = Annotated[FlowCatalystTokenAuth | BasicAuth | BearerAuth, Field(discriminator="kind")]


# --- api-poll authentication, results ---------------------------------------------------------


class NoAuth(_Part):
    kind: Literal["none"] = "none"


class ApiBasicAuth(_Part):
    kind: Literal["basic"] = "basic"
    username: str
    password: SecretRef


class ApiBearerAuth(_Part):
    kind: Literal["bearer"] = "bearer"
    token: SecretRef


class ApiKeyAuth(_Part):
    kind: Literal["api-key"] = "api-key"
    header: str
    value: SecretRef


class OAuthClientCredentialsAuth(_Part):
    kind: Literal["oauth-client-credentials"] = "oauth-client-credentials"
    token_url: str
    client_id: str
    client_secret: SecretRef
    scope: str | None = None
    audience: str | None = None
    credentials_in: Literal["BASIC_HEADER", "BODY"]


class LoginAuth(_Part):
    kind: Literal["login"] = "login"
    url: str
    format: Literal["FORM", "JSON"]
    username_field: str
    password_field: str
    username: str
    password: SecretRef
    extra_fields: dict[str, str] | None = None
    token_path: str
    expires_in_path: str | None = None
    lifetime: str
    header: str
    prefix: str


ApiAuth = Annotated[
    NoAuth | ApiBasicAuth | ApiBearerAuth | ApiKeyAuth | OAuthClientCredentialsAuth | LoginAuth,
    Field(discriminator="kind"),
]


class RecordResults(_Part):
    kind: Literal["records"] = "records"


class JsonPathResults(_Part):
    kind: Literal["json-path"] = "json-path"
    path: str


class HeaderResults(_Part):
    kind: Literal["header"] = "header"
    name: str
    equals: str | None = None


Results = Annotated[RecordResults | JsonPathResults | HeaderResults, Field(discriminator="kind")]


# --- database-poll tracking --------------------------------------------------------------------


class WatermarkTrack(_Part):
    kind: Literal["watermark"] = "watermark"
    column: str
    type: Literal["NUMBER", "TIMESTAMP"]


class StatusTrack(_Part):
    kind: Literal["status"] = "status"
    column: str
    key_column: str
    pending: str
    processed: str


Track = Annotated[WatermarkTrack | StatusTrack, Field(discriminator="kind")]


# --- sources -----------------------------------------------------------------------------------


class HttpSource(_Part):
    kind: Literal["http"] = "http"
    auth: HttpAuth
    rate_limit: RateLimit | None = None


class SftpSource(_Part):
    kind: Literal["sftp"] = "sftp"
    host: str
    port: int
    directory: str
    username: str
    credential: SecretRef
    #: The server's host key as a known_hosts line. Without it the runtime refuses to start the
    #: source unless ``accept_any_host_key`` is set.
    known_hosts: str | None = None
    #: Connect without checking who the server is (whoever answers receives the credential);
    #: turning it on needs the bind-secrets permission.
    accept_any_host_key: bool | None = None
    include: str | None = None
    every: str


class S3Source(_Part):
    kind: Literal["s3"] = "s3"
    region: str
    bucket: str
    prefix: str | None = None
    every: str
    credential: AwsCredential | None = None


class SqsSource(_Part):
    kind: Literal["sqs"] = "sqs"
    region: str
    queue: str
    credential: AwsCredential | None = None


class RabbitMqSource(_Part):
    kind: Literal["rabbitmq"] = "rabbitmq"
    uri: str
    queue: str
    username: str
    password: SecretRef


class DatabasePollSource(_Part):
    kind: Literal["database-poll"] = "database-poll"
    jdbc_url: str
    username: str
    password: SecretRef
    table: str
    track: Track
    max_rows: int
    every: str


class ApiPollSource(_Part):
    kind: Literal["api-poll"] = "api-poll"
    url: str
    method: str
    headers: list[Header] | None = None
    body: str | None = None
    auth: ApiAuth
    results: Results
    cursor: Cursor | None = None
    every: str
    idle_delay: str


class CustomSource(_Part):
    """A kind a runtime extension supplies (``camel-route`` included): ``type`` names it and
    ``settings`` is whatever it takes. A secret setting is the secret's name."""

    kind: Literal["custom"] = "custom"
    type: str
    settings: dict[str, Any] | None = None


Source = Annotated[
    HttpSource
    | SftpSource
    | S3Source
    | SqsSource
    | RabbitMqSource
    | DatabasePollSource
    | ApiPollSource
    | CustomSource,
    Field(discriminator="kind"),
]


# --- splits -------------------------------------------------------------------------------------


class NoSplit(_Part):
    kind: Literal["none"] = "none"


class CsvSplit(_Part):
    kind: Literal["csv"] = "csv"
    header: bool


class JsonSplit(_Part):
    kind: Literal["json"] = "json"
    array_path: str


class XmlSplit(_Part):
    kind: Literal["xml"] = "xml"
    element: str


Split = Annotated[NoSplit | CsvSplit | JsonSplit | XmlSplit, Field(discriminator="kind")]


class Target(_Part):
    """The dispatch job each record becomes. ``pool`` is a pool code; ``connection`` and
    ``service_account`` are optional codes and may not both be set; neither set is unsigned
    delivery."""

    code: str
    target_url: str
    pool: str
    connection: str | None = None
    service_account: str | None = None
    mode: TargetMode
    batch_size: int


class IngestionDefinition(_Part):
    """What an ingestion does, without who owns it: the body of a new version."""

    name: str
    source: Source
    split: Split
    target: Target
    keep_failed_payloads: bool | None = None


# --- responses ----------------------------------------------------------------------------------

#: What a create, update or version save did.
SaveOutcome = Literal["UNCHANGED", "SAVED", "MADE_LIVE", "DRAFT", "DRAFT_EXISTS"]

#: ``payload``: a record or payload the runtime could not take. ``rejected``: the platform
#: refused the runtime's jobs permanently, shown on the ingestion as a notice until dismissed.
FailureCategory = Literal["payload", "rejected"]


class IngestionSaveOutcome(BaseModel):
    """What a create, update or version save did. ``UNCHANGED``: the live version already says
    it, nothing written. ``SAVED``: saved as a new version and made live. ``MADE_LIVE``: a saved
    version that was not live already says it and was made live. ``DRAFT``: saved as a new
    version that is not live (no promote permission, or not asked to be live). ``DRAFT_EXISTS``:
    a saved draft already says it, nothing saved. ``version`` is the live version or the draft;
    ``awaiting_promote`` is true while someone who holds promote must make it live."""

    model_config = _RESPONSE

    outcome: SaveOutcome | str
    version: int
    awaiting_promote: bool
    message: str = ""


class Ingestion(BaseModel):
    model_config = _RESPONSE

    id: str
    application_id: str
    application_code: str
    client_id: str
    client_identifier: str
    code: str
    status: Literal["ACTIVE", "PAUSED"] | str
    version: int
    latest_version: int
    created_at: datetime
    updated_at: datetime
    name: str
    source: Source
    split: Split
    target: Target
    keep_failed_payloads: bool | None = None
    managed_by: str | None = None
    runtime_notice: str | None = None
    #: Undismissed notices: failures the platform refused (category ``rejected``).
    notices: int = 0
    latest_notice: str | None = None
    latest_notice_at: datetime | None = None
    #: What the definition says that will not run as meant.
    warnings: list[str] | None = None
    #: What this create or update did (set on those answers only).
    save: IngestionSaveOutcome | None = None

    @property
    def definition(self) -> IngestionDefinition:
        """The live definition, ready to edit and send back through ``update``."""
        return IngestionDefinition(
            name=self.name,
            source=self.source,
            split=self.split,
            target=self.target,
            keep_failed_payloads=self.keep_failed_payloads,
        )


class IngestionPage(BaseModel):
    """One page of ingestions; pass ``next_cursor`` as ``after`` for the next."""

    model_config = _RESPONSE

    ingestions: list[Ingestion]
    has_more: bool
    next_cursor: str | None = None


class IngestionVersion(BaseModel):
    model_config = _RESPONSE

    id: str
    version: int
    live: bool
    #: Saved but not live: waits for someone who holds promote to make it live.
    draft: bool = False
    created_at: datetime
    name: str
    source: Source
    split: Split
    target: Target
    keep_failed_payloads: bool | None = None
    note: str | None = None
    created_by: str | None = None
    applied_from: str | None = None
    #: sha256 (hex) of the definition's canonical form; equal definitions have equal digests.
    digest: str | None = None
    #: What this save did (the answer of ``save_version`` only).
    save: IngestionSaveOutcome | None = None


class IngestionFailure(BaseModel):
    model_config = _RESPONSE

    id: str
    kind: str
    category: FailureCategory | str = "payload"
    message: str
    after_records: int
    occurred_at: datetime
    created_at: datetime | None = None
    excerpt: str | None = None
    runtime_id: str | None = None
    version: int | None = None
    dismissed_at: datetime | None = None
    #: The principal who dismissed it.
    dismissed_by: str | None = None


class IngestionFailurePage(BaseModel):
    model_config = _RESPONSE

    failures: list[IngestionFailure]
    has_more: bool
    next_cursor: str | None = None


class IngestionSecretFetch(BaseModel):
    """When a runtime last fetched a bound secret."""

    model_config = _RESPONSE

    runtime_id: str
    version: int
    last_fetched_at: datetime
    fetch_count: int
    principal_id: str | None = None


class IngestionSecret(BaseModel):
    """A secret *name* the ingestion binds (``bound``) or bound under an earlier live version;
    never its value. ``fetches`` is per runtime, newest first."""

    model_config = _RESPONSE

    name: str
    bound: bool
    fetches: list[IngestionSecretFetch]


class IngestionSecretList(BaseModel):
    model_config = _RESPONSE

    secrets: list[IngestionSecret]


class IngestionHistoryEntry(BaseModel):
    model_config = _RESPONSE

    id: str
    operation: str
    performed_at: datetime
    details: Any = None
    principal_id: str | None = None
    principal_name: str | None = None


class IngestionHistoryPage(BaseModel):
    model_config = _RESPONSE

    entries: list[IngestionHistoryEntry]
    has_more: bool


class IngestionSourceKind(BaseModel):
    model_config = _RESPONSE

    kind: str
    built_in: bool
    single_owner: bool
    description: str | None = None
    required_permission: str | None = None
    settings: Any = None


class IngestionTest(BaseModel):
    """A connection test. Poll :meth:`IngestionsResource.get_test` until ``status`` is
    ``DONE`` or ``EXPIRED``."""

    model_config = _RESPONSE

    id: str
    kind: str
    status: Literal["PENDING", "RUNNING", "DONE", "EXPIRED"] | str
    created_at: datetime
    expires_at: datetime
    completed_at: datetime | None = None
    outcome: Literal["RESULTS", "NO_RESULTS", "REFUSED", "FAILED", "NOT_SUPPORTED"] | str | None = (
        None
    )
    excerpt: str | None = None
    ingestion_id: str | None = None
    notice: str | None = None
    records: int | None = None
    runtime_id: str | None = None
    steps: list[str] | None = None


# --- sets ---------------------------------------------------------------------------------------


class SetFile(_Part):
    """A file an item was read from, kept with the apply."""

    path: str
    content: str


class ApplyIngestionSetItem(_Part):
    """One ingestion of a set: a definition plus who owns it."""

    application: str
    client: str
    code: str
    name: str
    source: Source
    split: Split
    target: Target
    active: bool | None = None
    keep_failed_payloads: bool | None = None


class IngestionSet(BaseModel):
    model_config = _RESPONSE

    id: str
    name: str
    ingestions: int
    created_at: datetime
    updated_at: datetime


class IngestionSetChange(BaseModel):
    model_config = _RESPONSE

    application: str
    client: str
    code: str
    action: Literal["CREATE", "CHANGE", "ADOPT", "UNCHANGED", "REMOVE"] | str
    changed: list[str]
    ingestion_id: str | None = None


class IngestionSetPlan(BaseModel):
    """The outcome of an apply; ``apply_id`` is absent for a dry run and for an apply that
    changed nothing."""

    model_config = _RESPONSE

    name: str
    dry_run: bool
    changes: list[IngestionSetChange]
    apply_id: str | None = None


class IngestionSetApply(BaseModel):
    model_config = _RESPONSE

    id: str
    set: str
    applied_at: datetime
    items: int
    files: int
    changes: list[IngestionSetChange]
    applied_by: str | None = None
    going_back_to: str | None = None


class IngestionSetArchive(BaseModel):
    model_config = _RESPONSE

    set: str
    apply_id: str
    applied_at: datetime
    applied_by: str
    items: list[ApplyIngestionSetItem]
    files: list[SetFile]
    going_back_to: str | None = None


class IngestionSetRelease(BaseModel):
    model_config = _RESPONSE

    name: str
    released: int
