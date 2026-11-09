//! Push gateway rules and ticket keys — `POST /api/applications/{appCode}/push/sync`.

use super::{ClientError, FlowCatalystClient};
use crate::client::seg;
use crate::push::{PushDefinition, PushRule, TicketJwks};
use serde::{Deserialize, Serialize};

/// Request body of the push sync (`SyncPushRequest`): a declarative full
/// replace of the application's topic rules and ticket verification keys.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SyncPushRequest {
    pub rules: Vec<PushRule>,
    pub keys: TicketJwks,
}

impl From<&PushDefinition> for SyncPushRequest {
    fn from(d: &PushDefinition) -> Self {
        Self {
            rules: d.rules.clone(),
            keys: d.keys.clone(),
        }
    }
}

/// Created / updated / deleted counts of one half of the push sync.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct PushSyncCounts {
    #[serde(default)]
    pub created: u32,
    #[serde(default)]
    pub updated: u32,
    #[serde(default)]
    pub deleted: u32,
}

/// The gateway's answer (`PushSyncResponse`).
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct PushSyncResponse {
    pub application_code: String,
    #[serde(default)]
    pub rules: PushSyncCounts,
    #[serde(default)]
    pub keys: PushSyncCounts,
    /// The topic patterns the gateway holds for the application.
    #[serde(default)]
    pub synced_patterns: Vec<String>,
    /// The key ids the gateway holds for the application.
    #[serde(default)]
    pub synced_kids: Vec<String>,
}

/// Push resource accessor — created via [`FlowCatalystClient::push`].
#[derive(Debug, Clone, Copy)]
pub struct Push<'a> {
    pub(crate) client: &'a FlowCatalystClient,
}

impl Push<'_> {
    /// Replace the application's push topic rules and ticket keys. The gateway
    /// refuses (`400`, with a code) rules for another application's patterns,
    /// duplicates, more than 200 rules, more than 4 keys, or keys that are not
    /// public Ed25519 JWKs.
    pub async fn sync(
        &self,
        app_code: &str,
        req: &SyncPushRequest,
    ) -> Result<PushSyncResponse, ClientError> {
        self.client
            .post(
                &format!("/api/applications/{}/push/sync", seg(app_code)),
                req,
            )
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::client::test_support::MockPlatform;

    #[tokio::test]
    async fn sync_posts_rules_and_keys_and_reads_the_counts() {
        let stub = MockPlatform::start(&[(
            "POST",
            "/api/applications/shop/push/sync",
            200,
            r#"{"applicationCode":"shop","rules":{"created":1,"updated":0,"deleted":0},"keys":{"created":1,"updated":0,"deleted":2},"syncedPatterns":["shop:orders:{principal}"],"syncedKids":["k1"]}"#,
        )])
        .await;
        let def = PushDefinition::new(
            vec![
                PushRule::new("shop:orders:{principal}"),
                PushRule::new("shop:board").public(),
                PushRule::new("shop:ops").requires(["shop:ops:board:view"]),
            ],
            TicketJwks::default(),
        );
        let r = stub
            .client()
            .push()
            .sync("shop", &SyncPushRequest::from(&def))
            .await
            .unwrap();
        assert_eq!(r.keys.deleted, 2);
        assert_eq!(r.synced_kids, ["k1"]);
        let req = &stub.requests()[0];
        assert_eq!(
            req.json(),
            serde_json::json!({
                "rules": [
                    {"pattern": "shop:orders:{principal}"},
                    {"pattern": "shop:board", "public": true},
                    {"pattern": "shop:ops", "requires": ["shop:ops:board:view"]}
                ],
                "keys": {"keys": []}
            })
        );
    }
}
