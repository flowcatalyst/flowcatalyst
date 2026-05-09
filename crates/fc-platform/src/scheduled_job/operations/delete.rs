//! Delete ScheduledJob — hard removes the definition row. Instances + logs
//! remain (history retention is partition-driven). Prefer Archive for normal
//! lifecycle; Delete is for cleanup of mistakes / abandoned definitions.

use std::sync::Arc;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use super::events::ScheduledJobDeleted;
use crate::scheduled_job::ScheduledJobRepository;
use crate::usecase::{ExecutionContext, UnitOfWork, UseCase, UseCaseError, UseCaseResult};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteScheduledJobCommand {
    pub scheduled_job_id: String,
}

pub struct DeleteScheduledJobUseCase<U: UnitOfWork> {
    repo: Arc<ScheduledJobRepository>,
    uow: Arc<U>,
}

impl<U: UnitOfWork> DeleteScheduledJobUseCase<U> {
    pub fn new(repo: Arc<ScheduledJobRepository>, uow: Arc<U>) -> Self {
        Self { repo, uow }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for DeleteScheduledJobUseCase<U> {
    type Command = DeleteScheduledJobCommand;
    type Event = ScheduledJobDeleted;

    async fn validate(&self, cmd: &Self::Command) -> Result<(), UseCaseError> {
        if cmd.scheduled_job_id.trim().is_empty() {
            return Err(UseCaseError::validation("ID_REQUIRED", "ID required"));
        }
        Ok(())
    }

    async fn authorize(&self, _: &Self::Command, _: &ExecutionContext) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        cmd: Self::Command,
        ctx: ExecutionContext,
    ) -> UseCaseResult<Self::Event> {
        let job = match self.repo.find_by_id(&cmd.scheduled_job_id).await {
            Ok(Some(j)) => j,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "SCHEDULED_JOB_NOT_FOUND",
                    format!("ScheduledJob '{}' not found", cmd.scheduled_job_id),
                ))
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to load ScheduledJob: {}", e
                )))
            }
        };

        let event = ScheduledJobDeleted::new(&ctx, &job.id, job.client_id.as_deref(), &job.code);
        self.uow.commit_delete(&job, &*self.repo, event, &cmd).await
    }
}
