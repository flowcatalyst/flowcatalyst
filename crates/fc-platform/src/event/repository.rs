//! Event Repository — PostgreSQL via SQLx
//!
//! Direct SQL queries with explicit control over what's fetched.

use sqlx::PgPool;
use chrono::{DateTime, Utc};

use super::entity::{Event, EventRead, ContextData, CLOUDEVENTS_SPEC_VERSION};
use crate::shared::error::Result;

/// Row mapping for msg_events table
#[derive(sqlx::FromRow)]
struct EventRow {
    id: String,
    spec_version: Option<String>,
    #[sqlx(rename = "type")]
    event_type: String,
    source: String,
    subject: Option<String>,
    time: DateTime<Utc>,
    data: Option<serde_json::Value>,
    correlation_id: Option<String>,
    causation_id: Option<String>,
    deduplication_id: Option<String>,
    message_group: Option<String>,
    client_id: Option<String>,
    context_data: Option<serde_json::Value>,
    created_at: DateTime<Utc>,
}

impl From<EventRow> for Event {
    fn from(r: EventRow) -> Self {
        let context_data: Vec<ContextData> = r.context_data
            .and_then(|v| serde_json::from_value(v).ok())
            .unwrap_or_default();

        Self {
            id: r.id,
            event_type: r.event_type,
            source: r.source,
            subject: r.subject,
            time: r.time,
            data: r.data.unwrap_or(serde_json::Value::Null),
            spec_version: r.spec_version.unwrap_or_else(|| CLOUDEVENTS_SPEC_VERSION.to_string()),
            message_group: r.message_group,
            correlation_id: r.correlation_id,
            causation_id: r.causation_id,
            deduplication_id: r.deduplication_id,
            client_id: r.client_id,
            context_data,
            created_at: r.created_at,
        }
    }
}

/// Row mapping for msg_events_read table
#[derive(sqlx::FromRow)]
struct EventReadRow {
    id: String,
    #[sqlx(rename = "type")]
    event_type: String,
    source: String,
    subject: Option<String>,
    time: DateTime<Utc>,
    application: Option<String>,
    subdomain: Option<String>,
    aggregate: Option<String>,
    message_group: Option<String>,
    correlation_id: Option<String>,
    client_id: Option<String>,
    projected_at: DateTime<Utc>,
}

impl From<EventReadRow> for EventRead {
    fn from(r: EventReadRow) -> Self {
        Self {
            id: r.id,
            event_type: r.event_type,
            source: r.source,
            subject: r.subject,
            time: r.time,
            application: r.application,
            subdomain: r.subdomain,
            aggregate: r.aggregate,
            message_group: r.message_group,
            correlation_id: r.correlation_id,
            client_id: r.client_id,
            client_name: None,
            projected_at: r.projected_at,
        }
    }
}

pub struct EventRepository {
    pool: PgPool,
}

impl EventRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn insert(&self, event: &Event) -> Result<()> {
        let context_json = if event.context_data.is_empty() {
            None
        } else {
            serde_json::to_value(&event.context_data).ok()
        };

        sqlx::query(
            r#"INSERT INTO msg_events
                (id, spec_version, type, source, subject, time, data,
                 correlation_id, causation_id, deduplication_id,
                 message_group, client_id, context_data, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())"#
        )
        .bind(&event.id)
        .bind(&event.spec_version)
        .bind(&event.event_type)
        .bind(&event.source)
        .bind(&event.subject)
        .bind(event.time)
        .bind(&event.data)
        .bind(&event.correlation_id)
        .bind(&event.causation_id)
        .bind(&event.deduplication_id)
        .bind(&event.message_group)
        .bind(&event.client_id)
        .bind(&context_json)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    /// Batch insert events in a single query using UNNEST arrays.
    pub async fn insert_many(&self, events: &[Event]) -> Result<()> {
        if events.is_empty() {
            return Ok(());
        }

        let mut ids = Vec::with_capacity(events.len());
        let mut spec_versions = Vec::with_capacity(events.len());
        let mut types = Vec::with_capacity(events.len());
        let mut sources = Vec::with_capacity(events.len());
        let mut subjects: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut times = Vec::with_capacity(events.len());
        let mut datas: Vec<serde_json::Value> = Vec::with_capacity(events.len());
        let mut correlation_ids: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut causation_ids: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut deduplication_ids: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut message_groups: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut client_ids: Vec<Option<String>> = Vec::with_capacity(events.len());
        let mut context_datas: Vec<Option<serde_json::Value>> = Vec::with_capacity(events.len());

        for event in events {
            ids.push(event.id.as_str());
            spec_versions.push(event.spec_version.as_str());
            types.push(event.event_type.as_str());
            sources.push(event.source.as_str());
            subjects.push(event.subject.clone());
            times.push(event.time);
            datas.push(event.data.clone());
            correlation_ids.push(event.correlation_id.clone());
            causation_ids.push(event.causation_id.clone());
            deduplication_ids.push(event.deduplication_id.clone());
            message_groups.push(event.message_group.clone());
            client_ids.push(event.client_id.clone());
            context_datas.push(
                if event.context_data.is_empty() {
                    None
                } else {
                    serde_json::to_value(&event.context_data).ok()
                }
            );
        }

        sqlx::query(
            r#"INSERT INTO msg_events
                (id, spec_version, type, source, subject, time, data,
                 correlation_id, causation_id, deduplication_id,
                 message_group, client_id, context_data, created_at)
            SELECT * FROM UNNEST(
                $1::varchar[], $2::varchar[], $3::varchar[], $4::varchar[],
                $5::varchar[], $6::timestamptz[], $7::jsonb[],
                $8::varchar[], $9::varchar[], $10::varchar[],
                $11::varchar[], $12::varchar[], $13::jsonb[]
            ), NOW()"#
        )
        .bind(&ids)
        .bind(&spec_versions)
        .bind(&types)
        .bind(&sources)
        .bind(&subjects as &[Option<String>])
        .bind(&times)
        .bind(&datas)
        .bind(&correlation_ids as &[Option<String>])
        .bind(&causation_ids as &[Option<String>])
        .bind(&deduplication_ids as &[Option<String>])
        .bind(&message_groups as &[Option<String>])
        .bind(&client_ids as &[Option<String>])
        .bind(&context_datas as &[Option<serde_json::Value>])
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<Event>> {
        let row = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(Event::from))
    }

    pub async fn find_by_type(&self, event_type: &str, limit: u64) -> Result<Vec<Event>> {
        let rows = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events WHERE type = $1 ORDER BY time DESC LIMIT $2"
        )
        .bind(event_type)
        .bind(limit as i64)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(Event::from).collect())
    }

    pub async fn find_by_client(&self, client_id: &str, limit: u64) -> Result<Vec<Event>> {
        let rows = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events WHERE client_id = $1 ORDER BY time DESC LIMIT $2"
        )
        .bind(client_id)
        .bind(limit as i64)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(Event::from).collect())
    }

    pub async fn find_by_correlation_id(&self, correlation_id: &str) -> Result<Vec<Event>> {
        let rows = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events WHERE correlation_id = $1 ORDER BY time DESC"
        )
        .bind(correlation_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(Event::from).collect())
    }

    pub async fn find_by_deduplication_id(&self, deduplication_id: &str) -> Result<Option<Event>> {
        let row = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events WHERE deduplication_id = $1"
        )
        .bind(deduplication_id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(Event::from))
    }

    pub async fn find_recent_paged(&self, page: u64, size: u64) -> Result<Vec<Event>> {
        let rows = sqlx::query_as::<_, EventRow>(
            "SELECT * FROM msg_events ORDER BY created_at DESC LIMIT $1 OFFSET $2"
        )
        .bind(size as i64)
        .bind((page * size) as i64)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(Event::from).collect())
    }

    pub async fn count_all(&self) -> Result<u64> {
        let row: (i64,) = sqlx::query_as(
            "SELECT COUNT(*) FROM msg_events"
        )
        .fetch_one(&self.pool)
        .await?;

        Ok(row.0 as u64)
    }

    // ── Read projection methods ──────────────────────────────────────────

    pub async fn find_read_by_id(&self, id: &str) -> Result<Option<EventRead>> {
        let row = sqlx::query_as::<_, EventReadRow>(
            "SELECT id, type, source, subject, time, application, subdomain, \
             aggregate, message_group, correlation_id, client_id, projected_at \
             FROM msg_events_read WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(EventRead::from))
    }

    pub async fn insert_read_projection(&self, p: &EventRead) -> Result<()> {
        sqlx::query(
            r#"INSERT INTO msg_events_read
                (id, type, source, subject, time, application, subdomain,
                 aggregate, message_group, correlation_id, client_id, projected_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())"#
        )
        .bind(&p.id)
        .bind(&p.event_type)
        .bind(&p.source)
        .bind(&p.subject)
        .bind(p.time)
        .bind(&p.application)
        .bind(&p.subdomain)
        .bind(&p.aggregate)
        .bind(&p.message_group)
        .bind(&p.correlation_id)
        .bind(&p.client_id)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    pub async fn update_read_projection(&self, p: &EventRead) -> Result<()> {
        sqlx::query(
            r#"UPDATE msg_events_read SET
                type = $2, source = $3, subject = $4, time = $5,
                application = $6, subdomain = $7, aggregate = $8,
                message_group = $9, correlation_id = $10, client_id = $11,
                projected_at = NOW()
            WHERE id = $1"#
        )
        .bind(&p.id)
        .bind(&p.event_type)
        .bind(&p.source)
        .bind(&p.subject)
        .bind(p.time)
        .bind(&p.application)
        .bind(&p.subdomain)
        .bind(&p.aggregate)
        .bind(&p.message_group)
        .bind(&p.correlation_id)
        .bind(&p.client_id)
        .execute(&self.pool)
        .await?;

        Ok(())
    }
}
