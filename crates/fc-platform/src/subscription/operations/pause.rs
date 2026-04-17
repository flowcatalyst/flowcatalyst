//! Pause Subscription Use Case

use std::sync::Arc;
use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::SubscriptionStatus;
use crate::SubscriptionRepository;
use crate::usecase::{
    ExecutionContext, UseCase, UnitOfWork, UseCaseError, UseCaseResult,
};
use super::events::SubscriptionPaused;

/// Command for pausing a subscription.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PauseSubscriptionCommand {
    /// Subscription ID to pause
    pub subscription_id: String,
}

/// Use case for pausing a subscription.
pub struct PauseSubscriptionUseCase<U: UnitOfWork> {
    subscription_repo: Arc<SubscriptionRepository>,
    unit_of_work: Arc<U>,
}

impl<U: UnitOfWork> PauseSubscriptionUseCase<U> {
    pub fn new(subscription_repo: Arc<SubscriptionRepository>, unit_of_work: Arc<U>) -> Self {
        Self {
            subscription_repo,
            unit_of_work,
        }
    }
}

#[async_trait]
impl<U: UnitOfWork> UseCase for PauseSubscriptionUseCase<U> {
    type Command = PauseSubscriptionCommand;
    type Event = SubscriptionPaused;

    async fn validate(&self, command: &PauseSubscriptionCommand) -> Result<(), UseCaseError> {
        if command.subscription_id.trim().is_empty() {
            return Err(UseCaseError::validation(
                "SUBSCRIPTION_ID_REQUIRED",
                "Subscription ID is required",
            ));
        }
        Ok(())
    }

    async fn authorize(&self, _command: &PauseSubscriptionCommand, _ctx: &ExecutionContext) -> Result<(), UseCaseError> {
        Ok(())
    }

    async fn execute(
        &self,
        command: PauseSubscriptionCommand,
        ctx: ExecutionContext,
    ) -> UseCaseResult<SubscriptionPaused> {
        // Fetch existing subscription
        let mut subscription = match self.subscription_repo.find_by_id(&command.subscription_id).await {
            Ok(Some(s)) => s,
            Ok(None) => {
                return UseCaseResult::failure(UseCaseError::not_found(
                    "SUBSCRIPTION_NOT_FOUND",
                    format!("Subscription with ID '{}' not found", command.subscription_id),
                ));
            }
            Err(e) => {
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to fetch subscription: {}",
                    e
                )));
            }
        };

        // Business rule: can only pause active subscriptions
        if subscription.status == SubscriptionStatus::Paused {
            return UseCaseResult::failure(UseCaseError::business_rule(
                "ALREADY_PAUSED",
                "Subscription is already paused",
            ));
        }

        // Pause the subscription
        subscription.pause();

        // Create domain event
        let event = SubscriptionPaused::new(&ctx, &subscription.id, &subscription.code);

        // Atomic commit
        self.unit_of_work
            .commit(&subscription, &*self.subscription_repo, event, &command)
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_command_serialization() {
        let cmd = PauseSubscriptionCommand {
            subscription_id: "sub-123".to_string(),
        };

        let json = serde_json::to_string(&cmd).unwrap();
        assert!(json.contains("subscriptionId"));
    }
}
