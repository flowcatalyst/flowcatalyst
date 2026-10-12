//! Application documentation pages — `POST /api/applications/{appCode}/docs/sync`.

use super::{ClientError, FlowCatalystClient};
use crate::client::SyncResult;
use crate::client::seg;
use serde::{Deserialize, Serialize};

/// Request body for the docs sync endpoint (`SyncDocsRequest`).
///
/// The sync is a declarative full replace: the platform's pages for the
/// application become exactly this list.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SyncDocsRequest {
    pub docs: Vec<SyncDocItem>,
}

/// One documentation page (`SyncDocInputRequest`).
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SyncDocItem {
    /// URL-safe page id, unique within the application (kebab-case).
    pub slug: String,
    /// The page body, Markdown (Mermaid fences render as diagrams).
    pub content: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
}

/// Docs resource accessor — created via [`FlowCatalystClient::docs`].
#[derive(Debug, Clone, Copy)]
pub struct Docs<'a> {
    pub(crate) client: &'a FlowCatalystClient,
}

impl Docs<'_> {
    /// Replace the application's documentation pages with `req.docs`.
    pub async fn sync(
        &self,
        app_code: &str,
        req: &SyncDocsRequest,
    ) -> Result<SyncResult, ClientError> {
        self.client
            .post(
                &format!("/api/applications/{}/docs/sync", seg(app_code)),
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
    async fn sync_posts_docs_and_skips_absent_title() {
        let stub = MockPlatform::start(&[(
            "POST",
            "/api/applications/orders/docs/sync",
            200,
            r#"{"applicationCode":"orders","created":2,"updated":0,"deleted":0,"syncedCodes":["intro","api"]}"#,
        )])
        .await;
        let req = SyncDocsRequest {
            docs: vec![
                SyncDocItem {
                    slug: "intro".into(),
                    content: "# Hi".into(),
                    title: Some("Intro".into()),
                },
                SyncDocItem {
                    slug: "api".into(),
                    content: "x".into(),
                    title: None,
                },
            ],
        };
        let r = stub.client().docs().sync("orders", &req).await.unwrap();
        assert_eq!(r.created, 2);
        let rec = stub.single();
        assert_eq!(rec.query, "");
        assert_eq!(
            rec.json(),
            serde_json::json!({"docs":[{"slug":"intro","content":"# Hi","title":"Intro"},{"slug":"api","content":"x"}]})
        );
    }
}
