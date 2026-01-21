package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Pattern;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.principal.AnchorDomain;
import tech.flowcatalyst.platform.principal.AnchorDomainRepository;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;

/**
 * Admin API for managing anchor domains.
 *
 * Anchor domains are email domains whose users have god-mode access to all clients.
 * This is typically used for platform operators (e.g., flowcatalyst.tech).
 *
 * SECURITY:
 * - Adding/removing anchor domains is a highly privileged operation
 * - Only Super Admins should have access to these endpoints
 * - Changes affect all users from the domain immediately
 */
@Path("/api/admin/anchor-domains")
@Tag(name = "BFF - Anchor Domain Admin", description = "Manage anchor domains (platform operator domains)")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class AnchorDomainAdminResource {

    private static final Logger LOG = Logger.getLogger(AnchorDomainAdminResource.class);

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    AuditContext auditContext;

    // ==================== List & Get Operations ====================

    /**
     * List all anchor domains.
     */
    @GET
    @Operation(summary = "List all anchor domains",
        description = "Returns all configured anchor domains. Users from these domains have access to all clients.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of anchor domains",
            content = @Content(schema = @Schema(implementation = AnchorDomainListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listAnchorDomains() {

        auditContext.requirePrincipalId();

        List<AnchorDomain> domains = anchorDomainRepo.listAll();

        List<AnchorDomainDto> dtos = domains.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new AnchorDomainListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific anchor domain by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(summary = "Get anchor domain by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Anchor domain details",
            content = @Content(schema = @Schema(implementation = AnchorDomainDto.class))),
        @APIResponse(responseCode = "404", description = "Anchor domain not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getAnchorDomain(@PathParam("id") String id) {

        auditContext.requirePrincipalId();

        return anchorDomainRepo.findByIdOptional(id)
            .map(domain -> Response.ok(toDto(domain)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Anchor domain not found"))
                .build());
    }

    /**
     * Check if a domain is an anchor domain.
     */
    @GET
    @Path("/check/{domain}")
    @Operation(summary = "Check if domain is an anchor domain")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Domain check result",
            content = @Content(schema = @Schema(implementation = DomainCheckResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response checkDomain(@PathParam("domain") String domain) {

        auditContext.requirePrincipalId();

        String normalizedDomain = domain.toLowerCase().trim();
        boolean isAnchor = anchorDomainRepo.existsByDomain(normalizedDomain);

        // Count users from this domain
        long userCount = principalRepo.countByEmailDomain(normalizedDomain);

        return Response.ok(new DomainCheckResponse(normalizedDomain, isAnchor, userCount)).build();
    }

    // ==================== Create Operations ====================

    /**
     * Add a new anchor domain.
     */
    @POST
    @Operation(summary = "Add anchor domain",
        description = "Add a new anchor domain. Users from this domain will have access to all clients.")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Anchor domain created",
            content = @Content(schema = @Schema(implementation = AnchorDomainDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or domain already exists"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response createAnchorDomain(
            @Valid CreateAnchorDomainRequest request,
            @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();

        String normalizedDomain = request.domain().toLowerCase().trim();

        // Check for duplicate
        if (anchorDomainRepo.existsByDomain(normalizedDomain)) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("Anchor domain already exists: " + normalizedDomain))
                .build();
        }

        // Create new anchor domain
        AnchorDomain domain = new AnchorDomain();
        domain.id = TsidGenerator.generate();
        domain.domain = normalizedDomain;
        domain.createdAt = Instant.now();

        anchorDomainRepo.persist(domain);

        LOG.infof("Created anchor domain: %s by principal %s", normalizedDomain, principalId);

        // Count affected users
        long affectedUsers = principalRepo.countByEmailDomain(normalizedDomain);
        if (affectedUsers > 0) {
            LOG.infof("Anchor domain %s affects %d existing users who now have global access",
                normalizedDomain, affectedUsers);
        }

        return Response.status(Response.Status.CREATED)
            .entity(toDto(domain))
            .location(uriInfo.getAbsolutePathBuilder().path(String.valueOf(domain.id)).build())
            .build();
    }

    // ==================== Delete Operations ====================

    /**
     * Remove an anchor domain.
     */
    @DELETE
    @Path("/{id}")
    @Operation(summary = "Remove anchor domain",
        description = "Remove an anchor domain. Users from this domain will lose global access.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Anchor domain removed",
            content = @Content(schema = @Schema(implementation = DeleteAnchorDomainResponse.class))),
        @APIResponse(responseCode = "404", description = "Anchor domain not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response deleteAnchorDomain(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        AnchorDomain domain = anchorDomainRepo.findByIdOptional(id).orElse(null);
        if (domain == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Anchor domain not found"))
                .build();
        }

        String deletedDomain = domain.domain;

        // Count affected users before deletion
        long affectedUsers = principalRepo.countByEmailDomain(deletedDomain);

        anchorDomainRepo.delete(domain);

        LOG.infof("Deleted anchor domain: %s by principal %d (affected %d users)",
            deletedDomain, principalId, affectedUsers);

        return Response.ok(new DeleteAnchorDomainResponse(
            deletedDomain,
            affectedUsers,
            "Anchor domain removed. " + affectedUsers + " user(s) from this domain no longer have global access."
        )).build();
    }

    // ==================== Helper Methods ====================

    private AnchorDomainDto toDto(AnchorDomain domain) {
        // Count users from this domain
        long userCount = principalRepo.countByEmailDomain(domain.domain);

        return new AnchorDomainDto(
            TypedId.Ops.serialize(EntityType.ANCHOR_DOMAIN, domain.id),
            domain.domain,
            userCount,
            domain.createdAt
        );
    }

    // ==================== DTOs ====================

    public record AnchorDomainDto(
        String id,
        String domain,
        long userCount,
        Instant createdAt
    ) {}

    public record AnchorDomainListResponse(
        List<AnchorDomainDto> domains,
        int total
    ) {}

    public record CreateAnchorDomainRequest(
        @NotBlank(message = "Domain is required")
        @Pattern(regexp = "^[a-zA-Z0-9][a-zA-Z0-9.-]*\\.[a-zA-Z]{2,}$",
            message = "Invalid domain format")
        String domain
    ) {}

    public record DomainCheckResponse(
        String domain,
        boolean isAnchorDomain,
        long userCount
    ) {}

    public record DeleteAnchorDomainResponse(
        String domain,
        long affectedUsers,
        String message
    ) {}

    public record ErrorResponse(
        String error
    ) {}
}
