package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.common.api.ApiResponses.*;
import tech.flowcatalyst.platform.common.api.ApplicationResponses.*;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.ApplicationService;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.events.ApplicationActivated;
import tech.flowcatalyst.platform.application.events.ApplicationDeactivated;
import tech.flowcatalyst.platform.application.events.ApplicationDeleted;
import tech.flowcatalyst.platform.application.events.ApplicationDisabledForClient;
import tech.flowcatalyst.platform.application.events.ApplicationEnabledForClient;
import tech.flowcatalyst.platform.application.events.ApplicationUpdated;
import tech.flowcatalyst.platform.application.operations.DisableApplicationForClientCommand;
import tech.flowcatalyst.platform.application.operations.EnableApplicationForClientCommand;
import tech.flowcatalyst.platform.application.operations.activateapplication.ActivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.activateapplication.ActivateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.deactivateapplication.DeactivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deactivateapplication.DeactivateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.deleteapplication.DeleteApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deleteapplication.DeleteApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.provisionserviceaccount.ProvisionServiceAccountCommand;
import tech.flowcatalyst.platform.application.operations.provisionserviceaccount.ProvisionServiceAccountUseCase;
import tech.flowcatalyst.platform.application.operations.updateapplication.UpdateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.updateapplication.UpdateApplicationUseCase;
import tech.flowcatalyst.platform.application.events.ApplicationCreated;
import tech.flowcatalyst.platform.audit.AuditContext;

import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.platform.PlatformAdminPermissions;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;
import tech.flowcatalyst.platform.shared.TypedIdParam;

import java.util.List;
import java.util.Map;

/**
 * Admin API for managing applications in the platform ecosystem.
 *
 * Applications are the software products that users access. Each application
 * has a unique code that serves as the prefix for roles.
 */
@Path("/api/admin/applications")
@Tag(name = "BFF - Application Admin", description = "Application management endpoints")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@jakarta.transaction.Transactional
public class ApplicationAdminResource {

    @Inject
    ApplicationService applicationService;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @Inject
    ActivateApplicationUseCase activateApplicationUseCase;

    @Inject
    DeactivateApplicationUseCase deactivateApplicationUseCase;

    @Inject
    DeleteApplicationUseCase deleteApplicationUseCase;

    @Inject
    UpdateApplicationUseCase updateApplicationUseCase;

    @Inject
    CreateApplicationUseCase createApplicationUseCase;

    @Inject
    ProvisionServiceAccountUseCase provisionServiceAccountUseCase;

    // ========================================================================
    // Application CRUD
    // ========================================================================

    @GET
    @Operation(operationId = "listApplications", summary = "List all applications")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Applications retrieved successfully",
            content = @Content(schema = @Schema(implementation = ApplicationListResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response listApplications(
            @QueryParam("activeOnly") @DefaultValue("false") boolean activeOnly,
            @QueryParam("type") String type) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        List<Application> apps;
        if (type != null && !type.isBlank()) {
            try {
                Application.ApplicationType appType = Application.ApplicationType.valueOf(type.toUpperCase());
                apps = applicationRepo.findByType(appType, activeOnly);
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_TYPE", "Invalid type. Must be APPLICATION or INTEGRATION"))
                    .build();
            }
        } else {
            apps = activeOnly
                ? applicationService.findAllActive()
                : applicationService.findAll();
        }

        var items = apps.stream().map(ApplicationListItem::from).toList();
        return Response.ok(new ApplicationListResponse(items, apps.size())).build();
    }

    @GET
    @Path("/{id}")
    @Operation(operationId = "getApplication", summary = "Get application by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application found",
            content = @Content(schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response getApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        return applicationService.findById(id)
            .map(app -> Response.ok(ApplicationResponse.from(app)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new NotFoundResponse("application", id))
                .build());
    }

    @GET
    @Path("/by-code/{code}")
    @Operation(operationId = "getApplicationByCode", summary = "Get application by code")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application found",
            content = @Content(schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response getApplicationByCode(@PathParam("code") String code) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        return applicationService.findByCode(code)
            .map(app -> Response.ok(ApplicationResponse.from(app)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new NotFoundResponse("application", code))
                .build());
    }

    @POST
    @Operation(operationId = "createApplication", summary = "Create a new application")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Application created",
            content = @Content(schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "409", description = "Application code already exists",
            content = @Content(schema = @Schema(implementation = ConflictResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response createApplication(CreateApplicationRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_CREATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);

        // Parse application type
        Application.ApplicationType appType = Application.ApplicationType.APPLICATION;
        if (request.type != null && !request.type.isBlank()) {
            try {
                appType = Application.ApplicationType.valueOf(request.type.toUpperCase());
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_TYPE", "Invalid type. Must be APPLICATION or INTEGRATION"))
                    .build();
            }
        }

        // Create application via UseCase
        var command = new CreateApplicationCommand(
            request.code,
            request.name,
            request.description,
            request.defaultBaseUrl,
            request.iconUrl,
            request.website,
            request.logo,
            request.logoMimeType,
            appType,
            true  // provisionServiceAccount
        );
        Result<ApplicationCreated> result = createApplicationUseCase.execute(command, ctx);

        return switch (result) {
            case Result.Success<ApplicationCreated> s -> {
                // Fetch the created application
                Application app = applicationRepo.findByIdOptional(s.value().applicationId()).orElse(null);

                // Provision service account using UseCase
                var provisionCommand = new ProvisionServiceAccountCommand(app.id);
                var provisionResult = provisionServiceAccountUseCase.execute(provisionCommand, ctx);

                if (provisionResult.isSuccess()) {
                    // Build response including service account credentials
                    // Re-fetch app since provisioning updated it
                    app = applicationRepo.findByIdOptional(s.value().applicationId()).orElse(app);
                    var serviceAccount = new ServiceAccountInfo(
                        TypedId.Ops.serialize(EntityType.PRINCIPAL, provisionResult.principal().id),
                        provisionResult.principal().name,
                        new OAuthClientInfo(
                            TypedId.Ops.serialize(EntityType.OAUTH_CLIENT, provisionResult.oauthClient().id),
                            provisionResult.oauthClient().clientId,
                            provisionResult.clientSecret()  // Only available at creation time!
                        )
                    );
                    var response = ApplicationResponse.from(app).withServiceAccount(serviceAccount);

                    yield Response.status(Response.Status.CREATED)
                        .entity(response)
                        .build();
                } else {
                    // Service account provisioning failed, but app was created
                    var response = ApplicationResponse.from(app)
                        .withWarning("Application created but service account provisioning failed: " + provisionResult.error().message());
                    yield Response.status(Response.Status.CREATED)
                        .entity(response)
                        .build();
                }
            }
            case Result.Failure<ApplicationCreated> f -> {
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @PUT
    @Path("/{id}")
    @Operation(operationId = "updateApplication", summary = "Update an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application updated",
            content = @Content(schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response updateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id,
            UpdateApplicationRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new UpdateApplicationCommand(
            id,
            request.name,
            request.description,
            request.defaultBaseUrl,
            request.iconUrl,
            request.website,
            request.logo,
            request.logoMimeType
        );
        Result<ApplicationUpdated> result = updateApplicationUseCase.execute(command, ctx);

        return switch (result) {
            case Result.Success<ApplicationUpdated> s -> {
                // Fetch the updated application to return full details
                Application app = applicationRepo.findByIdOptional(id).orElse(null);
                yield Response.ok(ApplicationResponse.from(app)).build();
            }
            case Result.Failure<ApplicationUpdated> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application", id))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(operationId = "deleteApplication", summary = "Delete an application",
        description = "Permanently deletes an application. The application must be deactivated first.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application deleted",
            content = @Content(schema = @Schema(implementation = ApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Application must be deactivated first",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response deleteApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_DELETE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new DeleteApplicationCommand(id);
        Result<ApplicationDeleted> result = deleteApplicationUseCase.execute(command, ctx);

        return switch (result) {
            case Result.Success<ApplicationDeleted> s ->
                Response.ok(ApplicationStatusResponse.deleted(id)).build();
            case Result.Failure<ApplicationDeleted> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application", id))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "activateApplication", summary = "Activate an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application activated",
            content = @Content(schema = @Schema(implementation = ApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response activateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new ActivateApplicationCommand(id);
        Result<ApplicationActivated> result = activateApplicationUseCase.execute(command, ctx);

        return switch (result) {
            case Result.Success<ApplicationActivated> s ->
                Response.ok(ApplicationStatusResponse.activated(id)).build();
            case Result.Failure<ApplicationActivated> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application", id))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "deactivateApplication", summary = "Deactivate an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application deactivated",
            content = @Content(schema = @Schema(implementation = ApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response deactivateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new DeactivateApplicationCommand(id);
        Result<ApplicationDeactivated> result = deactivateApplicationUseCase.execute(command, ctx);

        return switch (result) {
            case Result.Success<ApplicationDeactivated> s ->
                Response.ok(ApplicationStatusResponse.deactivated(id)).build();
            case Result.Failure<ApplicationDeactivated> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application", id))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @POST
    @Path("/{id}/provision-service-account")
    @Operation(operationId = "provisionApplicationServiceAccount", summary = "Provision a service account for an existing application",
        description = "Creates a service account and OAuth client for an application that doesn't have one. " +
            "The client secret is only returned once and cannot be retrieved later.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Service account provisioned",
            content = @Content(schema = @Schema(implementation = ProvisionServiceAccountResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or service account already exists",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response provisionServiceAccount(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new ProvisionServiceAccountCommand(id);
        var provisionResult = provisionServiceAccountUseCase.execute(command, ctx);

        if (provisionResult.isSuccess()) {
            var serviceAccount = new ServiceAccountInfo(
                TypedId.Ops.serialize(EntityType.PRINCIPAL, provisionResult.principal().id),
                provisionResult.principal().name,
                new OAuthClientInfo(
                    TypedId.Ops.serialize(EntityType.OAUTH_CLIENT, provisionResult.oauthClient().id),
                    provisionResult.oauthClient().clientId,
                    provisionResult.clientSecret()  // Only available now!
                )
            );
            return Response.ok(new ProvisionServiceAccountResponse(
                "Service account provisioned",
                serviceAccount
            )).build();
        } else {
            var error = provisionResult.error();
            if (error instanceof UseCaseError.NotFoundError) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new NotFoundResponse("application", id))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(error.code(), error.message()))
                .build();
        }
    }

    // ========================================================================
    // Client Configuration
    // ========================================================================

    @GET
    @Path("/{id}/clients")
    @Operation(operationId = "getApplicationClientConfigs", summary = "Get client configurations for an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client configurations retrieved",
            content = @Content(schema = @Schema(implementation = ClientConfigListResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response getClientConfigs(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        Application app = applicationService.findById(id).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new NotFoundResponse("application", id))
                .build();
        }

        List<ApplicationClientConfig> configs = applicationService.getConfigsForApplication(id);
        var response = configs.stream()
            .map(config -> {
                Client client = clientRepo.findByIdOptional(config.clientId).orElse(null);
                return ClientConfigResponse.from(config, client, app);
            })
            .toList();

        return Response.ok(new ClientConfigListResponse(response, configs.size())).build();
    }

    @PUT
    @Path("/{id}/clients/{clientId}")
    @Operation(operationId = "configureApplicationForClient", summary = "Configure application for a specific client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client configuration updated",
            content = @Content(schema = @Schema(implementation = ClientConfigResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application or client not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response configureClient(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String applicationId,
            @TypedIdParam(EntityType.CLIENT) @PathParam("clientId") String clientId,
            ClientConfigRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);

        boolean enabled = request.enabled != null ? request.enabled : true;

        if (enabled) {
            var command = new EnableApplicationForClientCommand(
                applicationId,
                clientId,
                request.baseUrlOverride,
                request.websiteOverride,
                request.config
            );
            Result<ApplicationEnabledForClient> result = applicationService.enableForClient(ctx, command);

            return switch (result) {
                case Result.Success<ApplicationEnabledForClient> s -> {
                    var event = s.value();
                    Application app = applicationRepo.findByIdOptional(applicationId).orElse(null);
                    String effectiveBaseUrl = (request.baseUrlOverride != null && !request.baseUrlOverride.isBlank())
                        ? request.baseUrlOverride
                        : (app != null ? app.defaultBaseUrl : null);
                    String effectiveWebsite = (request.websiteOverride != null && !request.websiteOverride.isBlank())
                        ? request.websiteOverride
                        : (app != null ? app.website : null);

                    var response = ClientConfigResponse.enabled(
                        TypedId.Ops.serialize(EntityType.APP_CLIENT_CONFIG, event.configId()),
                        TypedId.Ops.serialize(EntityType.APPLICATION, event.applicationId()),
                        TypedId.Ops.serialize(EntityType.CLIENT, event.clientId()),
                        event.clientName(),
                        event.clientIdentifier(),
                        request.baseUrlOverride,
                        request.websiteOverride,
                        effectiveBaseUrl,
                        effectiveWebsite,
                        request.config
                    );
                    yield Response.ok(response).build();
                }
                case Result.Failure<ApplicationEnabledForClient> f -> {
                    if (f.error() instanceof UseCaseError.NotFoundError) {
                        yield Response.status(Response.Status.NOT_FOUND)
                            .entity(new NotFoundResponse("application or client", applicationId + "/" + clientId))
                            .build();
                    }
                    yield Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ErrorResponse(f.error().code(), f.error().message()))
                        .build();
                }
            };
        } else {
            var command = new DisableApplicationForClientCommand(applicationId, clientId);
            Result<ApplicationDisabledForClient> result = applicationService.disableForClient(ctx, command);

            return switch (result) {
                case Result.Success<ApplicationDisabledForClient> s -> {
                    var event = s.value();
                    Application app = applicationRepo.findByIdOptional(applicationId).orElse(null);

                    var response = ClientConfigResponse.disabled(
                        TypedId.Ops.serialize(EntityType.APP_CLIENT_CONFIG, event.configId()),
                        TypedId.Ops.serialize(EntityType.APPLICATION, event.applicationId()),
                        TypedId.Ops.serialize(EntityType.CLIENT, event.clientId()),
                        event.clientName(),
                        event.clientIdentifier(),
                        app != null ? app.defaultBaseUrl : null,
                        app != null ? app.website : null
                    );
                    yield Response.ok(response).build();
                }
                case Result.Failure<ApplicationDisabledForClient> f -> {
                    if (f.error() instanceof UseCaseError.NotFoundError) {
                        yield Response.status(Response.Status.NOT_FOUND)
                            .entity(new NotFoundResponse("application or client", applicationId + "/" + clientId))
                            .build();
                    }
                    yield Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ErrorResponse(f.error().code(), f.error().message()))
                        .build();
                }
            };
        }
    }

    @POST
    @Path("/{id}/clients/{clientId}/enable")
    @Operation(operationId = "enableApplicationForClient", summary = "Enable application for a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application enabled for client",
            content = @Content(schema = @Schema(implementation = ClientApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application or client not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response enableForClient(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String applicationId,
            @TypedIdParam(EntityType.CLIENT) @PathParam("clientId") String clientId) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new EnableApplicationForClientCommand(applicationId, clientId, null);
        Result<ApplicationEnabledForClient> result = applicationService.enableForClient(ctx, command);

        return switch (result) {
            case Result.Success<ApplicationEnabledForClient> s ->
                Response.ok(ClientApplicationStatusResponse.enabled(applicationId, clientId)).build();
            case Result.Failure<ApplicationEnabledForClient> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application or client", applicationId + "/" + clientId))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    @POST
    @Path("/{id}/clients/{clientId}/disable")
    @Operation(operationId = "disableApplicationForClient", summary = "Disable application for a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application disabled for client",
            content = @Content(schema = @Schema(implementation = ClientApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application or client not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response disableForClient(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String applicationId,
            @TypedIdParam(EntityType.CLIENT) @PathParam("clientId") String clientId) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        ExecutionContext ctx = ExecutionContext.create(principalId);
        var command = new DisableApplicationForClientCommand(applicationId, clientId);
        Result<ApplicationDisabledForClient> result = applicationService.disableForClient(ctx, command);

        return switch (result) {
            case Result.Success<ApplicationDisabledForClient> s ->
                Response.ok(ClientApplicationStatusResponse.disabled(applicationId, clientId)).build();
            case Result.Failure<ApplicationDisabledForClient> f -> {
                if (f.error() instanceof UseCaseError.NotFoundError) {
                    yield Response.status(Response.Status.NOT_FOUND)
                        .entity(new NotFoundResponse("application or client", applicationId + "/" + clientId))
                        .build();
                }
                yield Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse(f.error().code(), f.error().message()))
                    .build();
            }
        };
    }

    // ========================================================================
    // Roles for Application
    // ========================================================================

    @GET
    @Path("/{id}/roles")
    @Operation(operationId = "getApplicationRoles", summary = "Get all roles defined for this application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application roles info",
            content = @Content(schema = @Schema(implementation = ApplicationRolesResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ForbiddenResponse.class)))
    })
    public Response getApplicationRoles(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        return applicationService.findById(id)
            .map(app -> {
                // This would need PermissionRegistry injection to get actual roles
                // For now, return a placeholder
                return Response.ok(new ApplicationRolesResponse(
                    app.code,
                    "Use GET /api/admin/platform/roles?application=" + app.code + " to get roles"
                )).build();
            })
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new NotFoundResponse("application", id))
                .build());
    }

    // ========================================================================
    // DTOs and Response Mapping
    // ========================================================================

    public static class CreateApplicationRequest {
        public String code;
        public String name;
        public String description;
        public String defaultBaseUrl;
        public String iconUrl;
        public String website;
        public String logo;
        public String logoMimeType;
        public String type;  // "APPLICATION" or "INTEGRATION", defaults to APPLICATION
    }

    public static class UpdateApplicationRequest {
        public String name;
        public String description;
        public String defaultBaseUrl;
        public String iconUrl;
        public String website;
        public String logo;
        public String logoMimeType;
    }

    public static class ClientConfigRequest {
        public Boolean enabled;
        public String baseUrlOverride;
        public String websiteOverride;
        public Map<String, Object> config;
    }
}
