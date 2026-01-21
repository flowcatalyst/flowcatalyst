//! Principal Entity
//!
//! Unified model for users and service accounts.
//! Multi-tenant with UserScope determining client access.

use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};
use bson::serde_helpers::chrono_datetime_as_bson_datetime;
use crate::service_account::entity::RoleAssignment;

/// Principal type
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum PrincipalType {
    /// Human user
    User,
    /// Machine service account
    Service,
}

impl Default for PrincipalType {
    fn default() -> Self {
        Self::User
    }
}

/// User scope determines client access level
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum UserScope {
    /// Platform admin - access to all clients
    Anchor,
    /// Partner user - access to multiple assigned clients
    Partner,
    /// Client user - access to single home client
    Client,
}

impl Default for UserScope {
    fn default() -> Self {
        Self::Client
    }
}

impl UserScope {
    /// Check if this scope has access to all clients
    pub fn is_anchor(&self) -> bool {
        matches!(self, Self::Anchor)
    }

    /// Check if this scope can access a specific client
    pub fn can_access_client(&self, client_id: &str, home_client_id: Option<&str>, assigned_clients: &[String]) -> bool {
        match self {
            Self::Anchor => true,
            Self::Partner => assigned_clients.contains(&client_id.to_string()),
            Self::Client => home_client_id == Some(client_id),
        }
    }
}

/// User identity for human users
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UserIdentity {
    /// Email address (unique)
    pub email: String,

    /// Email verified
    #[serde(default)]
    pub email_verified: bool,

    /// First name
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_name: Option<String>,

    /// Last name
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_name: Option<String>,

    /// Profile picture URL
    #[serde(skip_serializing_if = "Option::is_none")]
    pub picture_url: Option<String>,

    /// Phone number
    #[serde(skip_serializing_if = "Option::is_none")]
    pub phone: Option<String>,

    /// External IDP subject ID (for federated auth)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub external_id: Option<String>,

    /// IDP provider name
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,

    /// Password hash (for embedded auth)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub password_hash: Option<String>,

    /// Last login time
    #[serde(skip_serializing_if = "Option::is_none", default, with = "bson::serde_helpers::chrono_datetime_as_bson_datetime_optional")]
    pub last_login_at: Option<DateTime<Utc>>,
}

impl UserIdentity {
    pub fn new(email: impl Into<String>) -> Self {
        Self {
            email: email.into(),
            email_verified: false,
            first_name: None,
            last_name: None,
            picture_url: None,
            phone: None,
            external_id: None,
            provider: None,
            password_hash: None,
            last_login_at: None,
        }
    }

    pub fn with_name(mut self, first_name: impl Into<String>, last_name: impl Into<String>) -> Self {
        self.first_name = Some(first_name.into());
        self.last_name = Some(last_name.into());
        self
    }

    pub fn display_name(&self) -> String {
        match (&self.first_name, &self.last_name) {
            (Some(first), Some(last)) => format!("{} {}", first, last),
            (Some(first), None) => first.clone(),
            (None, Some(last)) => last.clone(),
            (None, None) => self.email.clone(),
        }
    }
}

/// Principal entity - unified user/service account
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Principal {
    /// TSID as Crockford Base32 string
    #[serde(rename = "_id")]
    pub id: String,

    /// Principal type (user or service)
    #[serde(rename = "type")]
    #[serde(default)]
    pub principal_type: PrincipalType,

    /// User scope (for users only)
    #[serde(default)]
    pub scope: UserScope,

    /// Home client ID (for CLIENT scope users)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_id: Option<String>,

    /// Application ID (for service accounts created by an app)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub application_id: Option<String>,

    /// Display name
    pub name: String,

    /// Whether the principal is active
    #[serde(default = "default_active")]
    pub active: bool,

    /// User identity (for USER type)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub user_identity: Option<UserIdentity>,

    /// Service account ID reference (for SERVICE type)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub service_account_id: Option<String>,

    /// Assigned roles (denormalized)
    #[serde(default)]
    pub roles: Vec<RoleAssignment>,

    /// Assigned client IDs (for PARTNER scope)
    #[serde(default)]
    pub assigned_clients: Vec<String>,

    /// Audit fields
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub created_at: DateTime<Utc>,
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub updated_at: DateTime<Utc>,

    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_by: Option<String>,

    /// External identity for OIDC-authenticated users
    #[serde(skip_serializing_if = "Option::is_none")]
    pub external_identity: Option<ExternalIdentity>,
}

/// External identity reference for OIDC-authenticated users
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExternalIdentity {
    /// OIDC provider ID
    pub provider_id: String,
    /// Subject ID from the external IDP
    pub external_id: String,
}

fn default_active() -> bool {
    true
}

impl Principal {
    /// Create a new user principal
    pub fn new_user(email: impl Into<String>, scope: UserScope) -> Self {
        let email = email.into();
        let identity = UserIdentity::new(&email);
        let now = Utc::now();

        Self {
            id: crate::TsidGenerator::generate(),
            principal_type: PrincipalType::User,
            scope,
            client_id: None,
            application_id: None,
            name: identity.display_name(),
            active: true,
            user_identity: Some(identity),
            service_account_id: None,
            roles: vec![],
            assigned_clients: vec![],
            created_at: now,
            updated_at: now,
            created_by: None,
            external_identity: None,
        }
    }

    /// Create a new service principal
    pub fn new_service(service_account_id: impl Into<String>, name: impl Into<String>) -> Self {
        let now = Utc::now();
        Self {
            id: crate::TsidGenerator::generate(),
            principal_type: PrincipalType::Service,
            scope: UserScope::Anchor, // Service accounts typically have anchor scope
            client_id: None,
            application_id: None,
            name: name.into(),
            active: true,
            user_identity: None,
            service_account_id: Some(service_account_id.into()),
            roles: vec![],
            assigned_clients: vec![],
            created_at: now,
            updated_at: now,
            created_by: None,
            external_identity: None,
        }
    }

    pub fn with_client_id(mut self, client_id: impl Into<String>) -> Self {
        self.client_id = Some(client_id.into());
        self
    }

    pub fn with_application_id(mut self, application_id: impl Into<String>) -> Self {
        self.application_id = Some(application_id.into());
        self
    }

    pub fn assign_role(&mut self, role: impl Into<String>) {
        self.roles.push(RoleAssignment::new(role));
        self.updated_at = Utc::now();
    }

    pub fn assign_role_with_source(&mut self, role: impl Into<String>, source: impl Into<String>) {
        self.roles.push(RoleAssignment::with_source(role, source));
        self.updated_at = Utc::now();
    }

    pub fn assign_role_for_client(&mut self, role: impl Into<String>, client_id: impl Into<String>) {
        self.roles.push(RoleAssignment::for_client(role, client_id));
        self.updated_at = Utc::now();
    }

    /// Remove all roles from a specific source (e.g., "IDP_SYNC")
    /// Returns the number of roles removed
    pub fn remove_roles_by_source(&mut self, source: &str) -> usize {
        let original_count = self.roles.len();
        self.roles.retain(|r| r.assignment_source.as_deref() != Some(source));
        let removed = original_count - self.roles.len();
        if removed > 0 {
            self.updated_at = Utc::now();
        }
        removed
    }

    /// Update last login timestamp
    pub fn update_last_login(&mut self) {
        if let Some(ref mut identity) = self.user_identity {
            identity.last_login_at = Some(Utc::now());
        }
        self.updated_at = Utc::now();
    }

    pub fn grant_client_access(&mut self, client_id: impl Into<String>) {
        let id = client_id.into();
        if !self.assigned_clients.contains(&id) {
            self.assigned_clients.push(id);
            self.updated_at = Utc::now();
        }
    }

    pub fn revoke_client_access(&mut self, client_id: &str) {
        self.assigned_clients.retain(|c| c != client_id);
        self.updated_at = Utc::now();
    }

    pub fn has_role(&self, role: &str) -> bool {
        self.roles.iter().any(|r| r.role == role)
    }

    pub fn can_access_client(&self, client_id: &str) -> bool {
        self.scope.can_access_client(
            client_id,
            self.client_id.as_deref(),
            &self.assigned_clients,
        )
    }

    pub fn deactivate(&mut self) {
        self.active = false;
        self.updated_at = Utc::now();
    }

    pub fn activate(&mut self) {
        self.active = true;
        self.updated_at = Utc::now();
    }

    pub fn is_user(&self) -> bool {
        self.principal_type == PrincipalType::User
    }

    pub fn is_service(&self) -> bool {
        self.principal_type == PrincipalType::Service
    }

    pub fn email(&self) -> Option<&str> {
        self.user_identity.as_ref().map(|i| i.email.as_str())
    }
}
