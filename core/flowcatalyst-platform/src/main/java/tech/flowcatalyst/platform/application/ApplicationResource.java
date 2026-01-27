package tech.flowcatalyst.platform.application;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.application.events.*;
import tech.flowcatalyst.platform.application.operations.activateapplication.ActivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deactivateapplication.DeactivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deleteapplication.DeleteApplicationCommand;
import tech.flowcatalyst.platform.application.operations.updateapplication.UpdateApplicationCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;
import tech.flowcatalyst.platform.shared.TypedIdParam;

import java.util.List;
import java.util.Map;

@Path("/api/applications")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Applications", description = "Manage platform applications")
public class ApplicationResource {

    @Inject
    ApplicationOperations applicationOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @GET
    @Operation(operationId = "listApps", summary = "List all applications", description = "Returns all applications")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "List of applications",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationListResponse.class))
        )
    })
    public Response listApplications(@QueryParam("activeOnly") @DefaultValue("false") boolean activeOnly) {
        List<Application> applications = activeOnly
            ? applicationOperations.findAllActive()
            : applicationOperations.findAll();

        List<ApplicationResponse> responses = applications.stream()
            .map(ApplicationResponse::from)
            .toList();

        return Response.ok(new ApplicationListResponse(responses)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(operationId = "getApp", summary = "Get application by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid application ID format"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response getApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        return applicationOperations.findById(id)
            .map(app -> Response.ok(ApplicationResponse.from(app)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + id))
                .build());
    }

    @GET
    @Path("/code/{code}")
    @Operation(operationId = "getAppByCode", summary = "Get application by code")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response getApplicationByCode(@PathParam("code") String code) {
        return applicationOperations.findByCode(code)
            .map(app -> Response.ok(ApplicationResponse.from(app)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + code))
                .build());
    }

    @POST
    @Operation(operationId = "createApp", summary = "Create a new application")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Application created",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "409", description = "Business rule violation")
    })
    public Response createApplication(CreateApplicationRequest request) {
        ExecutionContext context = createExecutionContext();

        CreateApplicationCommand command = new CreateApplicationCommand(
            request.code(),
            request.name(),
            request.description(),
            request.defaultBaseUrl(),
            request.iconUrl()
        );

        Result<ApplicationCreated> result = applicationOperations.createApplication(command, context);

        return switch (result) {
            case Result.Success<ApplicationCreated> s -> {
                Application app = applicationOperations.findById(s.value().applicationId())
                    .orElseThrow();
                yield Response.status(201).entity(ApplicationResponse.from(app)).build();
            }
            case Result.Failure<ApplicationCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @PUT
    @Path("/{id}")
    @Operation(operationId = "updateApp", summary = "Update an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application updated",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or ID format"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response updateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id,
            UpdateApplicationRequest request) {
        ExecutionContext context = createExecutionContext();

        UpdateApplicationCommand command = new UpdateApplicationCommand(
            id,
            request.name(),
            request.description(),
            request.defaultBaseUrl(),
            request.iconUrl()
        );

        Result<ApplicationUpdated> result = applicationOperations.updateApplication(command, context);

        return switch (result) {
            case Result.Success<ApplicationUpdated> s -> {
                Application app = applicationOperations.findById(s.value().applicationId())
                    .orElseThrow();
                yield Response.ok(ApplicationResponse.from(app)).build();
            }
            case Result.Failure<ApplicationUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "activateApp", summary = "Activate an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application activated",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Application already active or invalid ID format"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response activateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        ExecutionContext context = createExecutionContext();

        ActivateApplicationCommand command = new ActivateApplicationCommand(id);

        Result<ApplicationActivated> result = applicationOperations.activateApplication(command, context);

        return switch (result) {
            case Result.Success<ApplicationActivated> s -> {
                Application app = applicationOperations.findById(s.value().applicationId())
                    .orElseThrow();
                yield Response.ok(ApplicationResponse.from(app)).build();
            }
            case Result.Failure<ApplicationActivated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "deactivateApp", summary = "Deactivate an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application deactivated",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationResponse.class))),
        @APIResponse(responseCode = "400", description = "Application already deactivated or invalid ID format"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response deactivateApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        ExecutionContext context = createExecutionContext();

        DeactivateApplicationCommand command = new DeactivateApplicationCommand(id);

        Result<ApplicationDeactivated> result = applicationOperations.deactivateApplication(command, context);

        return switch (result) {
            case Result.Success<ApplicationDeactivated> s -> {
                Application app = applicationOperations.findById(s.value().applicationId())
                    .orElseThrow();
                yield Response.ok(ApplicationResponse.from(app)).build();
            }
            case Result.Failure<ApplicationDeactivated> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(operationId = "deleteApp", summary = "Delete an application")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Application deleted"),
        @APIResponse(responseCode = "400", description = "Cannot delete active application or application with configurations, or invalid ID format"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response deleteApplication(
            @TypedIdParam(EntityType.APPLICATION) @PathParam("id") String id) {
        ExecutionContext context = createExecutionContext();

        DeleteApplicationCommand command = new DeleteApplicationCommand(id);

        Result<ApplicationDeleted> result = applicationOperations.deleteApplication(command, context);

        return switch (result) {
            case Result.Success<ApplicationDeleted> s -> Response.noContent().build();
            case Result.Failure<ApplicationDeleted> f -> mapErrorToResponse(f.error());
        };
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private ExecutionContext createExecutionContext() {
        return ExecutionContext.from(tracingContext, auditContext.requirePrincipalId());
    }

    private Response mapErrorToResponse(UseCaseError error) {
        Response.Status status = switch (error) {
            case UseCaseError.ValidationError v -> Response.Status.BAD_REQUEST;
            case UseCaseError.NotFoundError n -> Response.Status.NOT_FOUND;
            case UseCaseError.BusinessRuleViolation b -> Response.Status.CONFLICT;
            case UseCaseError.ConcurrencyError c -> Response.Status.CONFLICT;
            case UseCaseError.AuthorizationError a -> Response.Status.FORBIDDEN;
        };

        return Response.status(status)
            .entity(new ErrorResponse(error.code(), error.message(), error.details()))
            .build();
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record ApplicationListResponse(List<ApplicationResponse> items) {}

    public record ApplicationResponse(
        String id,
        String code,
        String name,
        String description,
        String defaultBaseUrl,
        String iconUrl,
        boolean active,
        String createdAt,
        String updatedAt
    ) {
        public static ApplicationResponse from(Application app) {
            return new ApplicationResponse(
                app.id != null ? TypedId.Ops.serialize(EntityType.APPLICATION, app.id) : null,
                app.code,
                app.name,
                app.description,
                app.defaultBaseUrl,
                app.iconUrl,
                app.active,
                app.createdAt != null ? app.createdAt.toString() : null,
                app.updatedAt != null ? app.updatedAt.toString() : null
            );
        }
    }

    public record CreateApplicationRequest(
        String code,
        String name,
        String description,
        String defaultBaseUrl,
        String iconUrl
    ) {}

    public record UpdateApplicationRequest(
        String name,
        String description,
        String defaultBaseUrl,
        String iconUrl
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
