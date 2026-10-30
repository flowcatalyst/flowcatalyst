package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const setTag = "ingestion-sets"

// The most files an apply keeps, and their size in all.
const (
	maxSetFiles     = 500
	maxSetFileChars = 2_000_000
)

// registerSets mounts the set routes: a set owns the ingestions it applied
// from definition files until it is released (docs/ingestion-design.md
// section 5).
func registerSets(api apiroute.Group, s *State) {
	g := apiroute.New(api.API, setTag)
	apiroute.Get(g, "listIngestionSets", "/api/ingestion-sets", "List the sets that own ingestions", s.listSets)
	apiroute.Put(g, "applyIngestionSet", "/api/ingestion-sets/{name}",
		"Make a set's ingestions what its items say: create, change, take in ones made by hand, and delete what it no longer lists", http.StatusOK, s.applySet)
	apiroute.Get(g, "listIngestionSetApplies", "/api/ingestion-sets/{name}/applies", "List a set's kept applies, newest first", s.listApplies)
	apiroute.Get(g, "getIngestionSetApply", "/api/ingestion-sets/{name}/applies/{applyId}", "Get what one apply of a set was given", s.getApply)
	apiroute.Post(g, "reapplyIngestionSet", "/api/ingestion-sets/{name}/applies/{applyId}/reapply",
		"Go back to an earlier apply: apply the set again with the items that apply was given, as a new apply", http.StatusOK, s.reapplySet)
	apiroute.Post(g, "releaseIngestionSet", "/api/ingestion-sets/{name}/release",
		"Let go of a set: its ingestions stay as they are and are edited one at a time again", http.StatusOK, s.releaseSet)
}

// canReadSets is the view permission, for a caller who sees every client and
// application: a set spans them, and its list and history name them all.
func canReadSets(ctx context.Context) error {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIngestions(ac); err != nil {
		return err
	}
	if !ac.IsAnchor() || ac.IsApplicationScoped() {
		return usecase.Authorization("INGESTION_SET_ANCHOR_ONLY", "sets span clients and applications: only a caller that reaches all of them lists or reads them")
	}
	return nil
}

type setPath struct {
	Name string `path:"name" doc:"The set's name: lowercase letters, digits and hyphens"`
}

type setApplyPath struct {
	Name    string `path:"name"`
	ApplyID string `path:"applyId"`
}

func (s *State) listSets(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[IngestionSetListResponse], error) {
	if err := canReadSets(ctx); err != nil {
		return nil, err
	}
	sets, err := s.Repo.ListSets(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "list sets failed", err)
	}
	body := IngestionSetListResponse{Sets: make([]IngestionSetResponse, 0, len(sets))}
	for _, set := range sets {
		body.Sets = append(body.Sets, IngestionSetResponse{
			ID: set.ID, Name: set.Name, Ingestions: set.Ingestions,
			CreatedAt: jsontime.New(set.CreatedAt), UpdatedAt: jsontime.New(set.UpdatedAt),
		})
	}
	return &apicommon.Out[IngestionSetListResponse]{Body: body}, nil
}

type applySetInput struct {
	Name   string `path:"name" doc:"The set's name: lowercase letters, digits and hyphens"`
	DryRun bool   `query:"dryRun" doc:"Do all the checks and say what would be done, writing nothing"`
	Body   ApplyIngestionSetRequest
}

func (s *State) applySet(ctx context.Context, in *applySetInput) (*apicommon.Out[IngestionSetPlanResponse], error) {
	if err := auth.CanApplyIngestionSets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	items := make([]ingestion.File, 0, len(in.Body.Items))
	for n, item := range in.Body.Items {
		f, err := item.file()
		if err != nil {
			return nil, itemError(n, err)
		}
		items = append(items, f)
	}
	files, err := setFiles(in.Body.Files)
	if err != nil {
		return nil, err
	}
	return s.runApply(ctx, operations.ApplySetCommand{Name: in.Name, Items: items, Files: files, DryRun: in.DryRun})
}

func (s *State) runApply(ctx context.Context, cmd operations.ApplySetCommand) (*apicommon.Out[IngestionSetPlanResponse], error) {
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.ApplySet(s.Repo, s.Resolver, s.SetArchive), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	// The applies no longer kept: their archives go once the rows are gone. A
	// store that cannot delete leaves garbage, not a wrong answer.
	for _, digest := range res.Forgotten {
		if err := s.SetArchive.Delete(ctx, digest); err != nil {
			slog.Warn("ingestion set archive is no longer kept and could not be removed", "set", res.Set, "digest", digest, "err", err)
		}
	}
	return &apicommon.Out[IngestionSetPlanResponse]{Body: IngestionSetPlanResponse{
		Name: res.Set, DryRun: res.DryRun, ApplyID: res.ApplyID, Changes: res.Changes,
	}}, nil
}

// findSet loads the set a by-name read acts on.
func (s *State) findSet(ctx context.Context, name string) (*ingestion.Set, error) {
	set, err := s.Repo.FindSetByName(ctx, name)
	if err != nil {
		return nil, usecase.Internal("REPO", "find set failed", err)
	}
	if set == nil {
		return nil, usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+name)
	}
	return set, nil
}

func (s *State) listApplies(ctx context.Context, in *setPath) (*apicommon.Out[IngestionSetApplyListResponse], error) {
	if err := canReadSets(ctx); err != nil {
		return nil, err
	}
	set, err := s.findSet(ctx, in.Name)
	if err != nil {
		return nil, err
	}
	applies, err := s.Repo.ListSetApplies(ctx, set.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list set applies failed", err)
	}
	body := IngestionSetApplyListResponse{Applies: make([]IngestionSetApplyResponse, 0, len(applies))}
	for _, a := range applies {
		body.Applies = append(body.Applies, fromSetApply(set.Name, a))
	}
	return &apicommon.Out[IngestionSetApplyListResponse]{Body: body}, nil
}

func (s *State) getApply(ctx context.Context, in *setApplyPath) (*apicommon.Out[IngestionSetArchiveResponse], error) {
	if err := canReadSets(ctx); err != nil {
		return nil, err
	}
	_, _, archived, err := s.readApply(ctx, in)
	if err != nil {
		return nil, err
	}
	body := IngestionSetArchiveResponse{
		Set: archived.Set, ApplyID: archived.ApplyID, AppliedAt: jsontime.New(archived.AppliedAt), AppliedBy: archived.AppliedBy,
		GoingBackTo: archived.GoingBackTo, Items: make([]IngestionSetItem, 0, len(archived.Items)), Files: archived.Files,
	}
	if body.Files == nil {
		body.Files = []ingestion.SetFile{}
	}
	for _, f := range archived.Items {
		item, err := itemOf(f)
		if err != nil {
			return nil, usecase.Internal("MARSHAL", "archived item does not render", err)
		}
		body.Items = append(body.Items, item)
	}
	return &apicommon.Out[IngestionSetArchiveResponse]{Body: body}, nil
}

// readApply loads a recorded apply and the archive of what it was given.
func (s *State) readApply(ctx context.Context, in *setApplyPath) (*ingestion.Set, *ingestion.SetApply, *ingestion.SetArchive, error) {
	set, err := s.findSet(ctx, in.Name)
	if err != nil {
		return nil, nil, nil, err
	}
	noApply := usecase.NotFound("INGESTION_SET_APPLY_NOT_FOUND", "the set does not keep apply "+in.ApplyID)
	apply, err := s.Repo.FindSetApply(ctx, set.ID, in.ApplyID)
	if err != nil {
		return nil, nil, nil, usecase.Internal("REPO", "find set apply failed", err)
	}
	if apply == nil || apply.ArchiveDigest == "" {
		return nil, nil, nil, noApply
	}
	archived, err := ingestion.ReadSetArchive(ctx, s.SetArchive, apply.ArchiveDigest)
	switch {
	case errors.Is(err, artifact.ErrNotFound):
		return nil, nil, nil, noApply
	case err != nil:
		return nil, nil, nil, usecase.Internal("INGESTION_SET_ARCHIVE_UNAVAILABLE", "the set archive could not be read; try again", err)
	}
	return set, apply, archived, nil
}

type reapplySetInput struct {
	Name    string `path:"name"`
	ApplyID string `path:"applyId"`
	DryRun  bool   `query:"dryRun" doc:"Do all the checks and say what would be done, writing nothing"`
}

func (s *State) reapplySet(ctx context.Context, in *reapplySetInput) (*apicommon.Out[IngestionSetPlanResponse], error) {
	if err := auth.CanApplyIngestionSets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, apply, archived, err := s.readApply(ctx, &setApplyPath{Name: in.Name, ApplyID: in.ApplyID})
	if err != nil {
		return nil, err
	}
	return s.runApply(ctx, operations.ApplySetCommand{
		Name: in.Name, Items: archived.Items, Files: archived.Files, DryRun: in.DryRun, GoingBackTo: &apply.ID,
	})
}

func (s *State) releaseSet(ctx context.Context, in *setPath) (*apicommon.Out[IngestionSetReleaseResponse], error) {
	if err := auth.CanApplyIngestionSets(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.ReleaseSet(s.Repo), operations.ReleaseSetCommand{Name: in.Name}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionSetReleaseResponse]{Body: IngestionSetReleaseResponse{Name: res.Name, Released: res.Released}}, nil
}

// setFiles checks the files kept with an apply.
func setFiles(files []ingestion.SetFile) ([]ingestion.SetFile, error) {
	if len(files) > maxSetFiles {
		return nil, usecase.Validation("INGESTION_SET_FILES_INVALID", fmt.Sprintf("files: at most %d files", maxSetFiles))
	}
	chars := 0
	for n, f := range files {
		if strings.TrimSpace(f.Path) == "" || len(f.Path) > 300 {
			return nil, usecase.Validation("INGESTION_SET_FILES_INVALID", fmt.Sprintf("files[%d].path: must be 1 to 300 characters", n))
		}
		chars += len(f.Content)
	}
	if chars > maxSetFileChars {
		return nil, usecase.Validation("INGESTION_SET_FILES_INVALID", fmt.Sprintf("files: at most %d characters in all", maxSetFileChars))
	}
	return files, nil
}

// itemError says which item a bad definition is in.
func itemError(n int, err error) error {
	if ue := usecase.AsError(err); ue != nil {
		at := *ue
		at.Message = fmt.Sprintf("items[%d]: %s", n, ue.Message)
		return &at
	}
	return err
}
