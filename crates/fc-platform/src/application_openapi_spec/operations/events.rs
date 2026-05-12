//! OpenAPI spec domain events.

use serde::{Deserialize, Serialize};

use crate::impl_domain_event;
use crate::usecase::domain_event::EventMetadata;
use crate::usecase::ExecutionContext;
use crate::TsidGenerator;

/// Emitted when an application syncs a new OpenAPI document, whether the
/// content was new (versionDelta=true) or byte-identical to the prior CURRENT
/// (versionDelta=false). The audit log keeps both cases for completeness.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplicationOpenApiSpecSynced {
    #[serde(flatten)]
    pub metadata: EventMetadata,

    pub application_id: String,
    pub application_code: String,
    pub spec_id: String,
    pub version: String,
    pub spec_hash: String,
    /// Some when a prior CURRENT was archived in this sync.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub archived_prior_version: Option<String>,
    /// True if the diff includes removed paths/schemas/verbs.
    pub has_breaking: bool,
    /// True if the incoming spec was byte-identical to the existing CURRENT
    /// (no new row was inserted).
    pub unchanged: bool,
}

impl_domain_event!(ApplicationOpenApiSpecSynced);

impl ApplicationOpenApiSpecSynced {
    const EVENT_TYPE: &'static str = "platform:developer:application-openapi:synced";
    const SPEC_VERSION: &'static str = "1.0";
    const SOURCE: &'static str = "platform:developer";

    #[allow(clippy::too_many_arguments)]
    pub fn new(
        ctx: &ExecutionContext,
        application_id: &str,
        application_code: &str,
        spec_id: &str,
        version: &str,
        spec_hash: &str,
        archived_prior_version: Option<String>,
        has_breaking: bool,
        unchanged: bool,
    ) -> Self {
        let event_id = TsidGenerator::generate_untyped();
        let subject = format!("platform.application-openapi.{}", spec_id);
        let message_group = format!("platform:application-openapi:{}", application_id);

        Self {
            metadata: EventMetadata::new(
                event_id,
                Self::EVENT_TYPE,
                Self::SPEC_VERSION,
                Self::SOURCE,
                subject,
                message_group,
                ctx.execution_id.clone(),
                ctx.correlation_id.clone(),
                ctx.causation_id.clone(),
                ctx.principal_id.clone(),
            ),
            application_id: application_id.to_string(),
            application_code: application_code.to_string(),
            spec_id: spec_id.to_string(),
            version: version.to_string(),
            spec_hash: spec_hash.to_string(),
            archived_prior_version,
            has_breaking,
            unchanged,
        }
    }
}
