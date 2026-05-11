//! Delete Dispatch Pool Use Case

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use super::events::DispatchPoolDeleted;
use crate::usecase::{ExecutionContext, UnitOfWork, UseCase, UseCaseError, UseCaseResult};
use crate::DispatchPoolRepository;

/// Command for deleting a dispatch pool.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteDispatchPoolCommand {
    /// Dispatch pool ID
    pub id: String,
}

/// Use case for deleting a dispatch pool.
pub struct DeleteDispatchPoolUseCase<U: UnitOfWork> {
    dispatch_pool_repo: Arc<DispatchPoolRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> DeleteDispatchPoolUseCase<U> {
    pub fn new(dispatch_pool_repo: Arc<DispatchPoolRepository>, unit_of_work: Arc<U>) -> Self {
        Self {
            dispatch_pool_repo,
            unit_of_work,
        }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for DeleteDispatchPoolUseCase<U> {
    type Command = DeleteDispatchPoolCommand;
    type Event = DispatchPoolDeleted;

    async fn validate(&self, _command: &DeleteDispatchPoolCommand) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn authorize(
        &self,
        _command: &DeleteDispatchPoolCommand,
        _ctx: &ExecutionContext,
    ) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        command: DeleteDispatchPoolCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<DispatchPoolDeleted> {
        // Find the dispatch pool
        let pool = match self.dispatch_pool_repo.find_by_id(&command.id).await {
            Ok(Some(p)) => p,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "DISPATCH_POOL_NOT_FOUND",
                    format!("Dispatch pool with ID '{}' not found", command.id),
                ));
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to find dispatch pool: {}",
                    e
                )));
            }
        };

        // Create domain event
        let event = DispatchPoolDeleted::new(&ctx, &pool.id, &pool.code);

        // Atomic commit with delete
        self.unit_of_work
            .commit_delete(&pool, &*self.dispatch_pool_repo, event, &command)
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_command_serialization() {
        let cmd = DeleteDispatchPoolCommand {
            id: "dp-123".to_string(),
        };

        let json = serde_json::to_string(&cmd).unwrap();
        assert!(json.contains("dp-123"));
    }
}
