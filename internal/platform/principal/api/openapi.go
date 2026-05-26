package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/openapi"
)

// OpenAPI registers the principal subdomain's OpenAPI ops on doc.
// Paired with RegisterRoutes — keep in sync until the fused
// Mount(r, doc, state) helper lands (HANDOFF #26).
func OpenAPI(doc *openapi.Doc) {
	const tag = "principals"
	listResp := map[string]any{"items": []principal.Principal{}}
	errResp := map[string]string{"code": "", "message": ""}
	noContent := struct{}{}

	doc.Op("GET", "/api/principals", "listPrincipals", "List principals",
		openapi.Tag(tag),
		openapi.QueryParam("type", "filter by type (USER, SERVICE, ANCHOR)", ""),
		openapi.QueryParam("clientId", "filter by client id", ""),
		openapi.QueryParam("status", "filter by status (ACTIVE, INACTIVE)", ""),
		openapi.Response(200, "Principals matching filters", "PrincipalList", listResp),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/principals", "createPrincipal", "Create a principal",
		openapi.Tag(tag),
		openapi.RequestBody("CreatePrincipalCommand", "Principal to create", &operations.CreateCommand{}),
		openapi.Response(201, "Principal created", "Principal", &principal.Principal{}),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
		openapi.Response(422, "Validation error", "ErrorEnvelope", errResp),
	)

	doc.Op("GET", "/api/principals/{id}", "getPrincipal", "Get a principal by id",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id (TSID)"),
		openapi.Response(200, "Principal", "Principal", &principal.Principal{}),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("PUT", "/api/principals/{id}", "updatePrincipal", "Update a principal",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.RequestBody("UpdatePrincipalCommand", "Fields to update", &operations.UpdateCommand{}),
		openapi.Response(204, "Updated", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("DELETE", "/api/principals/{id}", "deletePrincipal", "Delete a principal",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.Response(204, "Deleted", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/principals/{id}/activate", "activatePrincipal", "Activate a principal",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.Response(204, "Activated", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/principals/{id}/deactivate", "deactivatePrincipal", "Deactivate a principal",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.Response(204, "Deactivated", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/principals/{id}/reset-password", "resetPrincipalPassword", "Reset a user principal's password",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.RequestBody("ResetPasswordCommand", "New password", &operations.ResetPasswordCommand{}),
		openapi.Response(204, "Password reset", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("PUT", "/api/principals/{id}/roles", "assignPrincipalRoles", "Replace a principal's role set",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.RequestBody("AssignRolesCommand", "Roles to assign", &operations.AssignRolesCommand{}),
		openapi.Response(204, "Roles assigned", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("PUT", "/api/principals/{id}/application-access", "assignPrincipalApplicationAccess", "Replace application-access grants",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.RequestBody("AssignApplicationAccessCommand", "Application access", &operations.AssignApplicationAccessCommand{}),
		openapi.Response(204, "Access updated", "", nil),
	)

	doc.Op("GET", "/api/principals/{id}/client-access", "listPrincipalClientAccess", "List client-access grants",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.Response(200, "Client access grants", "ClientAccessList", noContent),
	)

	doc.Op("POST", "/api/principals/{id}/client-access", "grantPrincipalClientAccess", "Grant client access",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.RequestBody("GrantClientAccessCommand", "Client to grant", &operations.GrantClientAccessCommand{}),
		openapi.Response(204, "Granted", "", nil),
	)

	doc.Op("DELETE", "/api/principals/{id}/client-access/{clientId}", "revokePrincipalClientAccess", "Revoke client access",
		openapi.Tag(tag),
		openapi.PathParam("id", "Principal id"),
		openapi.PathParam("clientId", "Client id"),
		openapi.Response(204, "Revoked", "", nil),
	)
}
