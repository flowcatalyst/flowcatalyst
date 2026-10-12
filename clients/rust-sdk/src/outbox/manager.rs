//! Turning builders into rows, and writing them through a driver.

use std::future::Future;

use super::audit_log::CreateAuditLogDto;
use super::dispatch_job::DispatchJob;
use super::error::OutboxError;
use super::event::CreateEventDto;
use super::message::{MessageId, OutboxMessage};
use super::status::OutboxItemType;
use crate::tsid::Tsid;

/// Writes rows to the outbox table.
///
/// [`insert`](Self::insert) writes all the rows or none. For the
/// transactional-outbox guarantee (rows commit with the business change) use a
/// driver's in-transaction method instead, e.g.
/// [`PgOutboxDriver::insert_in`](super::PgOutboxDriver::insert_in), with rows
/// from the `prepare_*` methods of [`OutboxManager`].
pub trait OutboxDriver: Send + Sync {
    fn insert(
        &self,
        messages: &[OutboxMessage],
    ) -> impl Future<Output = Result<(), OutboxError>> + Send;
}

/// Builds outbox rows for one client and writes them through a driver.
///
/// ```
/// use flowcatalyst_sdk::outbox::{CreateEventDto, OutboxManager, OutboxMessage, OutboxDriver, OutboxError};
///
/// struct Discard;
/// impl OutboxDriver for Discard {
///     async fn insert(&self, _: &[OutboxMessage]) -> Result<(), OutboxError> { Ok(()) }
/// }
///
/// let outbox = OutboxManager::new(Discard, "clt_0HZXEQ5Y8JY5Z")?;
/// let row = outbox.prepare_event(&CreateEventDto::new("orders:sales:order:created", &serde_json::json!({}))?)?;
/// assert_eq!(row.message_type.as_str(), "EVENT");
/// # Ok::<(), OutboxError>(())
/// ```
#[derive(Debug, Clone)]
pub struct OutboxManager<D> {
    driver: D,
    client_id: String,
}

impl<D: OutboxDriver> OutboxManager<D> {
    /// A manager writing rows owned by `client_id` (the `client_id` column;
    /// required, as in every SDK).
    pub fn new(driver: D, client_id: impl Into<String>) -> Result<Self, OutboxError> {
        let client_id = client_id.into();
        if client_id.trim().is_empty() {
            return Err(OutboxError::MissingClientId);
        }
        Ok(Self { driver, client_id })
    }

    pub fn driver(&self) -> &D {
        &self.driver
    }

    pub fn client_id(&self) -> &str {
        &self.client_id
    }

    /// The `EVENT` row for an event (row id: a new TSID).
    pub fn prepare_event(&self, event: &CreateEventDto) -> Result<OutboxMessage, OutboxError> {
        Ok(OutboxMessage::pending(
            MessageId::new(Tsid::generate().to_string()),
            OutboxItemType::Event,
            event.group().map(str::to_owned),
            event.to_payload()?,
            self.client_id.clone(),
            event.headers(),
        ))
    }

    /// The `DISPATCH_JOB` row for a job (row id: the job's id; no headers).
    pub fn prepare_dispatch_job(&self, job: &DispatchJob) -> Result<OutboxMessage, OutboxError> {
        Ok(OutboxMessage::pending(
            MessageId::new(job.id().to_owned()),
            OutboxItemType::DispatchJob,
            job.group().map(str::to_owned),
            job.to_payload()?,
            self.client_id.clone(),
            &Default::default(),
        ))
    }

    /// The `AUDIT_LOG` row for an audit entry (row id: a new TSID; no group).
    pub fn prepare_audit_log(
        &self,
        audit: &CreateAuditLogDto,
    ) -> Result<OutboxMessage, OutboxError> {
        Ok(OutboxMessage::pending(
            MessageId::new(Tsid::generate().to_string()),
            OutboxItemType::AuditLog,
            None,
            audit.to_payload()?,
            self.client_id.clone(),
            audit.headers(),
        ))
    }

    /// Write one event; returns its row id.
    pub async fn create_event(&self, event: &CreateEventDto) -> Result<MessageId, OutboxError> {
        self.write_one(self.prepare_event(event)?).await
    }

    /// Write several events in one insert; returns their row ids in order.
    pub async fn create_events(
        &self,
        events: &[CreateEventDto],
    ) -> Result<Vec<MessageId>, OutboxError> {
        let rows = events
            .iter()
            .map(|e| self.prepare_event(e))
            .collect::<Result<Vec<_>, _>>()?;
        self.write_all(rows).await
    }

    /// Write one dispatch job; returns its row id (the job id).
    pub async fn create_dispatch_job(&self, job: &DispatchJob) -> Result<MessageId, OutboxError> {
        self.write_one(self.prepare_dispatch_job(job)?).await
    }

    /// Write several dispatch jobs in one insert.
    pub async fn create_dispatch_jobs(
        &self,
        jobs: &[DispatchJob],
    ) -> Result<Vec<MessageId>, OutboxError> {
        let rows = jobs
            .iter()
            .map(|j| self.prepare_dispatch_job(j))
            .collect::<Result<Vec<_>, _>>()?;
        self.write_all(rows).await
    }

    /// Write one audit entry; returns its row id.
    pub async fn create_audit_log(
        &self,
        audit: &CreateAuditLogDto,
    ) -> Result<MessageId, OutboxError> {
        self.write_one(self.prepare_audit_log(audit)?).await
    }

    /// Write several audit entries in one insert.
    pub async fn create_audit_logs(
        &self,
        audits: &[CreateAuditLogDto],
    ) -> Result<Vec<MessageId>, OutboxError> {
        let rows = audits
            .iter()
            .map(|a| self.prepare_audit_log(a))
            .collect::<Result<Vec<_>, _>>()?;
        self.write_all(rows).await
    }

    async fn write_one(&self, row: OutboxMessage) -> Result<MessageId, OutboxError> {
        self.driver.insert(std::slice::from_ref(&row)).await?;
        Ok(row.id)
    }

    async fn write_all(&self, rows: Vec<OutboxMessage>) -> Result<Vec<MessageId>, OutboxError> {
        if rows.is_empty() {
            return Ok(Vec::new());
        }
        self.driver.insert(&rows).await?;
        Ok(rows.into_iter().map(|r| r.id).collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::sync::Mutex;

    #[derive(Default)]
    struct Recording(Mutex<Vec<OutboxMessage>>);

    impl OutboxDriver for Recording {
        async fn insert(&self, messages: &[OutboxMessage]) -> Result<(), OutboxError> {
            self.0.lock().unwrap().extend_from_slice(messages);
            Ok(())
        }
    }

    #[test]
    fn a_client_id_is_required() {
        assert!(matches!(
            OutboxManager::new(Recording::default(), " "),
            Err(OutboxError::MissingClientId)
        ));
    }

    #[tokio::test]
    async fn rows_carry_type_group_client_and_byte_size() {
        let m = OutboxManager::new(Recording::default(), "clt_1").unwrap();
        let event = CreateEventDto::new("a:b:c:d", &json!({"name": "é"}))
            .unwrap()
            .message_group("g1")
            .header("x-trace", "t");
        let job = DispatchJob::builder("a:job", "https://h.example/x")
            .id("job_1")
            .message_group("g2")
            .build()
            .unwrap();
        let audit = CreateAuditLogDto::new("A", "1", "Op", "prn_1").unwrap();
        let ids = [
            m.create_event(&event).await.unwrap(),
            m.create_dispatch_job(&job).await.unwrap(),
            m.create_audit_log(&audit).await.unwrap(),
        ];
        let rows = m.driver().0.lock().unwrap().clone();
        assert_eq!(rows.len(), 3);
        assert_eq!(rows[0].message_type, OutboxItemType::Event);
        assert_eq!(rows[0].message_group.as_deref(), Some("g1"));
        assert_eq!(
            rows[0].headers_json().unwrap().as_deref(),
            Some(r#"{"x-trace":"t"}"#)
        );
        assert_eq!(rows[0].payload_size as usize, rows[0].payload.len());
        assert!(
            rows[0].payload.len() > rows[0].payload.chars().count(),
            "UTF-8 bytes, not chars"
        );
        assert_eq!(rows[1].id.as_str(), "job_1");
        assert_eq!(rows[1].message_type, OutboxItemType::DispatchJob);
        assert_eq!(rows[1].headers, None);
        assert_eq!(rows[2].message_type, OutboxItemType::AuditLog);
        assert_eq!(rows[2].message_group, None);
        for (row, id) in rows.iter().zip(&ids) {
            assert_eq!(&row.id, id);
            assert_eq!(row.client_id, "clt_1");
            assert_eq!(row.status_code(), 0);
        }
    }
}
