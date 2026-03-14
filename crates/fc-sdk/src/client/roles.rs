//! Role and permission management operations.

use serde::{Deserialize, Serialize};
use super::{FlowCatalystClient, ClientError, ListResponse};

/// Request to create a role.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct CreateRoleRequest {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub permissions: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_managed: Option<bool>,
}

/// Request to update a role.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct UpdateRoleRequest {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub permissions: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_managed: Option<bool>,
}

/// Role response from the platform API.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RoleResponse {
    pub name: String,
    #[serde(default)]
    pub display_name: Option<String>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub source: Option<String>,
    #[serde(default)]
    pub application_id: Option<String>,
    #[serde(default)]
    pub permissions: Option<Vec<String>>,
    #[serde(default)]
    pub client_managed: Option<bool>,
    #[serde(default)]
    pub created_at: Option<String>,
    #[serde(default)]
    pub updated_at: Option<String>,
}

/// Permission response from the platform API.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PermissionResponse {
    pub name: String,
    #[serde(default)]
    pub display_name: Option<String>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub category: Option<String>,
}

impl FlowCatalystClient {
    // ── Roles ────────────────────────────────────────────────────────────────

    /// List all roles.
    pub async fn list_roles(&self) -> Result<ListResponse<RoleResponse>, ClientError> {
        self.get("/api/admin/roles").await
    }

    /// Get a role by name.
    pub async fn get_role(&self, name: &str) -> Result<RoleResponse, ClientError> {
        self.get(&format!("/api/admin/roles/{}", name)).await
    }

    /// Create a new role.
    pub async fn create_role(
        &self,
        req: &CreateRoleRequest,
    ) -> Result<RoleResponse, ClientError> {
        self.post("/api/admin/roles", req).await
    }

    /// Update an existing role by name.
    pub async fn update_role(
        &self,
        name: &str,
        req: &UpdateRoleRequest,
    ) -> Result<RoleResponse, ClientError> {
        self.put(&format!("/api/admin/roles/{}", name), req).await
    }

    /// Delete a role by name.
    pub async fn delete_role(&self, name: &str) -> Result<(), ClientError> {
        self.delete_req(&format!("/api/admin/roles/{}", name)).await
    }

    /// List roles scoped to an application.
    pub async fn list_roles_for_application(
        &self,
        application_id: &str,
    ) -> Result<ListResponse<RoleResponse>, ClientError> {
        self.get(&format!(
            "/api/admin/roles/by-application/{}",
            application_id
        ))
        .await
    }

    // ── Permissions ──────────────────────────────────────────────────────────

    /// List all permissions.
    pub async fn list_permissions(
        &self,
    ) -> Result<ListResponse<PermissionResponse>, ClientError> {
        self.get("/api/admin/roles/permissions").await
    }

    /// Get a permission by name.
    pub async fn get_permission(
        &self,
        name: &str,
    ) -> Result<PermissionResponse, ClientError> {
        self.get(&format!("/api/admin/roles/permissions/{}", name))
            .await
    }
}
