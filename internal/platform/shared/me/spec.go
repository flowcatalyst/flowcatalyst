package me

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// The /api/me routes are plain chi handlers, so they never reach the huma
// registry. RegisterSpec describes them to a huma API for tools/dump-spec
// ONLY. It must never be called from the server wiring: the routes are
// already mounted by RegisterRoutes and the stub handlers here do nothing.

type specNoInput struct{}

type specClientInput struct {
	ClientID string `path:"clientId"`
}

type (
	specWhoamiOutput  struct{ Body WhoamiResponse }
	specAppsOutput    struct{ Body MyApplicationsListResponse }
	specClientsOutput struct{ Body MyClientsListResponse }
	specClientOutput  struct{ Body MyClientResponse }
)

// RegisterSpec documents the /api/me routes on api. Spec only; see above.
func RegisterSpec(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "Who am I: the calling principal, its roles, permissions and accessible clients/applications",
		Tags:        []string{"me"},
	}, func(context.Context, *specNoInput) (*specWhoamiOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "listMyApplications",
		Method:      http.MethodGet,
		Path:        "/api/me/applications",
		Summary:     "Applications the calling principal can access",
		Tags:        []string{"me"},
	}, func(context.Context, *specNoInput) (*specAppsOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "listMyClients",
		Method:      http.MethodGet,
		Path:        "/api/me/clients",
		Summary:     "Clients the calling principal can access (anchors see all)",
		Tags:        []string{"me"},
	}, func(context.Context, *specNoInput) (*specClientsOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "getMyClient",
		Method:      http.MethodGet,
		Path:        "/api/me/clients/{clientId}",
		Summary:     "One client the calling principal can access",
		Tags:        []string{"me"},
	}, func(context.Context, *specClientInput) (*specClientOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "listMyClientApplications",
		Method:      http.MethodGet,
		Path:        "/api/me/clients/{clientId}/applications",
		Summary:     "Applications enabled for a client the calling principal can access",
		Tags:        []string{"me"},
	}, func(context.Context, *specClientInput) (*specAppsOutput, error) { return nil, nil })
}
