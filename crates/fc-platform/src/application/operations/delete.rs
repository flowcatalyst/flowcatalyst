//! Delete Application Use Case

use std::sync::Arc;
use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::ApplicationRepository;
use crate::usecase::{
    ExecutionContext, UnitOfWork, UseCase, UseCaseError, UseCaseResult,
};
use super::events::ApplicationDeleted;

/// Command for deleting an application.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteApplicationCommand {
    pub application_id: String,
}

pub struct DeleteApplicationUseCase<U: UnitOfWork> {
    application_repo: Arc<ApplicationRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> DeleteApplicationUseCase<U> {
    pub fn new(application_repo: Arc<ApplicationRepository>, unit_of_work: Arc<U>) -> Self {
        Self { application_repo, unit_of_work }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for DeleteApplicationUseCase<U> {
    type Command = DeleteApplicationCommand;
    type Event = ApplicationDeleted;

    async fn validate(&self, command: &DeleteApplicationCommand) -> Result<(), UseCaseError> {
        if command.application_id.trim().is_empty() {
            return Err(UseCaseError::validation(
                "APPLICATION_ID_REQUIRED", "Application ID is required",
            ));
        }

        Ok(())
    }

    async fn authorize(&self, _command: &DeleteApplicationCommand, _ctx: &ExecutionContext) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        command: DeleteApplicationCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<ApplicationDeleted> {
        let application = match self.application_repo.find_by_id(&command.application_id).await {
            Ok(Some(a)) => a,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "APPLICATION_NOT_FOUND",
                    format!("Application with ID '{}' not found", command.application_id),
                ));
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to fetch application: {}", e
                )));
            }
        };

        let event = ApplicationDeleted::new(
            &ctx,
            &application.id,
            &application.code,
            &application.name,
        );

        self.unit_of_work.commit_delete(&application, event, &command).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_command_serialization() {
        let cmd = DeleteApplicationCommand {
            application_id: "app-123".to_string(),
        };
        let json = serde_json::to_string(&cmd).unwrap();
        assert!(json.contains("applicationId"));
    }
}
