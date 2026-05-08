//! Revoke Passkey Use Case
//!
//! Removes a single registered passkey. The caller must be the owning principal.

use std::sync::Arc;
use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::webauthn::repository::WebauthnCredentialRepository;
use crate::usecase::{
    ExecutionContext, UseCase, UnitOfWork, UseCaseError, UseCaseResult,
};
use super::events::PasskeyRevoked;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RevokePasskeyCommand {
    pub credential_id: String,
}

pub struct RevokePasskeyUseCase<U: UnitOfWork> {
    credential_repo: Arc<WebauthnCredentialRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> RevokePasskeyUseCase<U> {
    pub fn new(credential_repo: Arc<WebauthnCredentialRepository>, unit_of_work: Arc<U>) -> Self {
        Self { credential_repo, unit_of_work }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for RevokePasskeyUseCase<U> {
    type Command = RevokePasskeyCommand;
    type Event = PasskeyRevoked;

    async fn validate(&self, command: &RevokePasskeyCommand) -> Result<(), UseCaseError> {
        if command.credential_id.trim().is_empty() {
            return Err(UseCaseError::validation("CREDENTIAL_ID_REQUIRED", "credentialId is required"));
        }
        Ok(())
    }

    async fn authorize(&self, _command: &RevokePasskeyCommand, _ctx: &ExecutionContext) -> Result<(), UseCaseError> {
        // Owner check happens in execute() because we need to load the row first.
        Ok(())
    }

    async fn execute(
        &self,
        command: RevokePasskeyCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<PasskeyRevoked> {
        let credential = match self.credential_repo.find_by_id(&command.credential_id).await {
            Ok(Some(c)) => c,
            Ok(None) => return UseCaseResult::failure(UseCaseError::not_found(
                "CREDENTIAL_NOT_FOUND",
                format!("passkey '{}' not found", command.credential_id),
            )),
            Err(e) => return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to load passkey: {}", e,
            ))),
        };

        if ctx.principal_id != credential.principal_id {
            return UseCaseResult::failure(UseCaseError::business_rule(
                "PRINCIPAL_MISMATCH",
                "you may only revoke your own passkeys",
            ));
        }

        let event = PasskeyRevoked::new(&ctx, &credential.id, &credential.principal_id);

        self.unit_of_work
            .commit_delete(&credential, &*self.credential_repo, event, &command)
            .await
    }
}
