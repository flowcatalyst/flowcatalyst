//! EventType Repository — PostgreSQL via SeaORM

use async_trait::async_trait;
use sea_orm::*;
use sea_orm::sea_query::OnConflict;
use chrono::Utc;

use super::entity::{EventType, EventTypeStatus, SpecVersion};
use crate::entities::{msg_event_types, msg_event_type_spec_versions};
use crate::shared::error::Result;
use crate::usecase::unit_of_work::{HasId, PgPersist};

pub struct EventTypeRepository {
    db: DatabaseConnection,
}

impl EventTypeRepository {
    pub fn new(db: &DatabaseConnection) -> Self {
        Self { db: db.clone() }
    }

    async fn load_spec_versions(&self, event_type_id: &str) -> Result<Vec<SpecVersion>> {
        let rows = msg_event_type_spec_versions::Entity::find()
            .filter(msg_event_type_spec_versions::Column::EventTypeId.eq(event_type_id))
            .order_by_asc(msg_event_type_spec_versions::Column::Version)
            .all(&self.db)
            .await?;
        Ok(rows.into_iter().map(SpecVersion::from).collect())
    }

    async fn hydrate(&self, mut et: EventType) -> Result<EventType> {
        et.spec_versions = self.load_spec_versions(&et.id).await?;
        Ok(et)
    }

    /// Batch-hydrate spec versions for multiple event types (avoids N+1)
    async fn hydrate_all(&self, models: Vec<msg_event_types::Model>) -> Result<Vec<EventType>> {
        if models.is_empty() {
            return Ok(vec![]);
        }

        let ids: Vec<String> = models.iter().map(|m| m.id.clone()).collect();
        let all_specs = msg_event_type_spec_versions::Entity::find()
            .filter(msg_event_type_spec_versions::Column::EventTypeId.is_in(ids))
            .order_by_asc(msg_event_type_spec_versions::Column::Version)
            .all(&self.db)
            .await?;

        let mut spec_map: std::collections::HashMap<String, Vec<SpecVersion>> = std::collections::HashMap::new();
        for row in all_specs {
            let event_type_id = row.event_type_id.clone();
            spec_map.entry(event_type_id).or_default().push(SpecVersion::from(row));
        }

        Ok(models.into_iter().map(|m| {
            let id = m.id.clone();
            let mut et = EventType::from(m);
            if let Some(specs) = spec_map.remove(&id) {
                et.spec_versions = specs;
            }
            et
        }).collect())
    }

    pub async fn insert(&self, et: &EventType) -> Result<()> {
        let model = msg_event_types::ActiveModel {
            id: Set(et.id.clone()),
            code: Set(et.code.clone()),
            name: Set(et.name.clone()),
            description: Set(et.description.clone()),
            status: Set(et.status.as_str().to_string()),
            source: Set(et.source.as_str().to_string()),
            client_scoped: Set(et.client_scoped),
            application: Set(et.application.clone()),
            subdomain: Set(et.subdomain.clone()),
            aggregate: Set(et.aggregate.clone()),
            created_at: Set(Utc::now().into()),
            updated_at: Set(Utc::now().into()),
        };
        msg_event_types::Entity::insert(model).exec(&self.db).await?;

        for sv in &et.spec_versions {
            self.insert_spec_version(sv).await?;
        }
        Ok(())
    }

    pub async fn insert_spec_version(&self, sv: &SpecVersion) -> Result<()> {
        let schema_json = sv.schema_content.clone().map(|v| sea_orm::JsonValue::from(v));
        let model = msg_event_type_spec_versions::ActiveModel {
            id: Set(sv.id.clone()),
            event_type_id: Set(sv.event_type_id.clone()),
            version: Set(sv.version.clone()),
            mime_type: Set(sv.mime_type.clone()),
            schema_content: Set(schema_json),
            schema_type: Set(sv.schema_type.as_str().to_string()),
            status: Set(sv.status.as_str().to_string()),
            created_at: Set(Utc::now().into()),
            updated_at: Set(Utc::now().into()),
        };
        msg_event_type_spec_versions::Entity::insert(model).exec(&self.db).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<EventType>> {
        let result = msg_event_types::Entity::find_by_id(id).one(&self.db).await?;
        match result {
            Some(m) => Ok(Some(self.hydrate(EventType::from(m)).await?)),
            None => Ok(None),
        }
    }

    pub async fn find_by_code(&self, code: &str) -> Result<Option<EventType>> {
        let result = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Code.eq(code))
            .one(&self.db)
            .await?;
        match result {
            Some(m) => Ok(Some(self.hydrate(EventType::from(m)).await?)),
            None => Ok(None),
        }
    }

    pub async fn find_all(&self) -> Result<Vec<EventType>> {
        let rows = msg_event_types::Entity::find()
            .order_by_asc(msg_event_types::Column::Code)
            .all(&self.db)
            .await?;
        self.hydrate_all(rows).await
    }

    pub async fn find_by_application(&self, application: &str) -> Result<Vec<EventType>> {
        let rows = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Application.eq(application))
            .all(&self.db)
            .await?;
        self.hydrate_all(rows).await
    }

    pub async fn find_by_status(&self, status: EventTypeStatus) -> Result<Vec<EventType>> {
        let rows = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Status.eq(status.as_str()))
            .order_by_asc(msg_event_types::Column::Code)
            .all(&self.db)
            .await?;
        self.hydrate_all(rows).await
    }

    /// Search event types by code or name (case-insensitive partial match)
    pub async fn search(&self, term: &str) -> Result<Vec<EventType>> {
        let pattern = format!("%{}%", term);
        let rows = msg_event_types::Entity::find()
            .filter(
                Condition::any()
                    .add(msg_event_types::Column::Code.like(&pattern))
                    .add(msg_event_types::Column::Name.like(&pattern))
            )
            .all(&self.db)
            .await?;
        self.hydrate_all(rows).await
    }

    pub async fn find_active(&self) -> Result<Vec<EventType>> {
        let rows = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Status.eq("CURRENT"))
            .order_by_asc(msg_event_types::Column::Code)
            .all(&self.db)
            .await?;
        self.hydrate_all(rows).await
    }

    /// Find active event types without loading spec versions (for filter endpoints)
    pub async fn find_active_shallow(&self) -> Result<Vec<EventType>> {
        let rows = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Status.eq("CURRENT"))
            .order_by_asc(msg_event_types::Column::Code)
            .all(&self.db)
            .await?;
        Ok(rows.into_iter().map(EventType::from).collect())
    }

    pub async fn exists_by_code(&self, code: &str) -> Result<bool> {
        let count = msg_event_types::Entity::find()
            .filter(msg_event_types::Column::Code.eq(code))
            .count(&self.db)
            .await?;
        Ok(count > 0)
    }

    pub async fn update(&self, et: &EventType) -> Result<()> {
        let model = msg_event_types::ActiveModel {
            id: Set(et.id.clone()),
            code: Set(et.code.clone()),
            name: Set(et.name.clone()),
            description: Set(et.description.clone()),
            status: Set(et.status.as_str().to_string()),
            source: Set(et.source.as_str().to_string()),
            client_scoped: Set(et.client_scoped),
            application: Set(et.application.clone()),
            subdomain: Set(et.subdomain.clone()),
            aggregate: Set(et.aggregate.clone()),
            created_at: NotSet,
            updated_at: Set(Utc::now().into()),
        };
        msg_event_types::Entity::update(model).exec(&self.db).await?;
        Ok(())
    }

    pub async fn update_spec_version(&self, sv: &SpecVersion) -> Result<()> {
        let model = msg_event_type_spec_versions::ActiveModel {
            id: Set(sv.id.clone()),
            event_type_id: NotSet,
            version: NotSet,
            mime_type: Set(sv.mime_type.clone()),
            schema_content: Set(sv.schema_content.clone().map(sea_orm::JsonValue::from)),
            schema_type: Set(sv.schema_type.as_str().to_string()),
            status: Set(sv.status.as_str().to_string()),
            created_at: NotSet,
            updated_at: Set(Utc::now().into()),
        };
        msg_event_type_spec_versions::Entity::update(model).exec(&self.db).await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        // Delete spec versions first
        msg_event_type_spec_versions::Entity::delete_many()
            .filter(msg_event_type_spec_versions::Column::EventTypeId.eq(id))
            .exec(&self.db)
            .await?;
        let result = msg_event_types::Entity::delete_by_id(id).exec(&self.db).await?;
        Ok(result.rows_affected > 0)
    }
}

// ── PgPersist implementation ──────────────────────────────────────────────────

impl HasId for EventType {
    fn id(&self) -> &str { &self.id }
}

#[async_trait]
impl PgPersist for EventType {
    async fn pg_upsert(&self, txn: &sea_orm::DatabaseTransaction) -> Result<()> {
        let model = msg_event_types::ActiveModel {
            id: Set(self.id.clone()),
            code: Set(self.code.clone()),
            name: Set(self.name.clone()),
            description: Set(self.description.clone()),
            status: Set(self.status.as_str().to_string()),
            source: Set(self.source.as_str().to_string()),
            client_scoped: Set(self.client_scoped),
            application: Set(self.application.clone()),
            subdomain: Set(self.subdomain.clone()),
            aggregate: Set(self.aggregate.clone()),
            created_at: Set(Utc::now().into()),
            updated_at: Set(Utc::now().into()),
        };
        msg_event_types::Entity::insert(model)
            .on_conflict(
                OnConflict::column(msg_event_types::Column::Id)
                    .update_columns([
                        msg_event_types::Column::Name,
                        msg_event_types::Column::Description,
                        msg_event_types::Column::Status,
                        msg_event_types::Column::Source,
                        msg_event_types::Column::ClientScoped,
                        msg_event_types::Column::UpdatedAt,
                    ])
                    .to_owned(),
            )
            .exec(txn)
            .await?;
        Ok(())
    }

    async fn pg_delete(&self, txn: &sea_orm::DatabaseTransaction) -> Result<()> {
        msg_event_type_spec_versions::Entity::delete_many()
            .filter(msg_event_type_spec_versions::Column::EventTypeId.eq(&self.id))
            .exec(txn)
            .await?;
        msg_event_types::Entity::delete_by_id(&self.id).exec(txn).await?;
        Ok(())
    }
}
