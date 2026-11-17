// Package api is the authenticated app-update API (/api/app-updates/*):
// storing files, publishing, reading, rollouts and rollbacks
// (docs/app-updates-design.md section 4).
package api

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // G501: MD5 is the asset key expo export names files by, not a security hash
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// fileReadTimeout bounds reading one uploaded file (huma's default is 5
// seconds; 64 MiB needs minutes on a slow link).
const fileReadTimeout = 10 * time.Minute

// maxBranches caps one branches answer.
const maxBranches = 500

// State bundles the API's dependencies.
type State struct {
	Repo  *appupdate.Repository
	Apps  operations.AppLookup
	UoW   *usecasepgx.UnitOfWork
	Store artifact.Store
	// MaxFileBytes caps one file (FC_APP_UPDATES_MAX_FILE_BYTES); < 1 is
	// appupdate.DefaultMaxFileBytes.
	MaxFileBytes int64
}

func (s *State) maxFileBytes() int64 {
	if s.MaxFileBytes > 0 {
		return s.MaxFileBytes
	}
	return appupdate.DefaultMaxFileBytes
}

const tag = "app-updates"

// Register mounts the API.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "findMissingAppUpdateFiles", "/api/app-updates/files/{applicationCode}/missing",
		"Which of these files the application has not stored", http.StatusOK, s.missingFiles)
	huma.Register(api, huma.Operation{
		OperationID:     "putAppUpdateFile",
		Method:          http.MethodPut,
		Path:            "/api/app-updates/files/{applicationCode}/{sha256}",
		Summary:         "Store one update file by its SHA-256",
		Tags:            []string{tag},
		DefaultStatus:   http.StatusOK,
		MaxBodyBytes:    s.maxFileBytes(),
		BodyReadTimeout: fileReadTimeout,
	}, s.putFile)
	apiroute.Post(g, "publishAppUpdate", "/api/app-updates/updates", "Publish an update", http.StatusCreated, s.publish)
	apiroute.Get(g, "listAppUpdates", "/api/app-updates/updates", "List an application's updates, newest first", s.list)
	apiroute.Get(g, "getAppUpdate", "/api/app-updates/updates/{id}", "Get one update with its files", s.get)
	apiroute.Get(g, "listAppUpdateBranches", "/api/app-updates/branches", "List an application's branches with what is live on each", s.branches)
	apiroute.Put(g, "setAppUpdateRollout", "/api/app-updates/updates/{id}/rollout", "Change an update's rollout", http.StatusOK, s.setRollout)
	apiroute.Post(g, "republishAppUpdate", "/api/app-updates/updates/{id}/republish", "Make an earlier update live again", http.StatusCreated, s.republish)
	huma.Register(api, huma.Operation{
		OperationID: "rollbackAppUpdates",
		Method:      http.MethodPost,
		Path:        "/api/app-updates/rollback",
		Summary:     "Roll a branch back to its previous update or to the embedded bundle",
		Tags:        []string{tag},
	}, s.rollback)
}

type missingFilesInput struct {
	ApplicationCode string `path:"applicationCode"`
	Body            MissingFilesRequest
}

func (s *State) missingFiles(ctx context.Context, in *missingFilesInput) (*apicommon.Out[MissingFilesResponse], error) {
	if err := auth.CanPublishAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if len(in.Body.SHA256) > appupdate.MaxMissingQuery {
		return nil, usecase.Validation("TOO_MANY_FILES", fmt.Sprintf("ask about at most %d files at once", appupdate.MaxMissingQuery))
	}
	for i, h := range in.Body.SHA256 {
		if !appupdate.ValidSHA256(h) {
			return nil, usecase.Validation("INVALID_SHA256", fmt.Sprintf("sha256[%d] is not a lower-case hex SHA-256", i))
		}
	}
	missing, err := s.Repo.MissingFiles(ctx, string(app.ID), in.Body.SHA256)
	if err != nil {
		return nil, usecase.Internal("REPO", "missing files failed", err)
	}
	return &apicommon.Out[MissingFilesResponse]{Body: MissingFilesResponse{Missing: missing}}, nil
}

type putFileInput struct {
	ApplicationCode string `path:"applicationCode"`
	SHA256          string `path:"sha256" doc:"Lower-case hex SHA-256 of the body"`
	RawBody         []byte `contentType:"application/octet-stream"`
}

// putFile stores one file: the store verifies the body's SHA-256 against the
// address (400 DIGEST_MISMATCH otherwise) and writes it atomically; only then
// is it registered for the application. Idempotent.
func (s *State) putFile(ctx context.Context, in *putFileInput) (*apicommon.Out[FileStoredResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanPublishAppUpdates(ac); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if !appupdate.ValidSHA256(in.SHA256) {
		return nil, usecase.Validation("INVALID_SHA256", "the address must end in the body's lower-case hex SHA-256")
	}
	if len(in.RawBody) == 0 {
		return nil, usecase.Validation("EMPTY_FILE", "the body is empty")
	}
	if err := s.Store.Put(ctx, in.SHA256, bytes.NewReader(in.RawBody)); err != nil {
		if errors.Is(err, artifact.ErrDigestMismatch) {
			return nil, usecase.Validation("DIGEST_MISMATCH", "the body's SHA-256 does not match the address")
		}
		return nil, usecase.Internal("UPDATE_STORE", "store put failed", err)
	}
	sum := md5.Sum(in.RawBody) //nolint:gosec // G401: the asset key, not a security hash
	f := appupdate.File{SHA256: in.SHA256, MD5: hex.EncodeToString(sum[:]), SizeBytes: int64(len(in.RawBody))}
	created, err := s.Repo.RecordFile(ctx, string(app.ID), f, ac.PrincipalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "record file failed", err)
	}
	return &apicommon.Out[FileStoredResponse]{Body: FileStoredResponse{SHA256: f.SHA256, SizeBytes: f.SizeBytes, Created: created}}, nil
}

func (s *State) publish(ctx context.Context, in *apicommon.In[PublishUpdateRequest]) (*apicommon.Out[AppUpdateResponse], error) {
	if err := auth.CanPublishAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	u, err := usecaseop.RunTx(ctx, s.UoW, operations.Publish(s.Repo, s.Apps), in.Body.toCommand(), auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AppUpdateResponse]{Body: updateResponse(u, u.RolloutPercent)}, nil
}

type listInput struct {
	ApplicationCode string `query:"applicationCode" required:"true"`
	AppKey          string `query:"appKey"`
	Platform        string `query:"platform"`
	RuntimeVersion  string `query:"runtimeVersion"`
	Channel         string `query:"channel"`
	apicommon.PageQuery
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[apicommon.OffsetPage[AppUpdateResponse]], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(in.AppKey, in.Platform, in.RuntimeVersion, in.Channel); err != nil {
		return nil, err
	}
	rows, total, served, err := s.Repo.List(ctx, appupdate.ListFilter{
		ApplicationID: string(app.ID), AppKey: in.AppKey, Platform: in.Platform,
		RuntimeVersion: in.RuntimeVersion, Channel: in.Channel, Limit: in.LimitVal(), Offset: in.OffsetVal(),
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "list updates failed", err)
	}
	out := make([]AppUpdateResponse, 0, len(rows))
	for i := range rows {
		out = append(out, updateResponse(&rows[i], served[rows[i].ID]))
	}
	page := apicommon.NewOffsetPage(out, in.PageIndex(), in.PageSizeVal(), total)
	return &apicommon.Out[apicommon.OffsetPage[AppUpdateResponse]]{Body: page}, nil
}

// validateFilters checks the optional list filters (empty is "any").
func validateFilters(appKey, platform, runtimeVersion, channel string) error {
	switch {
	case appKey != "" && !appupdate.ValidAppKey(appKey):
		return usecase.Validation("INVALID_APP_KEY", "appKey is not an app key")
	case platform != "" && !appupdate.ValidPlatform(platform):
		return usecase.Validation("INVALID_PLATFORM", `platform must be "android" or "ios"`)
	case runtimeVersion != "" && !appupdate.ValidRuntimeVersion(runtimeVersion):
		return usecase.Validation("INVALID_RUNTIME_VERSION", "runtimeVersion is not a runtime version")
	case channel != "" && !appupdate.ValidChannel(channel):
		return usecase.Validation("INVALID_CHANNEL", "channel is not a channel name")
	}
	return nil
}

func (s *State) get(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[AppUpdateDetailResponse], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	u, err := operations.ReachUpdate(ctx, s.Repo, in.ID, true)
	if err != nil {
		return nil, err
	}
	served, err := s.Repo.ServedShare(ctx, u)
	if err != nil {
		return nil, usecase.Internal("REPO", "served share failed", err)
	}
	assets := make([]AppUpdateAssetResponse, 0, len(u.Assets))
	for _, a := range u.Assets {
		assets = append(assets, AppUpdateAssetResponse{
			SHA256: a.SHA256, Key: a.MD5, FileExtension: a.FileExtension,
			ContentType: a.ContentType, IsLaunchAsset: a.IsLaunchAsset,
		})
	}
	return &apicommon.Out[AppUpdateDetailResponse]{Body: AppUpdateDetailResponse{
		Update: updateResponse(u, served), Assets: assets, TotalBytes: u.TotalBytes, HasExpoClient: len(u.ExpoClient) > 0,
	}}, nil
}

type branchesInput struct {
	ApplicationCode string `query:"applicationCode" required:"true"`
	AppKey          string `query:"appKey"`
}

func (s *State) branches(ctx context.Context, in *branchesInput) (*apicommon.Out[AppUpdateBranchesResponse], error) {
	if err := auth.CanReadAppUpdates(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	app, err := operations.ReachApplication(ctx, s.Apps, in.ApplicationCode)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(in.AppKey, "", "", ""); err != nil {
		return nil, err
	}
	heads, err := s.Repo.Branches(ctx, string(app.ID), in.AppKey, maxBranches)
	if err != nil {
		return nil, usecase.Internal("REPO", "list branches failed", err)
	}
	out := make([]AppUpdateBranchResponse, 0, len(heads))
	for _, h := range heads {
		b := AppUpdateBranchResponse{
			AppKey: h.AppKey, Platform: h.Platform, RuntimeVersion: h.RuntimeVersion, Channel: h.Channel,
			Current: updateResponse(&h.Rows[0], appupdate.ServedPercent(h.Rows, 0)), UpdateCount: h.UpdateCount,
		}
		if len(h.Rows) > 1 && h.Rows[0].RolloutPercent < 100 {
			p := updateResponse(&h.Rows[1], appupdate.ServedPercent(h.Rows, 1))
			b.Previous = &p
		}
		out = append(out, b)
	}
	return &apicommon.Out[AppUpdateBranchesResponse]{Body: AppUpdateBranchesResponse{Branches: out}}, nil
}

type setRolloutInput struct {
	ID   string `path:"id"`
	Body SetRolloutRequest
}

func (s *State) setRollout(ctx context.Context, in *setRolloutInput) (*apicommon.Out[AppUpdateResponse], error) {
	if err := auth.CanManageAppUpdateRollouts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	u, err := usecaseop.RunTx(ctx, s.UoW, operations.SetRollout(s.Repo),
		operations.SetRolloutCommand{UpdateID: in.ID, RolloutPercent: in.Body.RolloutPercent}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AppUpdateResponse]{Body: updateResponse(u, u.RolloutPercent)}, nil
}

type republishInput struct {
	ID   string `path:"id"`
	Body RepublishRequest
}

func (s *State) republish(ctx context.Context, in *republishInput) (*apicommon.Out[AppUpdateResponse], error) {
	if err := auth.CanManageAppUpdateRollouts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	u, err := usecaseop.RunTx(ctx, s.UoW, operations.Republish(s.Repo),
		operations.RepublishCommand{UpdateID: in.ID, Message: in.Body.Message}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AppUpdateResponse]{Body: updateResponse(u, 100)}, nil
}

type rollbackOutput struct {
	Body   RollbackResponse
	Status int
}

func (s *State) rollback(ctx context.Context, in *apicommon.In[RollbackRequest]) (*rollbackOutput, error) {
	if err := auth.CanManageAppUpdateRollouts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	b := in.Body
	res, err := usecaseop.RunTx(ctx, s.UoW, operations.Rollback(s.Repo, s.Apps), operations.RollbackCommand{
		ApplicationCode: b.ApplicationCode, AppKey: b.AppKey, Platform: b.Platform, RuntimeVersion: b.RuntimeVersion,
		Channel: b.Channel, To: b.To, Message: b.Message,
	}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	return &rollbackOutput{Body: RollbackResponse{Update: updateResponse(res.Update, 100), Created: res.Created}, Status: status}, nil
}
