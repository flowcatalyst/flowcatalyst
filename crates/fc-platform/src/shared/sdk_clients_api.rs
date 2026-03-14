//! SDK Clients API — simplified client management for external SDK access

use axum::{
    routing::{get, post},
    extract::{State, Path},
    Json, Router,
};
use std::sync::Arc;

use crate::client::api::{
    ClientResponse, ClientListResponse,
    CreateClientRequest, UpdateClientRequest,
    StatusChangeRequest, StatusChangeResponse,
};
use crate::client::repository::ClientRepository;
use crate::client::operations::{
    CreateClientCommand, CreateClientUseCase,
    UpdateClientCommand, UpdateClientUseCase,
    ActivateClientCommand, ActivateClientUseCase,
    SuspendClientCommand, SuspendClientUseCase,
};
use crate::shared::api_common::CreatedResponse;
use crate::shared::error::PlatformError;
use crate::shared::middleware::Authenticated;
use crate::usecase::{ExecutionContext, PgUnitOfWork, UseCase};

/// SDK Clients service state
#[derive(Clone)]
pub struct SdkClientsState {
    pub client_repo: Arc<ClientRepository>,
    pub unit_of_work: Arc<PgUnitOfWork>,
}

/// List accessible clients
#[utoipa::path(
    get,
    path = "",
    tag = "sdk-clients",
    operation_id = "getApiSdkClients",
    responses(
        (status = 200, description = "List of accessible clients", body = ClientListResponse)
    ),
    security(("bearer_auth" = []))
)]
async fn list_sdk_clients(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
) -> Result<Json<ClientListResponse>, PlatformError> {
    let clients = state.client_repo.find_active().await?;

    let filtered: Vec<ClientResponse> = clients.into_iter()
        .filter(|c| auth.0.is_anchor() || auth.0.can_access_client(&c.id))
        .map(|c| c.into())
        .collect();

    let total = filtered.len();
    Ok(Json(ClientListResponse { clients: filtered, total }))
}

/// Get client by ID
#[utoipa::path(
    get,
    path = "/{id}",
    tag = "sdk-clients",
    operation_id = "getApiSdkClientsById",
    params(
        ("id" = String, Path, description = "Client ID")
    ),
    responses(
        (status = 200, description = "Client found", body = ClientResponse),
        (status = 403, description = "No access to this client"),
        (status = 404, description = "Client not found")
    ),
    security(("bearer_auth" = []))
)]
async fn get_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Path(id): Path<String>,
) -> Result<Json<ClientResponse>, PlatformError> {
    if !auth.0.is_anchor() && !auth.0.can_access_client(&id) {
        return Err(PlatformError::forbidden("No access to this client"));
    }

    let client = state.client_repo.find_by_id(&id).await?
        .ok_or_else(|| PlatformError::not_found("Client", &id))?;

    Ok(Json(client.into()))
}

/// Create a new client
#[utoipa::path(
    post,
    path = "",
    tag = "sdk-clients",
    operation_id = "postApiSdkClients",
    request_body = CreateClientRequest,
    responses(
        (status = 201, description = "Client created", body = CreatedResponse),
        (status = 400, description = "Validation error"),
        (status = 403, description = "Insufficient permissions"),
        (status = 409, description = "Duplicate identifier")
    ),
    security(("bearer_auth" = []))
)]
async fn create_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Json(req): Json<CreateClientRequest>,
) -> Result<Json<CreatedResponse>, PlatformError> {
    crate::shared::authorization_service::checks::require_anchor(&auth.0)?;

    let cmd = CreateClientCommand {
        name: req.name.clone(),
        identifier: req.identifier.clone(),
    };
    let ctx = ExecutionContext::from_auth(&auth.0);
    let use_case = CreateClientUseCase::new(state.client_repo.clone(), state.unit_of_work.clone());
    let event = use_case.run(cmd, ctx).await.into_result()?;

    Ok(Json(CreatedResponse::new(event.client_id)))
}

/// Update client
#[utoipa::path(
    put,
    path = "/{id}",
    tag = "sdk-clients",
    operation_id = "putApiSdkClientsById",
    params(
        ("id" = String, Path, description = "Client ID")
    ),
    request_body = UpdateClientRequest,
    responses(
        (status = 200, description = "Client updated", body = ClientResponse),
        (status = 403, description = "Insufficient permissions"),
        (status = 404, description = "Client not found")
    ),
    security(("bearer_auth" = []))
)]
async fn update_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Path(id): Path<String>,
    Json(req): Json<UpdateClientRequest>,
) -> Result<Json<ClientResponse>, PlatformError> {
    crate::shared::authorization_service::checks::require_anchor(&auth.0)?;

    let cmd = UpdateClientCommand {
        client_id: id.clone(),
        name: req.name,
    };
    let ctx = ExecutionContext::from_auth(&auth.0);
    let use_case = UpdateClientUseCase::new(state.client_repo.clone(), state.unit_of_work.clone());
    use_case.run(cmd, ctx).await.into_result()?;

    // Re-fetch updated entity for response
    let client = state.client_repo.find_by_id(&id).await?
        .ok_or_else(|| PlatformError::not_found("Client", &id))?;

    Ok(Json(client.into()))
}

/// Activate a client
#[utoipa::path(
    post,
    path = "/{id}/activate",
    tag = "sdk-clients",
    operation_id = "postApiSdkClientsByIdActivate",
    params(
        ("id" = String, Path, description = "Client ID")
    ),
    responses(
        (status = 200, description = "Client activated", body = StatusChangeResponse),
        (status = 403, description = "Insufficient permissions"),
        (status = 404, description = "Client not found")
    ),
    security(("bearer_auth" = []))
)]
async fn activate_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Path(id): Path<String>,
) -> Result<Json<StatusChangeResponse>, PlatformError> {
    crate::shared::authorization_service::checks::require_anchor(&auth.0)?;

    let cmd = ActivateClientCommand {
        client_id: id.clone(),
    };
    let ctx = ExecutionContext::from_auth(&auth.0);
    let use_case = ActivateClientUseCase::new(state.client_repo.clone(), state.unit_of_work.clone());
    use_case.run(cmd, ctx).await.into_result()?;

    tracing::info!(client_id = %id, principal_id = %auth.0.principal_id, "SDK: Client activated");

    Ok(Json(StatusChangeResponse {
        message: "Client activated".to_string(),
    }))
}

/// Suspend a client
#[utoipa::path(
    post,
    path = "/{id}/suspend",
    tag = "sdk-clients",
    operation_id = "postApiSdkClientsByIdSuspend",
    params(
        ("id" = String, Path, description = "Client ID")
    ),
    request_body = StatusChangeRequest,
    responses(
        (status = 200, description = "Client suspended", body = StatusChangeResponse),
        (status = 403, description = "Insufficient permissions"),
        (status = 404, description = "Client not found")
    ),
    security(("bearer_auth" = []))
)]
async fn suspend_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Path(id): Path<String>,
    Json(req): Json<StatusChangeRequest>,
) -> Result<Json<StatusChangeResponse>, PlatformError> {
    crate::shared::authorization_service::checks::require_anchor(&auth.0)?;

    let cmd = SuspendClientCommand {
        client_id: id.clone(),
        reason: req.reason.clone(),
    };
    let ctx = ExecutionContext::from_auth(&auth.0);
    let use_case = SuspendClientUseCase::new(state.client_repo.clone(), state.unit_of_work.clone());
    use_case.run(cmd, ctx).await.into_result()?;

    tracing::info!(
        client_id = %id,
        principal_id = %auth.0.principal_id,
        reason = %req.reason,
        "SDK: Client suspended"
    );

    Ok(Json(StatusChangeResponse {
        message: "Client suspended".to_string(),
    }))
}

// TODO: Rewrite to use DeactivateClientUseCase once it exists in the operations layer.
// Currently only Activate, Suspend, and Delete use cases are available.
/// Deactivate a client
#[utoipa::path(
    post,
    path = "/{id}/deactivate",
    tag = "sdk-clients",
    operation_id = "postApiSdkClientsByIdDeactivate",
    params(
        ("id" = String, Path, description = "Client ID")
    ),
    request_body = StatusChangeRequest,
    responses(
        (status = 200, description = "Client deactivated", body = StatusChangeResponse),
        (status = 403, description = "Insufficient permissions"),
        (status = 404, description = "Client not found")
    ),
    security(("bearer_auth" = []))
)]
async fn deactivate_sdk_client(
    State(state): State<SdkClientsState>,
    auth: Authenticated,
    Path(id): Path<String>,
    Json(req): Json<StatusChangeRequest>,
) -> Result<Json<StatusChangeResponse>, PlatformError> {
    crate::shared::authorization_service::checks::require_anchor(&auth.0)?;

    let mut client = state.client_repo.find_by_id(&id).await?
        .ok_or_else(|| PlatformError::not_found("Client", &id))?;

    client.deactivate(Some(req.reason.clone()));
    state.client_repo.update(&client).await?;

    tracing::info!(
        client_id = %id,
        principal_id = %auth.0.principal_id,
        reason = %req.reason,
        "SDK: Client deactivated"
    );

    Ok(Json(StatusChangeResponse {
        message: "Client deactivated".to_string(),
    }))
}

/// Create SDK clients router
pub fn sdk_clients_router(state: SdkClientsState) -> Router {
    Router::new()
        .route("/", get(list_sdk_clients).post(create_sdk_client))
        .route("/{id}", get(get_sdk_client).put(update_sdk_client))
        .route("/{id}/activate", post(activate_sdk_client))
        .route("/{id}/suspend", post(suspend_sdk_client))
        .route("/{id}/deactivate", post(deactivate_sdk_client))
        .with_state(state)
}
