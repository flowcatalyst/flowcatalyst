// Package api wires HTTP routes for function via huma.
package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// maxArtifactBytes is the raw-upload ceiling (plan §8.2 PUT
// .../artifacts/{digest}).
const maxArtifactBytes = 64 * 1024 * 1024 // 64 MiB

// State bundles deps.
type State struct {
	Repo      *function.Repository
	Apps      *application.Repository
	UoW       *usecasepgx.UnitOfWork
	Artifacts artifact.Store
	// Loader reads a published artifact's describe document — the platform's
	// own no-capability engine+loader (plan §3), built once at wire time.
	Loader *runtimes.Loader
	// Wiring bundles the repositories + runner URL template promote wiring
	// (WP8) reconciles against.
	Wiring operations.WiringDeps
}

const tag = "functions"

// Register mounts the function endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listFunctions", "/api/functions", "List functions", s.list)
	apiroute.Post(g, "createFunction", "/api/functions", "Create a function", http.StatusCreated, s.create)
	apiroute.Get(g, "getFunction", "/api/functions/{id}", "Get a function by id", s.getByID)
	apiroute.Delete(g, "deleteFunction", "/api/functions/{id}", "Delete a function", http.StatusNoContent, s.delete)

	// PATCH has no apiroute helper (apiroute only covers GET/POST/PUT/DELETE
	// — see internal/platform/shared/apiroute/apiroute.go); registered
	// directly per that package's documented escape hatch.
	huma.Register(api, huma.Operation{
		OperationID:   "updateFunction",
		Method:        http.MethodPatch,
		Path:          "/api/functions/{id}",
		Summary:       "Update a function's settings",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	// Raw-body upload — MaxBodyBytes and the RawBody content type are set
	// directly on the Operation, beyond apiroute's six fields.
	huma.Register(api, huma.Operation{
		OperationID:   "putFunctionArtifact",
		Method:        http.MethodPut,
		Path:          "/api/functions/{id}/artifacts/{digest}",
		Summary:       "Upload a function artifact by its sha256 digest",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
		MaxBodyBytes:  maxArtifactBytes,
	}, s.putArtifact)

	// ── Versions (WP4) ──────────────────────────────────────────────────
	// publishVersion has no apiroute helper: its 200-vs-201 status is
	// decided per-request (idempotent replay vs a real new version), and
	// apiroute only supports a single fixed DefaultStatus.
	huma.Register(api, huma.Operation{
		OperationID: "publishFunctionVersion",
		Method:      http.MethodPost,
		Path:        "/api/functions/{id}/versions",
		Summary:     "Publish a new function version from a stored artifact",
		Tags:        []string{tag},
	}, s.publishVersion)
	apiroute.Get(g, "listFunctionVersions", "/api/functions/{id}/versions", "List a function's versions", s.listVersions)
	apiroute.Get(g, "getFunctionVersion", "/api/functions/{id}/versions/{number}", "Get one function version", s.getVersion)
	apiroute.Post(g, "retireFunctionVersion", "/api/functions/{id}/versions/{number}/retire", "Retire a function version", http.StatusOK, s.retireVersion)

	// ── Aliases (WP4 + WP8 promote wiring) ──────────────────────────────
	apiroute.Put(g, "putFunctionAlias", "/api/functions/{id}/aliases/{name}", "Point an alias at a version", http.StatusOK, s.putAlias)
	apiroute.Delete(g, "deleteFunctionAlias", "/api/functions/{id}/aliases/{name}", "Remove an alias", http.StatusOK, s.deleteAlias)
	apiroute.Get(g, "listFunctionAliases", "/api/functions/{id}/aliases", "List a function's aliases", s.listAliases)

	// ── Settings (WP4) ───────────────────────────────────────────────────
	apiroute.Put(g, "putFunctionConfig", "/api/functions/{id}/config/{key}", "Set a config value", http.StatusOK, s.putConfig)
	apiroute.Delete(g, "deleteFunctionConfig", "/api/functions/{id}/config/{key}", "Remove a config value", http.StatusNoContent, s.deleteConfig)
	apiroute.Put(g, "putFunctionSecret", "/api/functions/{id}/secrets/{key}", "Set a secret value", http.StatusOK, s.putSecret)
	apiroute.Delete(g, "deleteFunctionSecret", "/api/functions/{id}/secrets/{key}", "Remove a secret value", http.StatusNoContent, s.deleteSecret)
	apiroute.Put(g, "putFunctionDB", "/api/functions/{id}/db/{name}", "Set a database DSN", http.StatusOK, s.putDB)
	apiroute.Delete(g, "deleteFunctionDB", "/api/functions/{id}/db/{name}", "Remove a database DSN", http.StatusNoContent, s.deleteDB)
	apiroute.Get(g, "listFunctionSettings", "/api/functions/{id}/settings", "List a function's settings keys", s.listSettings)
}

type listInput struct {
	ApplicationID string `query:"applicationId" doc:"Filter by owning application id"`
	ClientID      string `query:"clientId" doc:"Filter by client id"`
	AddressPrefix string `query:"addressPrefix" doc:"Filter by address prefix (e.g. an application code)"`
	apicommon.PageQuery
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[apicommon.OffsetPage[FunctionResponse]], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadFunctions(ac); err != nil {
		return nil, err
	}
	filters := function.ListFilters{
		ApplicationID: apicommon.OptStr(in.ApplicationID),
		ClientID:      apicommon.OptStr(in.ClientID),
		AddressPrefix: apicommon.OptStr(in.AddressPrefix),
	}
	// Scope to accessible clients in SQL (anchor sees all → no scoping) so
	// COUNT and LIMIT/OFFSET stay consistent across pages — the paginated
	// form of the auth.FilterClientScoped rule (see
	// function.ListFilters.AccessibleClientIDs and scheduledjob/api.list,
	// the sibling this mirrors).
	if !ac.IsAnchor() {
		clients := ac.Clients
		filters.AccessibleClientIDs = &clients
	}
	total, err := s.Repo.CountWithFilters(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "count_with_filters failed", err)
	}
	limit, offset := in.LimitVal(), in.OffsetVal()
	filters.Limit, filters.Offset = &limit, &offset
	rows, err := s.Repo.FindWithFilters(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	page := apicommon.NewOffsetPage(out, in.PageIndex(), in.PageSizeVal(), total)
	return &apicommon.Out[apicommon.OffsetPage[FunctionResponse]]{Body: page}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FunctionResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadFunctions(ac); err != nil {
		return nil, err
	}
	f, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, httperror.NotFound("Function", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, f.ClientID); err != nil {
		return nil, err
	}
	return &apicommon.Out[FunctionResponse]{Body: fromEntity(f)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateFunctionRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	// Coarse permission at the controller; the use case enforces per-client
	// resource access (you may only bind a function to a client you can
	// access; a platform-owned function requires anchor).
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateFunction(s.Repo, s.Apps), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.FunctionID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateFunctionRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateFunction(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	result, err := usecaseop.RunTx(ctx, s.UoW, operations.DeleteFunction(s.Repo, s.Wiring), operations.DeleteCommand{ID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	// Artifact store cleanup happens AFTER the transaction committed
	// (result.OrphanedDigests was computed inside it) — best-effort: a
	// failed delete here leaves a harmless orphan in the store, never a
	// failed function delete (the DB rows are already gone).
	for _, digest := range result.OrphanedDigests {
		if delErr := s.Artifacts.Delete(ctx, digest); delErr != nil {
			slog.Error("function delete: orphaned artifact cleanup failed",
				"digest", digest, "err", delErr)
		}
	}
	return &apicommon.Empty{}, nil
}

type putArtifactInput struct {
	ID      string `path:"id"`
	Digest  string `path:"digest" doc:"Lowercase hex sha256 of the artifact (64 characters)"`
	RawBody []byte `contentType:"application/wasm"`
}

type putArtifactOutput struct {
	Status int
}

// putArtifact stores the raw request body under Digest, verifying its
// sha256 matches (400 DIGEST_MISMATCH otherwise). Idempotent: re-uploading
// the same digest with matching content is a 200; a fresh digest is 201.
func (s *State) putArtifact(ctx context.Context, in *putArtifactInput) (*putArtifactOutput, error) {
	ac := auth.FromContext(ctx)
	// Publishing an artifact is gated on the publish permission (the
	// artifact upload is the first half of "publish" — plan §4: "Publish
	// validates; promote materialises"), not the coarse manage permission.
	if err := auth.CanPublishFunctions(ac); err != nil {
		return nil, err
	}

	f, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, httperror.NotFound("Function", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, f.ClientID); err != nil {
		return nil, err
	}

	digest := in.Digest
	if !artifact.ValidDigest(digest) {
		return nil, usecase.Validation("INVALID_DIGEST", "digest must be a lowercase hex sha256 (64 characters)")
	}

	existed, err := s.Artifacts.Exists(ctx, digest)
	if err != nil {
		return nil, usecase.Internal("ARTIFACT_STORE", "exists check failed", err)
	}

	// Store.Put verifies the streamed content's sha256 equals digest (plan
	// §8.2), returning artifact.ErrDigestMismatch otherwise — mapped to 400
	// DIGEST_MISMATCH here.
	if err := s.Artifacts.Put(ctx, digest, bytes.NewReader(in.RawBody)); err != nil {
		if errors.Is(err, artifact.ErrDigestMismatch) {
			return nil, usecase.Validation("DIGEST_MISMATCH", "uploaded content's sha256 does not match the requested digest")
		}
		return nil, usecase.Internal("ARTIFACT_STORE", "put failed", err)
	}

	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	return &putArtifactOutput{Status: status}, nil
}

// ── Versions (WP4) ────────────────────────────────────────────────────────

type publishVersionInput struct {
	ID   string `path:"id"`
	Body PublishVersionRequest
}

type publishVersionOutput struct {
	Body   VersionResponse
	Status int
}

// publishVersion reads an artifact's describe document through the
// platform's own engine (s.Loader), validates it, and stores a new version
// — or, on a same-digest replay, returns the existing one unchanged (200
// instead of 201).
func (s *State) publishVersion(ctx context.Context, in *publishVersionInput) (*publishVersionOutput, error) {
	if err := auth.CanPublishFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	result, err := usecaseop.RunTx(ctx, s.UoW, operations.Publish(s.Repo, s.Artifacts, s.Loader), in.Body.toCommand(in.ID), ec)
	if err != nil {
		return nil, err
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	return &publishVersionOutput{Body: versionResponse(result.Version), Status: status}, nil
}

func (s *State) listVersions(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[[]VersionResponse], error) {
	if err := auth.CanReadFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, versions, err := operations.ListVersions(ctx, s.Repo, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[[]VersionResponse]{Body: apicommon.MapSlice(versions, func(v *function.Version) VersionResponse { return versionResponse(*v) })}, nil
}

type versionPathInput struct {
	ID     string `path:"id"`
	Number int32  `path:"number"`
}

func (s *State) getVersion(ctx context.Context, in *versionPathInput) (*apicommon.Out[VersionResponse], error) {
	if err := auth.CanReadFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, v, err := operations.GetVersion(ctx, s.Repo, in.ID, in.Number)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[VersionResponse]{Body: versionResponse(*v)}, nil
}

func (s *State) retireVersion(ctx context.Context, in *versionPathInput) (*apicommon.Out[VersionResponse], error) {
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	v, err := usecaseop.RunTx(ctx, s.UoW, operations.RetireVersion(s.Repo), operations.RetireCommand{FunctionID: in.ID, Number: in.Number}, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[VersionResponse]{Body: versionResponse(v)}, nil
}

// ── Aliases (WP4 + WP8 promote wiring) ──────────────────────────────────

type aliasPathInput struct {
	ID   string `path:"id"`
	Name string `path:"name"`
}

type putAliasInput struct {
	ID   string `path:"id"`
	Name string `path:"name"`
	Body PutAliasRequest
}

// putAlias points (id, name) at a READY version. Promoting `live`
// additionally reconciles wiring (WP8) — see operations.PutAlias.
func (s *State) putAlias(ctx context.Context, in *putAliasInput) (*apicommon.Out[PutAliasResponse], error) {
	if err := auth.CanPromoteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.PutAliasCommand{FunctionID: in.ID, Name: in.Name, Number: in.Body.Version}
	result, err := usecaseop.RunTx(ctx, s.UoW, operations.PutAlias(s.Repo, s.Wiring), cmd, ec)
	if err != nil {
		return nil, err
	}
	body := PutAliasResponse{
		Alias: AliasResponse{
			FunctionID: result.Alias.FunctionID,
			Name:       result.Alias.Name,
			Version:    result.Version.Number,
			UpdatedAt:  jsontime.New(result.Alias.UpdatedAt),
			UpdatedBy:  result.Alias.UpdatedBy,
		},
	}
	if in.Name == function.LiveAlias {
		body.Wiring = WiringResponse{
			DispatchPoolCode:     result.Wiring.DispatchPoolCode,
			SubscriptionsCreated: result.Wiring.SubscriptionsCreated,
			SubscriptionsUpdated: result.Wiring.SubscriptionsUpdated,
			SubscriptionsDeleted: result.Wiring.SubscriptionsDeleted,
			SchedulesCreated:     result.Wiring.SchedulesCreated,
			SchedulesUpdated:     result.Wiring.SchedulesUpdated,
			SchedulesDeleted:     result.Wiring.SchedulesDeleted,
		}
	}
	return &apicommon.Out[PutAliasResponse]{Body: body}, nil
}

// deleteAlias removes one alias. Removing `live` unwires the function
// (WP8) — see operations.DeleteAlias.
func (s *State) deleteAlias(ctx context.Context, in *aliasPathInput) (*apicommon.Out[WiringResponse], error) {
	if err := auth.CanPromoteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.DeleteAliasCommand{FunctionID: in.ID, Name: in.Name}
	result, err := usecaseop.RunTx(ctx, s.UoW, operations.DeleteAlias(s.Repo, s.Wiring), cmd, ec)
	if err != nil {
		return nil, err
	}
	var body WiringResponse
	if in.Name == function.LiveAlias {
		body = WiringResponse{
			DispatchPoolCode:     result.Wiring.DispatchPoolCode,
			SubscriptionsCreated: result.Wiring.SubscriptionsCreated,
			SubscriptionsUpdated: result.Wiring.SubscriptionsUpdated,
			SubscriptionsDeleted: result.Wiring.SubscriptionsDeleted,
			SchedulesCreated:     result.Wiring.SchedulesCreated,
			SchedulesUpdated:     result.Wiring.SchedulesUpdated,
			SchedulesDeleted:     result.Wiring.SchedulesDeleted,
		}
	}
	return &apicommon.Out[WiringResponse]{Body: body}, nil
}

func (s *State) listAliases(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[[]AliasResponse], error) {
	if err := auth.CanReadFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	f, aliases, err := operations.ListAliases(ctx, s.Repo, in.ID)
	if err != nil {
		return nil, err
	}
	versions, err := s.Repo.ListVersionsByFunction(ctx, f.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_versions_by_function failed", err)
	}
	numberByVersionID := make(map[string]int32, len(versions))
	for _, v := range versions {
		numberByVersionID[v.ID] = v.Number
	}
	out := make([]AliasResponse, 0, len(aliases))
	for _, a := range aliases {
		out = append(out, AliasResponse{
			FunctionID: a.FunctionID,
			Name:       a.Name,
			Version:    numberByVersionID[a.VersionID],
			UpdatedAt:  jsontime.New(a.UpdatedAt),
			UpdatedBy:  a.UpdatedBy,
		})
	}
	return &apicommon.Out[[]AliasResponse]{Body: out}, nil
}

// ── Settings (WP4) ───────────────────────────────────────────────────────

type keyPathInput struct {
	ID  string `path:"id"`
	Key string `path:"key"`
}

type putKeyInput struct {
	ID   string `path:"id"`
	Key  string `path:"key"`
	Body PutSettingRequest
}

type dbPathInput struct {
	ID   string `path:"id"`
	Name string `path:"name"`
}

type putDBInput struct {
	ID   string `path:"id"`
	Name string `path:"name"`
	Body PutSettingRequest
}

// putConfig, putSecret and putDB all funnel through putSetting; only the
// coarse permission and the settings Kind differ (plan §8.2: config is
// "manage", secrets/db are "secret:manage" — write-only, never returned).
func (s *State) putConfig(ctx context.Context, in *putKeyInput) (*apicommon.Out[SettingResponse], error) {
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.putSetting(ctx, in.ID, function.SettingConfig, in.Key, in.Body.Value)
}

func (s *State) putSecret(ctx context.Context, in *putKeyInput) (*apicommon.Out[SettingResponse], error) {
	if err := auth.CanManageFunctionSecrets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.putSetting(ctx, in.ID, function.SettingSecret, in.Key, in.Body.Value)
}

func (s *State) putDB(ctx context.Context, in *putDBInput) (*apicommon.Out[SettingResponse], error) {
	if err := auth.CanManageFunctionSecrets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.putSetting(ctx, in.ID, function.SettingDB, in.Name, in.Body.Value)
}

func (s *State) putSetting(ctx context.Context, functionID string, kind function.SettingKind, key, value string) (*apicommon.Out[SettingResponse], error) {
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.PutSettingCommand{FunctionID: functionID, Kind: kind, Key: key, Value: value}
	setting, err := usecaseop.RunTx(ctx, s.UoW, operations.PutSetting(s.Repo), cmd, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[SettingResponse]{Body: settingResponse(setting)}, nil
}

func (s *State) deleteConfig(ctx context.Context, in *keyPathInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.deleteSetting(ctx, in.ID, function.SettingConfig, in.Key)
}

func (s *State) deleteSecret(ctx context.Context, in *keyPathInput) (*apicommon.Empty, error) {
	if err := auth.CanManageFunctionSecrets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.deleteSetting(ctx, in.ID, function.SettingSecret, in.Key)
}

func (s *State) deleteDB(ctx context.Context, in *dbPathInput) (*apicommon.Empty, error) {
	if err := auth.CanManageFunctionSecrets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	return s.deleteSetting(ctx, in.ID, function.SettingDB, in.Name)
}

func (s *State) deleteSetting(ctx context.Context, functionID string, kind function.SettingKind, key string) (*apicommon.Empty, error) {
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.DeleteSettingCommand{FunctionID: functionID, Kind: kind, Key: key}
	if _, err := usecaseop.RunTx(ctx, s.UoW, operations.DeleteSetting(s.Repo), cmd, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) listSettings(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[[]SettingResponse], error) {
	if err := auth.CanReadFunctions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, settings, err := operations.ListSettings(ctx, s.Repo, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[[]SettingResponse]{Body: apicommon.MapSlice(settings, func(st *function.Setting) SettingResponse { return settingResponse(*st) })}, nil
}
