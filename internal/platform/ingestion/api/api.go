// Package api wires the HTTP routes for the ingestion subdomain via huma.
package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// State bundles the dependencies.
type State struct {
	Repo     *ingestion.Repository
	Resolver ingestion.Resolver
	Apps     AppLookup
	Clients  ClientLookup
	Audit    *audit.Repository
	UoW      *usecasepgx.UnitOfWork
	// Configs is where the config entries an ingestion's source names are
	// written (docs/ingestion-design.md section 13); reads go through Repo.
	Configs *platformconfig.Repository
	// SetArchive keeps what each apply of a set was given (file:// or s3://,
	// as for function artifacts).
	SetArchive artifact.Store
}

// AppLookup is the slice of the application repository the responses need.
type AppLookup interface {
	FindByID(ctx context.Context, id string) (*application.Application, error)
}

// ClientLookup is the slice of the client repository a client reference and
// the responses need.
type ClientLookup interface {
	FindByID(ctx context.Context, id string) (*client.Client, error)
	FindByIdentifier(ctx context.Context, identifier string) (*client.Client, error)
}

const tag = "ingestions"

// Register mounts the ingestion endpoints. The server mounts them only when
// FC_INGESTION_ENABLED is on; the spec generator always registers them.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listIngestions", "/api/ingestions", "List ingestions", s.list)
	apiroute.Post(g, "createIngestion", "/api/ingestions", "Create an ingestion", http.StatusCreated, s.create)
	apiroute.Get(g, "getIngestion", "/api/ingestions/{id}", "Get an ingestion by id", s.getByID)
	apiroute.Put(g, "updateIngestion", "/api/ingestions/{id}", "Save a definition as the next version: made live with promote, a draft awaiting promote without it; a definition an existing version says is not saved again", http.StatusOK, s.update)
	apiroute.Delete(g, "deleteIngestion", "/api/ingestions/{id}", "Delete an ingestion", http.StatusNoContent, s.delete)
	apiroute.Post(g, "pauseIngestion", "/api/ingestions/{id}/pause", "Pause an ingestion", http.StatusOK, s.pause)
	apiroute.Post(g, "resumeIngestion", "/api/ingestions/{id}/resume", "Resume a paused ingestion", http.StatusOK, s.resume)
	apiroute.Get(g, "listIngestionVersions", "/api/ingestions/{id}/versions", "List an ingestion's saved versions", s.listVersions)
	apiroute.Post(g, "saveIngestionVersion", "/api/ingestions/{id}/versions", "Save a definition as the next version (a definition an existing version says is not saved again)", http.StatusCreated, s.saveVersion)
	apiroute.Post(g, "makeIngestionVersionLive", "/api/ingestions/{id}/versions/{version}/live", "Make a saved version the live one", http.StatusOK, s.makeLive)
	registerDefinition(g, s)
	registerConfig(g, s)
	apiroute.Get(g, "listIngestionHistory", "/api/ingestions/{id}/history", "List the audit history of an ingestion", s.history)
	apiroute.Get(g, "listIngestionSecrets", "/api/ingestions/{id}/secrets", "List the secrets the live definition binds and when each runtime last fetched them (names only, never values)", s.secrets)
	apiroute.Get(g, "listIngestionFailures", "/api/ingestions/{id}/failures", "List the failures runtimes reported for an ingestion", s.failures)
	apiroute.Post(g, "dismissIngestionFailure", "/api/ingestions/{id}/failures/{failureId}/dismiss", "Dismiss a failure (a notice stops showing on the ingestion)", http.StatusOK, s.dismissFailure)
	apiroute.Get(g, "listIngestionSourceKinds", "/api/ingestion-source-kinds", "List the source kinds a definition can use", s.sourceKinds)
	registerTests(g, s)
	registerSets(g, s)
}

// resolveClient canonicalises a request's client reference (the clt_ id or the
// client's identifier) to the client id. An unknown client is a 404, so a
// reference naming no client never reaches a query or an insert.
func (s *State) resolveClient(ctx context.Context, ref string) (ids.ClientID, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", usecase.Validation("CLIENT_REQUIRED", "client is required")
	}
	c, err := s.Clients.FindByID(ctx, ref)
	if err != nil {
		return "", usecase.Internal("REPO", "client lookup failed", err)
	}
	if c == nil {
		// Identifiers are stored lower-case.
		c, err = s.Clients.FindByIdentifier(ctx, strings.ToLower(ref))
		if err != nil {
			return "", usecase.Internal("REPO", "client lookup failed", err)
		}
	}
	if c == nil {
		return "", usecase.NotFound("CLIENT_NOT_FOUND", "Client not found: "+ref)
	}
	return c.ID, nil
}

// find loads the ingestion a by-id read acts on and checks the caller reaches it.
func (s *State) find(ctx context.Context, id string) (*ingestion.Ingestion, error) {
	i, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return nil, operations.NotFound(id)
	}
	if err := ingestion.CheckReach(auth.FromContext(ctx), i); err != nil {
		return nil, err
	}
	return i, nil
}

// shown is res as the caller (taken from ctx) may see it: the settings of the
// config entry a source names are shown only to a caller who can update
// ingestions, so anyone else gets the status and the reference as it was
// given, which the warnings and everything else built from res then follow.
func (s *State) shown(ctx context.Context, res ingestion.Resolved, reference ingestion.Definition) ingestion.Resolved {
	if canSeeEntries(ctx) {
		return res
	}
	return res.Redacted(reference)
}

// canSeeEntries is the one decision on whether the caller reads what a config
// entry holds.
func canSeeEntries(ctx context.Context) bool {
	return auth.CanUpdateIngestions(auth.FromContext(ctx)) == nil
}

// respond builds the response for one ingestion, reading its application's
// code and its client's identifier.
func (s *State) respond(ctx context.Context, i *ingestion.Ingestion) (*apicommon.Out[IngestionResponse], error) {
	app, err := s.Apps.FindByID(ctx, string(i.ApplicationID))
	if err != nil || app == nil {
		return nil, usecase.Internal("REPO", "application lookup failed", err)
	}
	c, err := s.Clients.FindByID(ctx, string(i.ClientID))
	if err != nil || c == nil {
		return nil, usecase.Internal("REPO", "client lookup failed", err)
	}
	body, err := fromEntity(i, app.Code, c.Identifier)
	if err != nil {
		return nil, usecase.Internal("MARSHAL", "ingestion does not render", err)
	}
	res, err := s.Repo.Resolve(ctx, app.Code, i.ClientID, i.Code, i.Definition)
	if err != nil {
		return nil, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	withConfig(&body, s.shown(ctx, res, i.Definition))
	notice, err := s.runtimeNotices(ctx)
	if err != nil {
		return nil, err
	}
	body.RuntimeNotice = notice(i)
	notices, err := s.Repo.NoticesOf(ctx, []string{i.ID})
	if err != nil {
		return nil, usecase.Internal("REPO", "notices failed", err)
	}
	withNotices(&body, notices[i.ID])
	return &apicommon.Out[IngestionResponse]{Body: body}, nil
}

// reload re-reads an ingestion an operation just changed and responds with it.
func (s *State) reload(ctx context.Context, id string) (*apicommon.Out[IngestionResponse], error) {
	i, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return nil, operations.NotFound(id)
	}
	return s.respond(ctx, i)
}

type listInput struct {
	ApplicationCode string `query:"applicationCode" doc:"Only the ingestions of this application"`
	ClientID        string `query:"clientId" doc:"Only the ingestions of this client: its id or identifier"`
	Q               string `query:"q" doc:"Matches the code or name, ignoring case"`
	Status          string `query:"status" enum:"ACTIVE,PAUSED"`
	Kind            string `query:"kind" doc:"A source kind; for an extension's kind, its type"`
	After           string `query:"after" doc:"Cursor from a previous page's nextCursor"`
	Limit           int    `query:"limit" minimum:"0" maximum:"200" doc:"Page size (default 50, at most 200)"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[IngestionListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIngestions(ac); err != nil {
		return nil, err
	}
	f := ingestion.ListFilter{
		ApplicationCode: apicommon.OptStr(in.ApplicationCode),
		Q:               apicommon.OptStr(in.Q),
		Status:          apicommon.OptStr(in.Status),
		Kind:            apicommon.OptStr(in.Kind),
		After:           in.After,
		Limit:           in.Limit,
	}
	f.AccessibleClientIDs, f.AccessibleApplicationIDs = ingestion.ListReach(ac)
	if in.ClientID != "" {
		cid, err := s.resolveClient(ctx, in.ClientID)
		if err != nil {
			return nil, err
		}
		f.ClientID = new(string(cid))
	}
	page, err := s.Repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	notice, err := s.runtimeNotices(ctx)
	if err != nil {
		return nil, err
	}
	shown := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		shown = append(shown, item.ID)
	}
	notices, err := s.Repo.NoticesOf(ctx, shown)
	if err != nil {
		return nil, usecase.Internal("REPO", "notices failed", err)
	}
	body := IngestionListResponse{Ingestions: make([]IngestionResponse, 0, len(page.Items)), HasMore: page.Next != ""}
	for _, item := range page.Items {
		r, err := fromEntity(&item.Ingestion, item.ApplicationCode, item.ClientIdentifier)
		if err != nil {
			return nil, usecase.Internal("MARSHAL", "ingestion does not render", err)
		}
		res, err := s.Repo.Resolve(ctx, item.ApplicationCode, item.ClientID, item.Code, item.Definition)
		if err != nil {
			return nil, usecase.Internal("REPO", "config entry lookup failed", err)
		}
		withConfig(&r, s.shown(ctx, res, item.Definition))
		r.RuntimeNotice = notice(&item.Ingestion)
		withNotices(&r, notices[item.ID])
		body.Ingestions = append(body.Ingestions, r)
	}
	if page.Next != "" {
		body.NextCursor = &page.Next
	}
	return &apicommon.Out[IngestionListResponse]{Body: body}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return s.respond(ctx, i)
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateIngestionRequest]) (*apicommon.Out[IngestionResponse], error) {
	// Coarse permission at the controller; the use case enforces reach (the
	// client and the application). The client reference is resolved only
	// after the permission check, so a caller without it learns nothing
	// about which clients exist.
	if err := auth.CanCreateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	def, err := parseDefinition(in.Body.IngestionDefinition)
	if err != nil {
		return nil, err
	}
	cid, err := s.resolveClient(ctx, in.Body.Client)
	if err != nil {
		return nil, err
	}
	cmd := operations.CreateCommand{
		Application: in.Body.Application,
		ClientID:    cid,
		Code:        in.Body.Code,
		Definition:  def,
		// Live by default; without promote the first version is a draft
		// (owner ruling 2026-10-25), whatever the request asked.
		Live: operations.LiveIfPermitted(ctx, in.Body.Live == nil || *in.Body.Live),
		Note: in.Body.Note,
	}
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateIngestion(s.Repo, s.Resolver), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	out, err := s.reload(ctx, event.IngestionID)
	if err != nil {
		return nil, err
	}
	res := operations.SaveResult{Outcome: operations.OutcomeSaved, Version: event.Version}
	if !cmd.Live {
		res.Outcome = operations.OutcomeDraft
	}
	out.Body.Save = saveOutcome(res)
	return out, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateIngestionRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Out[IngestionResponse], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	def, err := parseDefinition(in.Body.IngestionDefinition)
	if err != nil {
		return nil, err
	}
	cmd := operations.UpdateCommand{ID: in.ID, ExpectedVersion: in.Body.ExpectedVersion, Definition: def, Note: in.Body.Note}
	// An update that changes nothing answers 200 with the ingestion as it is;
	// save says what the update did (a draft without promote).
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.UpdateIngestion(s.Repo, s.Resolver), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	out, err := s.reload(ctx, res.Ingestion.ID)
	if err != nil {
		return nil, err
	}
	out.Body.Save = saveOutcome(res)
	// The answer shows the live definition; the warnings of the one just saved
	// (a draft over a live version, say) are added to its own.
	out.Body.Warnings = mergeWarnings(out.Body.Warnings, res.Warnings)
	return out, nil
}

// mergeWarnings is a with the warnings of b it does not have.
func mergeWarnings(a, b []string) []string {
	for _, w := range b {
		if !slices.Contains(a, w) {
			a = append(a, w)
		}
	}
	return a
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteIngestion(s.Repo), operations.DeleteCommand{ID: in.ID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) pause(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionResponse], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.PauseIngestion(s.Repo), operations.PauseCommand{ID: in.ID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return s.reload(ctx, in.ID)
}

func (s *State) resume(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionResponse], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ResumeIngestion(s.Repo), operations.ResumeCommand{ID: in.ID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return s.reload(ctx, in.ID)
}

func (s *State) listVersions(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionVersionListResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	versions, err := s.Repo.ListVersions(ctx, i.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_versions failed", err)
	}
	body := IngestionVersionListResponse{Versions: make([]IngestionVersionResponse, 0, len(versions))}
	for _, v := range versions {
		r, err := fromVersion(v, i.Version)
		if err != nil {
			return nil, usecase.Internal("MARSHAL", "version does not render", err)
		}
		body.Versions = append(body.Versions, r)
	}
	return &apicommon.Out[IngestionVersionListResponse]{Body: body}, nil
}

type saveVersionInput struct {
	ID   string `path:"id"`
	Body SaveIngestionVersionRequest
}

func (s *State) saveVersion(ctx context.Context, in *saveVersionInput) (*apicommon.Out[IngestionVersionResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateIngestions(ac); err != nil {
		return nil, err
	}
	live := in.Body.Live != nil && *in.Body.Live
	if live {
		if err := auth.CanPromoteIngestions(ac); err != nil {
			return nil, err
		}
	}
	def, err := parseDefinition(in.Body.IngestionDefinition)
	if err != nil {
		return nil, err
	}
	cmd := operations.SaveVersionCommand{ID: in.ID, Definition: def, Live: live, Note: in.Body.Note}
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.SaveVersion(s.Repo, s.Resolver), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	v, err := s.Repo.FindVersion(ctx, in.ID, res.Version)
	if err != nil || v == nil {
		return nil, usecase.Internal("REPO", "post-save version reload failed", err)
	}
	body, err := fromVersion(*v, res.Ingestion.Version)
	if err != nil {
		return nil, usecase.Internal("MARSHAL", "version does not render", err)
	}
	body.Save = saveOutcome(res)
	body.Warnings = res.Warnings
	return &apicommon.Out[IngestionVersionResponse]{Body: body}, nil
}

type makeLiveInput struct {
	ID      string `path:"id"`
	Version int    `path:"version" minimum:"1"`
}

func (s *State) makeLive(ctx context.Context, in *makeLiveInput) (*apicommon.Out[IngestionResponse], error) {
	if err := auth.CanPromoteIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	cmd := operations.MakeVersionLiveCommand{ID: in.ID, Version: in.Version}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.MakeVersionLive(s.Repo), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return s.reload(ctx, in.ID)
}

type definitionOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// registerDefinition mounts getIngestionDefinition by hand: its answer is YAML
// text, which the standard route helpers (JSON bodies) do not describe.
func registerDefinition(g apiroute.Group, s *State) {
	huma.Register(g.API, huma.Operation{
		OperationID: "getIngestionDefinition",
		Method:      http.MethodGet,
		Path:        "/api/ingestions/{id}/definition",
		Summary:     "Get an ingestion as a definition file (YAML)",
		Tags:        []string{g.Tag},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "The definition file `fcdev ingest apply` reads back",
				Content:     map[string]*huma.MediaType{"application/yaml": {Schema: &huma.Schema{Type: "string"}}},
			},
		},
	}, s.definition)
}

func (s *State) definition(ctx context.Context, in *apicommon.IDInput) (*definitionOutput, error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	app, err := s.Apps.FindByID(ctx, string(i.ApplicationID))
	if err != nil || app == nil {
		return nil, usecase.Internal("REPO", "application lookup failed", err)
	}
	c, err := s.Clients.FindByID(ctx, string(i.ClientID))
	if err != nil || c == nil {
		return nil, usecase.Internal("REPO", "client lookup failed", err)
	}
	// The client is written by identifier, which ports between environments.
	file := ingestion.File{Application: app.Code, Client: c.Identifier, Code: i.Code, Active: i.Active, Definition: i.Definition}
	yaml, err := file.YAML()
	if err != nil {
		return nil, usecase.Internal("MARSHAL", "definition does not render", err)
	}
	return &definitionOutput{ContentType: "application/yaml", Body: yaml}, nil
}

type historyInput struct {
	ID     string `path:"id"`
	Limit  int    `query:"limit" minimum:"0" maximum:"200" doc:"Page size (default 50, at most 200)"`
	Offset int    `query:"offset" minimum:"0"`
}

// history reads the ingestion's rows from the platform audit log: who did what
// and when, written by the use-case envelope with every change.
func (s *State) history(ctx context.Context, in *historyInput) (*apicommon.Out[IngestionHistoryResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	entityType := "Ingestion"
	// One more than the page, to know whether there is a next one.
	rows, err := s.Audit.FindWithFilters(ctx, audit.FilterParams{
		EntityType: &entityType, EntityID: &i.ID, Limit: limit + 1, Offset: in.Offset,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "audit lookup failed", err)
	}
	body := IngestionHistoryResponse{Entries: make([]IngestionHistoryEntry, 0, min(len(rows), limit)), HasMore: len(rows) > limit}
	for _, l := range rows[:min(len(rows), limit)] {
		body.Entries = append(body.Entries, IngestionHistoryEntry{
			ID: l.ID, Operation: l.Operation, PrincipalID: l.PrincipalID, PrincipalName: l.PrincipalName,
			Details: l.OperationJSON, PerformedAt: jsontime.New(l.PerformedAt),
		})
	}
	return &apicommon.Out[IngestionHistoryResponse]{Body: body}, nil
}

type failuresInput struct {
	ID          string `path:"id"`
	After       string `query:"after" doc:"Cursor from a previous page's nextCursor"`
	Limit       int    `query:"limit" minimum:"0" maximum:"200" doc:"Page size (default 50, at most 200)"`
	Undismissed bool   `query:"undismissed" doc:"Only the failures nobody has dismissed"`
	Category    string `query:"category" enum:"payload,rejected" doc:"Only this category; rejected is the notices"`
}

func (s *State) failures(ctx context.Context, in *failuresInput) (*apicommon.Out[IngestionFailureListResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	page, err := s.Repo.ListFailuresFiltered(ctx, i.ID, ingestion.FailureFilter{
		Undismissed: in.Undismissed, Category: in.Category, After: in.After, Limit: in.Limit,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "list_failures failed", err)
	}
	body := IngestionFailureListResponse{Failures: apicommon.MapSlice(page.Items, fromFailure), HasMore: page.Next != ""}
	if page.Next != "" {
		body.NextCursor = &page.Next
	}
	return &apicommon.Out[IngestionFailureListResponse]{Body: body}, nil
}

// sourceKinds lists the platform's own kinds, then the kinds the live runtimes
// report. It is the catalogue the definition form is drawn from, so it is
// global: no ingestion's reach applies, only the view permission.
func (s *State) sourceKinds(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[IngestionSourceKindListResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	reported, err := s.Repo.LiveKinds(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "live kinds failed", err)
	}
	kinds := make([]IngestionSourceKindResponse, 0, 8+len(reported))
	for _, src := range []ingestion.Source{
		ingestion.HTTP{},
		ingestion.Sftp{},
		ingestion.S3{},
		ingestion.Sqs{},
		ingestion.RabbitMQ{},
		ingestion.DatabasePoll{},
		ingestion.APIPoll{},
	} {
		kinds = append(kinds, IngestionSourceKindResponse{Kind: src.Kind(), BuiltIn: true, SingleOwner: src.SingleOwner()})
	}
	for _, k := range reported {
		if ingestion.IsBuiltInKind(k.Kind) {
			continue // a runtime cannot redefine a platform kind
		}
		kinds = append(kinds, IngestionSourceKindResponse{
			Kind: k.Kind, Description: k.Description, SingleOwner: ingestion.Custom{}.SingleOwner(),
			RequiredPermission: k.RequiredPermission, Settings: k.Settings,
		})
	}
	return &apicommon.Out[IngestionSourceKindListResponse]{Body: IngestionSourceKindListResponse{Kinds: kinds}}, nil
}

type dismissInput struct {
	ID        string `path:"id"`
	FailureID string `path:"failureId"`
}

// dismissFailure marks a failure as seen and dealt with; a notice stops
// showing on the ingestion. Dismissing one already dismissed answers it as it
// is and writes nothing.
func (s *State) dismissFailure(ctx context.Context, in *dismissInput) (*apicommon.Out[IngestionFailureResponse], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	cmd := operations.DismissFailureCommand{IngestionID: in.ID, FailureID: in.FailureID}
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.DismissFailure(s.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionFailureResponse]{Body: fromFailure(res.Failure)}, nil
}

func fromFailure(f *ingestion.Failure) IngestionFailureResponse {
	r := IngestionFailureResponse{
		ID: f.ID, RuntimeID: f.RuntimeID, Version: f.Version, Kind: f.Kind, Category: f.Category, Message: f.Message,
		AfterRecords: f.AfterRecords, Excerpt: f.Excerpt, OccurredAt: jsontime.New(f.OccurredAt),
		CreatedAt: jsontime.New(f.CreatedAt), DismissedBy: f.DismissedBy,
	}
	if f.DismissedAt != nil {
		r.DismissedAt = new(jsontime.New(*f.DismissedAt))
	}
	return r
}
