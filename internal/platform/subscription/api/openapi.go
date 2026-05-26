package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/openapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
)

// OpenAPI registers the subscription subdomain's OpenAPI ops on doc.
// Paired with RegisterRoutes — keep in sync until the fused
// Mount(r, doc, state) helper lands (HANDOFF #26).
func OpenAPI(doc *openapi.Doc) {
	const tag = "subscriptions"
	listResp := map[string]any{"items": []subscription.Subscription{}}
	errResp := map[string]string{"code": "", "message": ""}

	doc.Op("GET", "/api/subscriptions", "listSubscriptions", "List subscriptions",
		openapi.Tag(tag),
		openapi.QueryParam("status", "filter by status (ACTIVE, PAUSED)", ""),
		openapi.QueryParam("clientId", "filter by client id", ""),
		openapi.Response(200, "Subscriptions matching filters", "SubscriptionList", listResp),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/subscriptions", "createSubscription", "Create a subscription",
		openapi.Tag(tag),
		openapi.RequestBody("CreateSubscriptionCommand", "Subscription to create", &operations.CreateCommand{}),
		openapi.Response(201, "Subscription created", "Subscription", &subscription.Subscription{}),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
		openapi.Response(422, "Validation error", "ErrorEnvelope", errResp),
	)

	doc.Op("GET", "/api/subscriptions/{id}", "getSubscription", "Get a subscription by id",
		openapi.Tag(tag),
		openapi.PathParam("id", "Subscription id (TSID)"),
		openapi.Response(200, "Subscription", "Subscription", &subscription.Subscription{}),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("PUT", "/api/subscriptions/{id}", "updateSubscription", "Update a subscription",
		openapi.Tag(tag),
		openapi.PathParam("id", "Subscription id"),
		openapi.RequestBody("UpdateSubscriptionCommand", "Fields to update", &operations.UpdateCommand{}),
		openapi.Response(204, "Updated", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("DELETE", "/api/subscriptions/{id}", "deleteSubscription", "Delete a subscription",
		openapi.Tag(tag),
		openapi.PathParam("id", "Subscription id"),
		openapi.Response(204, "Deleted", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/subscriptions/{id}/pause", "pauseSubscription", "Pause a subscription",
		openapi.Tag(tag),
		openapi.PathParam("id", "Subscription id"),
		openapi.Response(204, "Paused", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/subscriptions/{id}/resume", "resumeSubscription", "Resume a subscription",
		openapi.Tag(tag),
		openapi.PathParam("id", "Subscription id"),
		openapi.Response(204, "Resumed", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)
}
