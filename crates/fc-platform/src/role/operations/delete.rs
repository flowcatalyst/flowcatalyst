//! Delete Role Use Case

use std::sync::Arc;
use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::role::entity::RoleSource;
use crate::role::repository::RoleRepository;
use crate::usecase::{
    ExecutionContext, UseCase, UnitOfWork, UseCaseError, UseCaseResult,
};
use super::events::RoleDeleted;

/// Command for deleting a role.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteRoleCommand {
    /// Role ID to delete
    pub role_id: String,
}

/// Use case for deleting a role.
pub struct DeleteRoleUseCase<U: UnitOfWork> {
    role_repo: Arc<RoleRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> DeleteRoleUseCase<U> {
    pub fn new(role_repo: Arc<RoleRepository>, unit_of_work: Arc<U>) -> Self {
        Self {
            role_repo,
            unit_of_work,
        }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for DeleteRoleUseCase<U> {
    type Command = DeleteRoleCommand;
    type Event = RoleDeleted;

    async fn validate(&self, command: &DeleteRoleCommand) -> Result<(), UseCaseError> {
        if command.role_id.trim().is_empty() {
            return Err(UseCaseError::validation(
                "ROLE_ID_REQUIRED",
                "Role ID is required",
            ));
        }
        Ok(())
    }

    async fn authorize(&self, _command: &DeleteRoleCommand, _ctx: &ExecutionContext) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        command: DeleteRoleCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<RoleDeleted> {
        // Fetch existing role
        let role = match self.role_repo.find_by_id(&command.role_id).await {
            Ok(Some(r)) => r,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "ROLE_NOT_FOUND",
                    format!("Role with ID '{}' not found", command.role_id),
                ));
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to fetch role: {}",
                    e
                )));
            }
        };

        // Business rule: can only delete database-defined roles
        if role.source != RoleSource::Database {
            return UseCaseResult::failure(UseCaseError::business_rule(
                "CANNOT_DELETE_ROLE",
                "Cannot delete a code-defined or SDK-synced role",
            ));
        }

        // Create domain event
        let event = RoleDeleted::new(&ctx, &role.id, &role.name);

        // Atomic commit with delete
        self.unit_of_work
            .commit_delete(&role, &*self.role_repo, event, &command)
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_command_serialization() {
        let cmd = DeleteRoleCommand {
            role_id: "role-123".to_string(),
        };

        let json = serde_json::to_string(&cmd).unwrap();
        assert!(json.contains("roleId"));
    }
}
