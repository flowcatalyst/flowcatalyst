//! Client Entity
//!
//! Represents a tenant/organization in the multi-tenant system.

use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};
use bson::serde_helpers::chrono_datetime_as_bson_datetime;

/// Client status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ClientStatus {
    /// Client is active and operational
    Active,
    /// Client is suspended (e.g., billing issue)
    Suspended,
    /// Client is pending activation
    Pending,
    /// Client is deleted (soft delete)
    Deleted,
}

impl Default for ClientStatus {
    fn default() -> Self {
        Self::Active
    }
}

/// Client note for audit trail
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClientNote {
    /// Note category
    pub category: String,

    /// Note text
    pub text: String,

    /// Who added the note
    #[serde(skip_serializing_if = "Option::is_none")]
    pub added_by: Option<String>,

    /// When the note was added
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub added_at: DateTime<Utc>,
}

impl ClientNote {
    pub fn new(category: impl Into<String>, text: impl Into<String>) -> Self {
        Self {
            category: category.into(),
            text: text.into(),
            added_by: None,
            added_at: Utc::now(),
        }
    }

    pub fn with_author(mut self, author: impl Into<String>) -> Self {
        self.added_by = Some(author.into());
        self
    }
}

/// Client entity - represents a tenant/organization
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Client {
    /// TSID as Crockford Base32 string
    #[serde(rename = "_id")]
    pub id: String,

    /// Human-readable name
    pub name: String,

    /// Unique identifier/slug (URL-safe)
    pub identifier: String,

    /// Description
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,

    /// Current status
    #[serde(default)]
    pub status: ClientStatus,

    /// Reason for current status
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status_reason: Option<String>,

    /// When status was last changed
    #[serde(skip_serializing_if = "Option::is_none", default, with = "bson::serde_helpers::chrono_datetime_as_bson_datetime_optional")]
    pub status_changed_at: Option<DateTime<Utc>>,

    /// Audit notes
    #[serde(default)]
    pub notes: Vec<ClientNote>,

    /// Custom metadata
    #[serde(default)]
    pub metadata: serde_json::Value,

    /// Audit fields
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub created_at: DateTime<Utc>,
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub updated_at: DateTime<Utc>,

    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_by: Option<String>,
}

impl Client {
    pub fn new(name: impl Into<String>, identifier: impl Into<String>) -> Self {
        let now = Utc::now();
        Self {
            id: crate::TsidGenerator::generate(),
            name: name.into(),
            identifier: identifier.into(),
            description: None,
            status: ClientStatus::Active,
            status_reason: None,
            status_changed_at: None,
            notes: vec![],
            metadata: serde_json::Value::Null,
            created_at: now,
            updated_at: now,
            created_by: None,
        }
    }

    pub fn with_description(mut self, description: impl Into<String>) -> Self {
        self.description = Some(description.into());
        self
    }

    pub fn add_note(&mut self, note: ClientNote) {
        self.notes.push(note);
        self.updated_at = Utc::now();
    }

    pub fn set_status(&mut self, status: ClientStatus, reason: Option<String>) {
        self.status = status;
        self.status_reason = reason;
        self.status_changed_at = Some(Utc::now());
        self.updated_at = Utc::now();
    }

    pub fn suspend(&mut self, reason: impl Into<String>) {
        self.set_status(ClientStatus::Suspended, Some(reason.into()));
    }

    pub fn activate(&mut self) {
        self.set_status(ClientStatus::Active, None);
    }

    pub fn delete(&mut self, reason: Option<String>) {
        self.set_status(ClientStatus::Deleted, reason);
    }

    pub fn is_active(&self) -> bool {
        self.status == ClientStatus::Active
    }

    pub fn is_suspended(&self) -> bool {
        self.status == ClientStatus::Suspended
    }
}
