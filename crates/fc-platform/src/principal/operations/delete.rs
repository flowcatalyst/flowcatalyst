//! Delete User Use Case

use std::sync::Arc;
use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::principal::repository::PrincipalRepository;
use crate::usecase::{
    ExecutionContext, UseCase, UnitOfWork, UseCaseError, UseCaseResult,
};
use super::events::UserDeleted;

/// Command for deleting a user.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteUserCommand {
    /// Principal ID to delete
    pub principal_id: String,
}

/// Use case for deleting a user (soft delete - deactivates permanently).
pub struct DeleteUserUseCase<U: UnitOfWork> {
    principal_repo: Arc<PrincipalRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> DeleteUserUseCase<U> {
    pub fn new(principal_repo: Arc<PrincipalRepository>, unit_of_work: Arc<U>) -> Self {
        Self {
            principal_repo,
            unit_of_work,
        }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for DeleteUserUseCase<U> {
    type Command = DeleteUserCommand;
    type Event = UserDeleted;

    async fn validate(&self, command: &DeleteUserCommand) -> Result<(), UseCaseError> {
        if command.principal_id.trim().is_empty() {
            return Err(UseCaseError::validation(
                "PRINCIPAL_ID_REQUIRED",
                "Principal ID is required",
            ));
        }

        Ok(())
    }

    async fn authorize(&self, _command: &DeleteUserCommand, _ctx: &ExecutionContext) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        command: DeleteUserCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<UserDeleted> {
        // Business rule: cannot delete yourself
        if command.principal_id == ctx.principal_id {
            return UseCaseResult::failure(UseCaseError::business_rule(
                "CANNOT_DELETE_SELF",
                "Cannot delete your own account",
            ));
        }

        // Fetch existing principal
        let principal = match self.principal_repo.find_by_id(&command.principal_id).await {
            Ok(Some(p)) => p,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "USER_NOT_FOUND",
                    format!("User with ID '{}' not found", command.principal_id),
                ));
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to fetch user: {}",
                    e
                )));
            }
        };

        // Create domain event
        let event = UserDeleted::new(&ctx, &principal.id);

        // Atomic commit with delete
        self.unit_of_work
            .commit_delete(&principal, &*self.principal_repo, event, &command)
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_command_serialization() {
        let cmd = DeleteUserCommand {
            principal_id: "user-123".to_string(),
        };

        let json = serde_json::to_string(&cmd).unwrap();
        assert!(json.contains("principalId"));
    }
}
