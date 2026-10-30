package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// lockCase is one client method exercised against the lock server. call
// returns the method's non-error result (nil for an error-only method).
//
//   - Every request must name a route+method in the lock, send only declared
//     query parameters and a body whose keys are declared properties with all
//     required ones present.
//   - A route the lock answers 204 must not return a decoded value.
//   - Any other route with a response body must return it, and the returned
//     value must keep every member the platform sent — unless discardsBody
//     says the method is deliberately error-only (message-only envelopes).
type lockCase struct {
	name         string
	call         func(ctx context.Context, c *client.FlowCatalystClient) (any, error)
	discardsBody bool
	// shape maps the returned value to the platform's body shape when the
	// method deliberately unwraps it (a bare slice for { roles: [...] }).
	shape func(got any) any
	// clientFiltered marks a method that narrows the platform's answer
	// client-side, so its result is not comparable to the canned body.
	clientFiltered bool
	// queryNotSent lists lock query parameters the method deliberately does
	// not expose (aliases of ones it does).
	queryNotSent []string
	// outsideLock marks routes that are real platform/router routes that the
	// huma-generated lock does not carry. The reason is mandatory.
	outsideLock string
}

func lockCases() []lockCase {
	raw := json.RawMessage(`{"a":1}`)
	return []lockCase{
		// ── Applications ──
		{name: "Applications.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().Create(ctx, &client.CreateApplicationRequest{Code: "a", Name: "A"})
		}},
		{name: "Applications.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().List(ctx, "APPLICATION", new(true))
		}},
		{name: "Applications.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().Get(ctx, "app_1")
		}},
		{name: "Applications.GetByCode", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().GetByCode(ctx, "orders")
		}},
		{name: "Applications.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Applications().Update(ctx, "app_1", &client.UpdateApplicationRequest{Name: new("N")})
		}},
		{name: "Applications.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Applications().Delete(ctx, "app_1")
		}},
		{name: "Applications.Activate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().Activate(ctx, "app_1")
		}},
		{name: "Applications.Deactivate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().Deactivate(ctx, "app_1")
		}},
		{name: "Applications.ProvisionServiceAccount", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().ProvisionServiceAccount(ctx, "app_1")
		}},
		{name: "Applications.LinkServiceAccount", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Applications().LinkServiceAccount(ctx, "app_1", &client.LinkServiceAccountRequest{ServiceAccountID: "sa_1", ServiceAccountCode: "c"})
		}},
		{name: "Applications.ListRoles", shape: unwrap("roles"), call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().ListRoles(ctx, "app_1")
		}},
		{name: "Applications.ListClients", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().ListClients(ctx, "app_1")
		}},
		{name: "Applications.GetClientConfig", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Applications().GetClientConfig(ctx, "app_1", "clt_1")
		}},
		{name: "Applications.EnableForClient", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Applications().EnableForClient(ctx, "app_1", "clt_1")
		}},
		{name: "Applications.DisableForClient", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Applications().DisableForClient(ctx, "app_1", "clt_1")
		}},

		// ── Clients ──
		{name: "Clients.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Create(ctx, &client.CreateClientRequest{Identifier: "acme", Name: "Acme"})
		}},
		{name: "Clients.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().List(ctx)
		}},
		{name: "Clients.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Get(ctx, "clt_1")
		}},
		{name: "Clients.GetByIdentifier", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().GetByIdentifier(ctx, "acme")
		}},
		{name: "Clients.Search", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Search(ctx, "acme")
		}},
		{name: "Clients.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Clients().Update(ctx, "clt_1", &client.UpdateClientRequest{Name: new("N")})
		}},
		{name: "Clients.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Clients().Delete(ctx, "clt_1")
		}},
		{name: "Clients.Activate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Activate(ctx, "clt_1")
		}},
		{name: "Clients.Suspend", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Suspend(ctx, "clt_1", &client.StatusChangeRequest{Reason: "r"})
		}},
		{name: "Clients.Deactivate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().Deactivate(ctx, "clt_1", &client.StatusChangeRequest{Reason: "r"})
		}},
		{name: "Clients.AddNote", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().AddNote(ctx, "clt_1", &client.AddNoteRequest{Category: "c", Text: "t"})
		}},
		{name: "Clients.ListApplications", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Clients().ListApplications(ctx, "clt_1")
		}},
		{name: "Clients.EnableApplication", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Clients().EnableApplication(ctx, "clt_1", "app_1")
		}},
		{name: "Clients.DisableApplication", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Clients().DisableApplication(ctx, "clt_1", "app_1")
		}},
		{name: "Clients.UpdateApplications", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Clients().UpdateApplications(ctx, "clt_1", &client.UpdateClientApplicationsRequest{EnabledApplicationIDs: []string{"app_1"}})
		}},

		// ── Connections ──
		{name: "Connections.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().Create(ctx, &client.CreateConnectionRequest{Code: "c", Name: "n", ServiceAccountID: "sa_1"})
		}},
		{name: "Connections.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().Get(ctx, "con_1")
		}},
		{name: "Connections.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().List(ctx, "clt_1", "ACTIVE")
		}},
		{name: "Connections.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Connections().Update(ctx, "con_1", &client.UpdateConnectionRequest{Name: "n"})
		}},
		{name: "Connections.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Connections().Delete(ctx, "con_1")
		}},
		{name: "Connections.Pause", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().Pause(ctx, "con_1")
		}},
		{name: "Connections.Activate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().Activate(ctx, "con_1")
		}},
		{name: "Connections.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Connections().Sync(ctx, "orders", &client.SyncConnectionsRequest{Connections: []client.SyncConnectionItem{{Code: "c", Name: "n"}}}, true)
		}},

		// ── Ingestions ──
		{name: "Ingestions.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().List(ctx, &client.ListIngestionsFilters{ApplicationCode: "app", ClientID: "clt_1", Q: "q", Status: "ACTIVE", Kind: "http", After: "cur", Limit: new(uint32(10))})
		}},
		{name: "Ingestions.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Create(ctx, &client.CreateIngestionRequest{Application: "app", Client: "acme", Code: "orders", Name: "Orders", Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget(), Live: new(true)})
		}},
		{name: "Ingestions.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Get(ctx, "ing_1")
		}},
		{name: "Ingestions.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Update(ctx, "ing_1", &client.UpdateIngestionRequest{ExpectedVersion: 1, Name: "Orders", Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget()})
		}},
		{name: "Ingestions.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Ingestions().Delete(ctx, "ing_1")
		}},
		{name: "Ingestions.Pause", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Pause(ctx, "ing_1")
		}},
		{name: "Ingestions.Resume", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Resume(ctx, "ing_1")
		}},
		{name: "Ingestions.ListVersions", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().ListVersions(ctx, "ing_1")
		}},
		{name: "Ingestions.SaveVersion", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().SaveVersion(ctx, "ing_1", &client.SaveIngestionVersionRequest{Name: "Orders", Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget(), Live: new(false)})
		}},
		{name: "Ingestions.MakeVersionLive", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().MakeVersionLive(ctx, "ing_1", "3")
		}},
		{name: "Ingestions.Definition", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Definition(ctx, "ing_1")
		}, discardsBody: true},
		{name: "Ingestions.History", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().History(ctx, "ing_1", new(uint32(10)), new(uint32(20)))
		}},
		{name: "Ingestions.Failures", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Failures(ctx, "ing_1", client.IngestionFailureFilters{After: "cur", Limit: new(uint32(10)), Category: "rejected", Undismissed: true})
		}},
		{name: "Ingestions.DismissFailure", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().DismissFailure(ctx, "ing_1", "igf_1")
		}},
		{name: "Ingestions.Secrets", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().Secrets(ctx, "ing_1")
		}},
		{name: "Ingestions.SourceKinds", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().SourceKinds(ctx)
		}},
		{name: "Ingestions.TestConnection", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().TestConnection(ctx, &client.TestIngestionRequest{Application: "app", Client: "acme", Definition: &client.IngestionDefinition{Name: "Orders", Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget()}})
		}},
		{name: "Ingestions.GetTest", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Ingestions().GetTest(ctx, "igt_1")
		}},
		// ── Ingestion sets ──
		{name: "IngestionSets.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().List(ctx)
		}},
		{name: "IngestionSets.Apply", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().Apply(ctx, "repo", &client.ApplyIngestionSetRequest{
				Items: []client.IngestionSetItem{{Application: "app", Client: "acme", Code: "orders", Name: "Orders", Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget()}},
				Files: []client.SetFile{{Path: "orders.yaml", Content: "x"}},
			}, new(true))
		}},
		{name: "IngestionSets.ListApplies", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().ListApplies(ctx, "repo")
		}},
		{name: "IngestionSets.GetApply", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().GetApply(ctx, "repo", "ina_1")
		}},
		{name: "IngestionSets.Reapply", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().Reapply(ctx, "repo", "ina_1", new(true))
		}},
		{name: "IngestionSets.Release", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.IngestionSets().Release(ctx, "repo")
		}},
		// ── Platform config ──
		{name: "PlatformConfig.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.PlatformConfig().Get(ctx, "orders", "smtp", "password", "clt_1")
		}},
		{name: "PlatformConfig.Set", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.PlatformConfig().Set(ctx, "orders", "smtp", "password", &client.SetConfigPropertyRequest{Value: "v", ValueType: "SECRET", Description: "d", ClientID: "clt_1"})
		}},
		{name: "PlatformConfig.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.PlatformConfig().Delete(ctx, "orders", "smtp", "password", "clt_1")
		}},
		{name: "PlatformConfig.RevealSecret", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.PlatformConfig().RevealSecret(ctx, "orders", "smtp", "password", "clt_1")
		}},

		// ── Dispatch pools ──
		{name: "DispatchPools.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.DispatchPools().Create(ctx, &client.CreateDispatchPoolRequest{Code: "p", Name: "P"})
		}},
		{name: "DispatchPools.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.DispatchPools().Get(ctx, "dp_1")
		}},
		{name: "DispatchPools.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.DispatchPools().List(ctx, "clt_1", "ACTIVE")
		}},
		{name: "DispatchPools.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.DispatchPools().Update(ctx, "dp_1", &client.UpdateDispatchPoolRequest{Name: new("n")})
		}},
		{name: "DispatchPools.Archive", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.DispatchPools().Archive(ctx, "dp_1")
		}},
		{name: "DispatchPools.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.DispatchPools().Delete(ctx, "dp_1")
		}},
		{name: "DispatchPools.Suspend", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.DispatchPools().Suspend(ctx, "dp_1")
		}},
		{name: "DispatchPools.Activate", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.DispatchPools().Activate(ctx, "dp_1")
		}},
		{name: "DispatchPools.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.DispatchPools().Sync(ctx, "orders", &client.SyncDispatchPoolsRequest{Pools: []client.SyncDispatchPoolItem{{Code: "p", Name: "P"}}}, true)
		}},

		// ── Event types ──
		{name: "EventTypes.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().Create(ctx, &client.CreateEventTypeRequest{Code: "a:b:c:d", Name: "n"})
		}},
		{name: "EventTypes.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().Get(ctx, "et_1")
		}},
		{name: "EventTypes.GetByCode", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().GetByCode(ctx, "a:b:c:d")
		}},
		{name: "EventTypes.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().List(ctx, &client.EventTypeFilters{Application: "orders", Subdomain: "s", Aggregate: "a", Status: "CURRENT", ClientID: "clt_1"})
		}},
		{name: "EventTypes.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.EventTypes().Update(ctx, "et_1", &client.UpdateEventTypeRequest{Name: "n"})
		}},
		{name: "EventTypes.AddSchemaVersion", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().AddSchemaVersion(ctx, "et_1", &client.AddSchemaVersionRequest{Version: "1.0", Schema: raw})
		}},
		{name: "EventTypes.Archive", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.EventTypes().Archive(ctx, "et_1")
		}},
		{name: "EventTypes.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.EventTypes().Sync(ctx, "orders", &client.SyncEventTypesRequest{EventTypes: []client.SyncEventTypeItem{{Code: "a:b:c:d", Name: "n"}}}, true)
		}},

		// ── Subscriptions ──
		{name: "Subscriptions.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Subscriptions().Create(ctx, &client.CreateSubscriptionRequest{Code: "s", Name: "n", Endpoint: "https://x"})
		}},
		{name: "Subscriptions.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Subscriptions().Get(ctx, "sub_1")
		}},
		{name: "Subscriptions.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Subscriptions().List(ctx, "clt_1", "ACTIVE")
		}},
		{name: "Subscriptions.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Subscriptions().Update(ctx, "sub_1", &client.UpdateSubscriptionRequest{Name: new("n")})
		}},
		{name: "Subscriptions.Pause", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Subscriptions().Pause(ctx, "sub_1")
		}},
		{name: "Subscriptions.Resume", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Subscriptions().Resume(ctx, "sub_1")
		}},
		{name: "Subscriptions.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Subscriptions().Delete(ctx, "sub_1")
		}},
		{name: "Subscriptions.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Subscriptions().Sync(ctx, "orders", &client.SyncSubscriptionsRequest{Subscriptions: []client.SyncSubscriptionItem{{Code: "s", Name: "n", Target: "https://x", EventTypes: []client.EventTypeBinding{{EventTypeCode: "a:b:c:d"}}}}}, true)
		}},

		// ── Processes ──
		{name: "Processes.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Processes().Create(ctx, &client.CreateProcessRequest{Code: "a:b:c", Name: "n"})
		}},
		{name: "Processes.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Processes().Get(ctx, "prc_1")
		}},
		{name: "Processes.GetByCode", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Processes().GetByCode(ctx, "a:b:c")
		}},
		{name: "Processes.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Processes().List(ctx, &client.ProcessFilters{Application: "orders", Subdomain: "s", Status: "CURRENT"})
		}},
		{name: "Processes.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Processes().Update(ctx, "prc_1", &client.UpdateProcessRequest{Name: new("n")})
		}},
		{name: "Processes.Archive", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Processes().Archive(ctx, "prc_1")
		}},
		{name: "Processes.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Processes().Delete(ctx, "prc_1")
		}},
		{name: "Processes.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Processes().Sync(ctx, "orders", &client.SyncProcessesRequest{Processes: []client.SyncProcessInput{{Code: "a:b:c", Name: "n"}}}, true)
		}},

		// ── Principals ──
		{name: "Principals.CreateUser", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().CreateUser(ctx, &client.CreateUserRequest{Email: "a@b.c", Name: "A"})
		}},
		{name: "Principals.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().List(ctx, &client.PrincipalFilters{ClientID: "clt_1", Type: "USER", Active: "true", Email: "a", Roles: "r", Page: new(uint32(1)), PageSize: new(uint32(5)), SortField: "name", SortOrder: "asc"})
		}},
		{name: "Principals.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().Get(ctx, "prn_1")
		}},
		{name: "Principals.FindByEmail", clientFiltered: true, queryNotSent: []string{"active", "clientId", "page", "pageSize", "roles", "sortField", "sortOrder", "type"}, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().FindByEmail(ctx, "a@b.c")
		}},
		{name: "Principals.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().Update(ctx, "prn_1", &client.UpdatePrincipalRequest{Name: new("n")})
		}},
		{name: "Principals.Activate", discardsBody: true, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Principals().Activate(ctx, "prn_1")
		}},
		{name: "Principals.Deactivate", discardsBody: true, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Principals().Deactivate(ctx, "prn_1")
		}},
		{name: "Principals.Roles", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().Roles(ctx, "prn_1")
		}},
		{name: "Principals.AddRole", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().AddRole(ctx, "prn_1", "orders:admin")
		}},
		{name: "Principals.RemoveRole", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().RemoveRole(ctx, "prn_1", "orders:admin")
		}},
		{name: "Principals.SetRoles", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().SetRoles(ctx, "prn_1", []string{"orders:admin"})
		}},
		{name: "Principals.ClientAccessGrants", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().ClientAccessGrants(ctx, "prn_1")
		}},
		{name: "Principals.GrantClientAccess", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().GrantClientAccess(ctx, "prn_1", "clt_1")
		}},
		{name: "Principals.RevokeClientAccess", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Principals().RevokeClientAccess(ctx, "prn_1", "clt_1")
		}},
		{name: "Principals.ResetPassword", discardsBody: true, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Principals().ResetPassword(ctx, "prn_1", &client.ResetPasswordRequest{NewPassword: "Password-123!"})
		}},
		{name: "Principals.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().Sync(ctx, "orders", &client.SyncPrincipalsRequest{Principals: []client.SyncPrincipalItem{{Email: "a@b.c", Name: "A", Active: true}}}, true)
		}},
		{name: "Principals.SyncUsers", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Principals().SyncUsers(ctx, &client.SyncPrincipalsRequest{Principals: []client.SyncPrincipalItem{{Email: "a@b.c", Name: "A", Active: true}}})
		}},

		// ── Roles / permissions ──
		{name: "Roles.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().List(ctx)
		}},
		{name: "Roles.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().Get(ctx, "orders:admin")
		}},
		{name: "Roles.GetByCode", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().GetByCode(ctx, "orders:admin")
		}},
		{name: "Roles.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().Create(ctx, &client.CreateRoleRequest{ApplicationCode: "orders", RoleName: "admin", DisplayName: "Admin"})
		}},
		{name: "Roles.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Roles().Update(ctx, "orders:admin", &client.UpdateRoleRequest{DisplayName: new("n")})
		}},
		{name: "Roles.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.Roles().Delete(ctx, "orders:admin")
		}},
		{name: "Roles.ListForApplication", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().ListForApplication(ctx, "app_1")
		}},
		{name: "Roles.GrantPermission", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().GrantPermission(ctx, "orders:admin", "orders:o:o:read")
		}},
		{name: "Roles.RevokePermission", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().RevokePermission(ctx, "orders:admin", "orders:o:o:read")
		}},
		{name: "Roles.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Roles().Sync(ctx, "orders", &client.SyncRolesRequest{Roles: []client.SyncRoleItem{{Name: "admin"}}}, true)
		}},
		{name: "Permissions.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Permissions().List(ctx)
		}},
		{name: "Permissions.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Permissions().Get(ctx, "orders:o:o:read")
		}},

		// ── Audit logs ──
		{name: "AuditLogs.List", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.AuditLogs().List(ctx, &client.AuditLogFilters{EntityType: "Principal", EntityID: "e", Operation: "o", PrincipalID: "p", ApplicationIDs: []string{"a"}, ClientIDs: []string{"c"}, After: "cur", PageSize: new(uint32(5))})
		}},
		{name: "AuditLogs.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.AuditLogs().Get(ctx, "aud_1")
		}},

		// ── Scheduled jobs ──
		{name: "ScheduledJobs.Create", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().Create(ctx, &client.CreateScheduledJobRequest{Code: "j", Name: "J", Crons: []string{"* * * * *"}})
		}},
		{name: "ScheduledJobs.List", queryNotSent: []string{"limit", "pageSize", "page_size"}, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().List(ctx, &client.ScheduledJobFilters{ClientID: "c", Status: "ACTIVE", Search: "s", Page: new(uint32(1)), Size: new(uint32(5))})
		}},
		{name: "ScheduledJobs.Get", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().Get(ctx, "sj_1")
		}},
		{name: "ScheduledJobs.GetByCode", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().GetByCode(ctx, "j", "clt_1")
		}},
		{name: "ScheduledJobs.Update", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().Update(ctx, "sj_1", &client.UpdateScheduledJobRequest{Name: new("n")})
		}},
		{name: "ScheduledJobs.Pause", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().Pause(ctx, "sj_1")
		}},
		{name: "ScheduledJobs.Resume", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().Resume(ctx, "sj_1")
		}},
		{name: "ScheduledJobs.Archive", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().Archive(ctx, "sj_1")
		}},
		{name: "ScheduledJobs.Delete", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().Delete(ctx, "sj_1")
		}},
		{name: "ScheduledJobs.Fire", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().Fire(ctx, "sj_1", &client.FireRequest{CorrelationID: "x"})
		}},
		{name: "ScheduledJobs.ListInstances", queryNotSent: []string{"limit", "pageSize", "page_size"}, call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().ListInstances(ctx, "sj_1", &client.InstanceFilters{Status: "COMPLETED", Page: new(uint32(1)), Size: new(uint32(5))})
		}},
		{name: "ScheduledJobs.GetInstance", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().GetInstance(ctx, "sji_1")
		}},
		{name: "ScheduledJobs.ListInstanceLogs", shape: field("logs"), call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().ListInstanceLogs(ctx, "sji_1")
		}},
		{name: "ScheduledJobs.LogForInstance", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().LogForInstance(ctx, "sji_1", &client.InstanceLogRequest{Message: "m"})
		}},
		{name: "ScheduledJobs.CompleteInstance", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return nil, c.ScheduledJobs().CompleteInstance(ctx, "sji_1", &client.InstanceCompleteRequest{Status: client.CompletionStatusSuccess})
		}},
		{name: "ScheduledJobs.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.ScheduledJobs().Sync(ctx, "orders", &client.SyncScheduledJobsRequest{Jobs: []client.SyncScheduledJobItem{{Code: "j", Name: "J", Crons: []string{"* * * * *"}}}})
		}},

		// ── Docs / OpenAPI ──
		{name: "Docs.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Docs().Sync(ctx, "orders", &client.SyncDocsRequest{Docs: []client.SyncDocInput{{Slug: "s", Content: "# t"}}})
		}},
		{name: "OpenAPI.Sync", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.OpenAPI().Sync(ctx, "orders", raw)
		}},

		// ── /api/me/*: spec-only stubs in the lock since the x-sdk tagging (21f9734) ──
		{name: "Me.Clients", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Me().Clients(ctx)
		}},
		{name: "Me.Client", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Me().Client(ctx, "clt_1")
		}},
		{name: "Me.ClientApplications", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Me().ClientApplications(ctx, "clt_1")
		}},

		// ── Outside the lock ──
		{name: "Router.InPipeline", outsideLock: "served by the message router (internal/router/api), a separate OpenAPI document", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Router().InPipeline(ctx, "m1")
		}},
		{name: "Router.InPipelineBatch", outsideLock: "served by the message router, see Router.InPipeline", call: func(ctx context.Context, c *client.FlowCatalystClient) (any, error) {
			return c.Router().InPipelineBatch(ctx, []string{"m1"})
		}},
	}
}

// unwrap maps a wrapper struct (or a bare slice) back to { key: slice }.
func unwrap(key string) func(any) any {
	return func(got any) any {
		if m, ok := got.(map[string]any); ok {
			return map[string]any{key: m[key]}
		}
		return map[string]any{key: got}
	}
}

// field maps a wrapper struct to one member (the bare array the platform sends).
func field(key string) func(any) any {
	return func(got any) any {
		m, _ := got.(map[string]any)
		return m[key]
	}
}

// isNilResult reports whether the method returned nothing (untyped nil or a
// nil pointer).
func isNilResult(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	if k := rv.Kind(); k == reflect.Pointer || k == reflect.Map || k == reflect.Slice || k == reflect.Interface {
		return rv.IsNil()
	}
	return false
}

func TestClientMethodsConformToTheLock(t *testing.T) {
	for _, tc := range lockCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.outsideLock != "" {
				return // exercised by TestOutsideLockRoutesStayOutside
			}
			ls := newLockServer(t)
			c := client.New(ls.URL, client.WithRetry(1, 0))

			res, err := tc.call(context.Background(), c)
			call := ls.last()
			require.NotNil(t, call, "the method made no request")
			require.Empty(t, call.Problems, "%s %s", call.Method, call.Path)
			require.NoError(t, err, "%s %s", call.Method, call.Tpl)

			assertEveryQueryParamReachable(t, ls, call, tc.queryNotSent)

			if call.Status == http.StatusNoContent || call.ResponseSchema == nil {
				assert.True(t, isNilResult(res),
					"%s %s answers %d with no body, but the method returns %T: it must return only an error",
					call.Method, call.Tpl, call.Status, res)
				return
			}
			if tc.discardsBody {
				return
			}
			require.False(t, isNilResult(res),
				"%s %s answers %d with a body; the method discards it", call.Method, call.Tpl, call.Status)
			if tc.clientFiltered {
				return
			}
			b, err := json.Marshal(res)
			require.NoError(t, err)
			var got any
			require.NoError(t, json.Unmarshal(b, &got))
			if tc.shape != nil {
				got = tc.shape(got)
			}
			assert.Empty(t, missingKeys(call.Response, got, "$"),
				"%s %s: members the platform sends that the returned %T drops", call.Method, call.Tpl, res)
			assert.Empty(t, missingKeys(got, call.Response, "$"),
				"%s %s: members the returned %T invents (the platform never sends them)", call.Method, call.Tpl, res)
		})
	}
}

func TestOutsideLockRoutesStayOutside(t *testing.T) {
	spec := loadLock(t)
	for _, tc := range lockCases() {
		if tc.outsideLock == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := newRecordingServer(t, &gotMethod, &gotPath)
			c := client.New(srv.URL, client.WithRetry(1, 0))
			_, _ = tc.call(context.Background(), c)
			require.NotEmpty(t, gotPath, "the method made no request")
			_, _, err := spec.operation(gotMethod, gotPath)
			assert.Error(t, err, "%s %s is now in the lock: drop the outsideLock exemption (%s)", gotMethod, gotPath, tc.outsideLock)
		})
	}
}

// TestEveryResourceMethodIsRegistered keeps the table honest: a method added
// to a resource without a lockCase fails here.
func TestEveryResourceMethodIsRegistered(t *testing.T) {
	c := client.New("http://unused")
	resources := map[string]any{
		"Applications":   c.Applications(),
		"Clients":        c.Clients(),
		"Connections":    c.Connections(),
		"DispatchPools":  c.DispatchPools(),
		"PlatformConfig": c.PlatformConfig(),
		"EventTypes":     c.EventTypes(),
		"Subscriptions":  c.Subscriptions(),
		"Processes":      c.Processes(),
		"Principals":     c.Principals(),
		"Roles":          c.Roles(),
		"Permissions":    c.Permissions(),
		"AuditLogs":      c.AuditLogs(),
		"ScheduledJobs":  c.ScheduledJobs(),
		"Docs":           c.Docs(),
		"Ingestions":     c.Ingestions(),
		"IngestionSets":  c.IngestionSets(),
		"OpenAPI":        c.OpenAPI(),
		"Me":             c.Me(),
		"Router":         c.Router(),
	}
	registered := map[string]bool{}
	for _, tc := range lockCases() {
		registered[tc.name] = true
	}
	var missing []string
	for name, r := range resources {
		rt := reflect.TypeOf(r)
		for m := range rt.Methods() {
			full := name + "." + m.Name
			if !registered[full] {
				missing = append(missing, full)
			}
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing, "resource methods with no lockCase: %s", strings.Join(missing, ", "))
}

// assertEveryQueryParamReachable fails when the lock declares a query
// parameter the call never sent. Each case passes every filter it exposes, so
// a missing one is a filter the client cannot express.
func assertEveryQueryParamReachable(t *testing.T, ls *lockServer, call *lockCall, notSent []string) {
	t.Helper()
	op, _, err := loadLock(t).operation(call.Method, call.Path)
	require.NoError(t, err)
	skip := map[string]bool{}
	for _, n := range notSent {
		skip[n] = true
	}
	var missing []string
	for _, p := range asSlice(op["parameters"]) {
		pm := asMap(p)
		if pm["in"] != "query" {
			continue
		}
		name, _ := pm["name"].(string)
		if _, sent := call.Query[name]; !sent && !skip[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing, "%s %s: query parameters the lock declares but the client cannot send", call.Method, call.Tpl)
}
