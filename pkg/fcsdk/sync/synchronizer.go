package sync

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flowcatalyst/go-sdk/client"
)

// Synchronizer orchestrates per-category sync calls against a
// FlowCatalystClient. Reuse one Synchronizer across calls — the client
// is cheap to share.
//
// Categories are sync'd in a fixed order so references already exist by the
// time later categories arrive (the Rust and Java SDKs' order):
//
//  1. roles
//  2. event types
//  3. dispatch pools (before subscriptions: the platform drops a
//     subscription's unresolvable dispatchPoolCode)
//  4. connections
//  5. subscriptions (not sent, with an error, when connections failed:
//     their connection codes would not resolve)
//  6. processes
//  7. scheduled jobs (one request per job ClientID)
//  8. principals (after roles, which they name)
//  9. OpenAPI document
//  10. documentation pages
//
// A failure in one category does NOT abort the rest — the error lands
// on the per-category Result.Error field.
type Synchronizer struct {
	client *client.FlowCatalystClient
}

// NewSynchronizer wires a Synchronizer against an existing client.
func NewSynchronizer(c *client.FlowCatalystClient) *Synchronizer {
	return &Synchronizer{client: c}
}

// Sync pushes the DefinitionSet to the platform per Options. Returns
// the aggregate result; inspect Result.HasErrors / Result.Errors to
// see what failed.
func (s *Synchronizer) Sync(ctx context.Context, set *DefinitionSet, opts Options) *Result {
	out := &Result{ApplicationCode: set.ApplicationCode}
	app := set.ApplicationCode
	remove := opts.RemoveUnlisted

	if opts.SyncRoles && len(set.Roles) > 0 {
		out.Roles = s.runRoles(ctx, app, set.Roles, remove)
	}
	if opts.SyncEventTypes && len(set.EventTypes) > 0 {
		out.EventTypes = s.runEventTypes(ctx, app, set.EventTypes, remove)
	}
	if opts.SyncDispatchPools && len(set.DispatchPools) > 0 {
		out.DispatchPools = s.runDispatchPools(ctx, app, set.DispatchPools, remove)
	}
	if opts.SyncConnections && len(set.Connections) > 0 {
		out.Connections = s.runConnections(ctx, app, set.ClientID, set.Connections, remove)
	}
	if opts.SyncSubscriptions && len(set.Subscriptions) > 0 {
		if out.Connections != nil && out.Connections.IsError() {
			out.Subscriptions = &CategoryResult{Error: "subscriptions not synced because the connections sync failed: " + out.Connections.Error}
		} else {
			out.Subscriptions = s.runSubscriptions(ctx, app, set.ClientID, set.Subscriptions, remove)
		}
	}
	if opts.SyncProcesses && len(set.Processes) > 0 {
		out.Processes = s.runProcesses(ctx, app, set.Processes, remove)
	}
	if opts.SyncScheduledJobs && len(set.ScheduledJobs) > 0 {
		out.ScheduledJobs = s.runScheduledJobs(ctx, app, set.ScheduledJobs, remove)
	}
	if opts.SyncPrincipals && len(set.Principals) > 0 {
		out.Principals = s.runPrincipals(ctx, app, set.Principals, remove)
	}
	if opts.SyncOpenAPI && len(set.OpenAPISpec) > 0 {
		out.OpenAPI = s.runOpenAPI(ctx, app, set.OpenAPISpec)
	}
	if opts.SyncDocs && len(set.Docs) > 0 {
		out.Docs = s.runDocs(ctx, app, set.Docs)
	}
	return out
}

// SyncAll syncs several sets in order; one set's failures do not stop the
// next.
func (s *Synchronizer) SyncAll(ctx context.Context, sets []*DefinitionSet, opts Options) []*Result {
	out := make([]*Result, 0, len(sets))
	for _, set := range sets {
		out = append(out, s.Sync(ctx, set, opts))
	}
	return out
}

// ─── Per-category runners ────────────────────────────────────────────

func (s *Synchronizer) runRoles(ctx context.Context, app string, defs []RoleDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncRoleItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncRoleItem{
			Name:          d.Name,
			DisplayName:   d.DisplayName,
			Description:   d.Description,
			Permissions:   d.Permissions,
			ClientManaged: d.ClientManaged,
		})
	}
	res, err := s.client.Roles().Sync(ctx, app, &client.SyncRolesRequest{Roles: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

func (s *Synchronizer) runEventTypes(ctx context.Context, app string, defs []EventTypeDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncEventTypeItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncEventTypeItem{
			Code:        d.Code,
			Name:        d.Name,
			Description: d.Description,
		})
	}
	res, err := s.client.EventTypes().Sync(ctx, app, &client.SyncEventTypesRequest{EventTypes: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

func (s *Synchronizer) runSubscriptions(ctx context.Context, app, clientID string, defs []SubscriptionDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncSubscriptionItem, 0, len(defs))
	for _, d := range defs {
		bindings := make([]client.EventTypeBinding, 0, len(d.EventTypes))
		for _, b := range d.EventTypes {
			bindings = append(bindings, client.EventTypeBinding{EventTypeCode: b.EventTypeCode, Filter: b.Filter})
		}
		items = append(items, client.SyncSubscriptionItem{
			Code:             d.Code,
			Name:             d.Name,
			Description:      d.Description,
			Target:           d.Target,
			ConnectionID:     d.ConnectionID,
			ConnectionCode:   d.ConnectionCode,
			SharedConnection: d.SharedConnection,
			DispatchPoolCode: d.DispatchPoolCode,
			Mode:             d.Mode,
			MaxRetries:       d.MaxRetries,
			TimeoutSeconds:   d.TimeoutSeconds,
			DataOnly:         d.DataOnly,
			EventTypes:       bindings,
		})
	}
	res, err := s.client.Subscriptions().Sync(ctx, app, &client.SyncSubscriptionsRequest{ClientID: clientID, Subscriptions: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

func (s *Synchronizer) runDispatchPools(ctx context.Context, app string, defs []DispatchPoolDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncDispatchPoolItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncDispatchPoolItem{
			Code:        d.Code,
			Name:        d.Name,
			Description: d.Description,
			Concurrency: d.Concurrency,
			RateLimit:   d.RateLimit,
		})
	}
	res, err := s.client.DispatchPools().Sync(ctx, app, &client.SyncDispatchPoolsRequest{Pools: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

func principalItems(defs []PrincipalDefinition) []client.SyncPrincipalItem {
	items := make([]client.SyncPrincipalItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncPrincipalItem{
			Email:        d.Email,
			Name:         d.Name,
			Roles:        d.Roles,
			Active:       d.Active,
			PasswordHash: d.PasswordHash,
		})
	}
	return items
}

func (s *Synchronizer) runPrincipals(ctx context.Context, app string, defs []PrincipalDefinition, removeUnlisted bool) *CategoryResult {
	res, err := s.client.Principals().Sync(ctx, app, &client.SyncPrincipalsRequest{Principals: principalItems(defs)}, removeUnlisted)
	return toCategoryResult(res, err)
}

// SyncUsers performs an application-less user sync (POST /api/principals/sync):
// a declarative upsert keyed on email that needs no application. Use it to
// migrate users — including their existing password hashes via
// PrincipalDefinition.WithPasswordHash — without wrapping the call in an
// application. Pure upsert: it never strips roles from unlisted users.
func (s *Synchronizer) SyncUsers(ctx context.Context, defs []PrincipalDefinition) *CategoryResult {
	res, err := s.client.Principals().SyncUsers(ctx, &client.SyncPrincipalsRequest{Principals: principalItems(defs)})
	if err != nil {
		return toCategoryResult(nil, err)
	}
	// The application-less sync names its principals by email; SyncedCodes
	// carries them so callers read one field across every category.
	return toCategoryResult(&client.SyncResult{
		Created:             res.Created,
		Updated:             res.Updated,
		Deleted:             res.Deleted,
		SyncedCodes:         res.SyncedEmails,
		PasswordHashIgnored: res.PasswordHashIgnored,
	}, nil)
}

func (s *Synchronizer) runProcesses(ctx context.Context, app string, defs []ProcessDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncProcessInput, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncProcessInput{
			Code:        d.Code,
			Name:        d.Name,
			Description: d.Description,
			Body:        d.Body,
			DiagramType: d.DiagramType,
			Tags:        d.Tags,
		})
	}
	res, err := s.client.Processes().Sync(ctx, app, &client.SyncProcessesRequest{Processes: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

func (s *Synchronizer) runConnections(ctx context.Context, app, clientID string, defs []ConnectionDefinition, removeUnlisted bool) *CategoryResult {
	items := make([]client.SyncConnectionItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncConnectionItem{
			Code: d.Code, Name: d.Name, Description: d.Description, ExternalID: d.ExternalID,
		})
	}
	res, err := s.client.Connections().Sync(ctx, app, &client.SyncConnectionsRequest{ClientID: clientID, Connections: items}, removeUnlisted)
	return toCategoryResult(res, err)
}

// runScheduledJobs sends one request per job ClientID (first-seen order;
// the empty ClientID is the platform scope). The endpoint answers per-code
// lists; they are folded into counts, SyncedCodes being created + updated.
func (s *Synchronizer) runScheduledJobs(ctx context.Context, app string, defs []ScheduledJobDefinition, archiveUnlisted bool) *CategoryResult {
	var order []string
	groups := map[string][]client.SyncScheduledJobItem{}
	for _, d := range defs {
		if _, seen := groups[d.ClientID]; !seen {
			order = append(order, d.ClientID)
		}
		groups[d.ClientID] = append(groups[d.ClientID], client.SyncScheduledJobItem{
			Code: d.Code, Name: d.Name, Description: d.Description, Crons: d.Crons,
			Timezone: d.Timezone, Payload: d.Payload, Concurrent: d.Concurrent,
			TracksCompletion: d.TracksCompletion, TimeoutSeconds: d.TimeoutSeconds,
			DeliveryMaxAttempts: d.DeliveryMaxAttempts, TargetURL: d.TargetURL,
		})
	}
	out := &CategoryResult{}
	var errs []string
	for _, clientID := range order {
		res, err := s.client.ScheduledJobs().Sync(ctx, app, &client.SyncScheduledJobsRequest{
			ClientID: clientID, Jobs: groups[clientID], ArchiveUnlisted: archiveUnlisted,
		})
		if err != nil {
			if clientID != "" {
				errs = append(errs, "client "+clientID+": "+err.Error())
			} else {
				errs = append(errs, err.Error())
			}
			continue
		}
		out.Created += uint32(len(res.Created))
		out.Updated += uint32(len(res.Updated))
		out.Deleted += uint32(len(res.Archived))
		out.SyncedCodes = append(out.SyncedCodes, res.Created...)
		out.SyncedCodes = append(out.SyncedCodes, res.Updated...)
	}
	out.Error = strings.Join(errs, "; ")
	return out
}

// runOpenAPI publishes the document: one row created (first version),
// updated (a new version archived the prior one) or neither (unchanged);
// SyncedCodes holds the published version.
func (s *Synchronizer) runOpenAPI(ctx context.Context, app string, spec json.RawMessage) *CategoryResult {
	res, err := s.client.OpenAPI().Sync(ctx, app, spec)
	if err != nil {
		return &CategoryResult{Error: err.Error()}
	}
	out := &CategoryResult{SyncedCodes: []string{res.Version}}
	switch {
	case res.Unchanged:
	case res.ArchivedPriorVersion != "":
		out.Updated = 1
	default:
		out.Created = 1
	}
	return out
}

func (s *Synchronizer) runDocs(ctx context.Context, app string, defs []DocDefinition) *CategoryResult {
	items := make([]client.SyncDocInput, 0, len(defs))
	for _, d := range defs {
		items = append(items, client.SyncDocInput{Slug: d.Slug, Title: d.Title, Content: d.Content})
	}
	res, err := s.client.Docs().Sync(ctx, app, &client.SyncDocsRequest{Docs: items})
	return toCategoryResult(res, err)
}

func toCategoryResult(res *client.SyncResult, err error) *CategoryResult {
	if err != nil {
		return &CategoryResult{Error: err.Error()}
	}
	if res == nil {
		return &CategoryResult{}
	}
	return &CategoryResult{
		Created:             res.Created,
		Updated:             res.Updated,
		Deleted:             res.Deleted,
		SyncedCodes:         res.SyncedCodes,
		PasswordHashIgnored: res.PasswordHashIgnored,
		SkippedRoles:        res.SkippedRoles,
	}
}
