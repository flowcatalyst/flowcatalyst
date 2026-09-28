// Package api wires HTTP routes for function via huma.
package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
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
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteFunction(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
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
