package api

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// MissingFilesRequest asks which files an application has not stored.
type MissingFilesRequest struct {
	SHA256 []string `json:"sha256" doc:"Lower-case hex SHA-256 of each file (at most 5000)"`
}

// MissingFilesResponse lists the files still to upload, in the order asked.
type MissingFilesResponse struct {
	Missing []string `json:"missing" doc:"The SHA-256 of every file the application has not stored"`
}

// FileStoredResponse is the answer to a file upload.
type FileStoredResponse struct {
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	Created   bool   `json:"created" doc:"false when the application had already stored this file"`
}

// PublishAssetRequest is one file of an update.
type PublishAssetRequest struct {
	SHA256        string `json:"sha256" doc:"Lower-case hex SHA-256 of a stored file"`
	FileExtension string `json:"fileExtension" doc:"The extension the phone saves it under (png or .png): the expo export metadata's ext"`
}

// PublishLaunchAssetRequest names the entry bundle.
type PublishLaunchAssetRequest struct {
	SHA256 string `json:"sha256" doc:"Lower-case hex SHA-256 of the stored bundle (the expo export metadata's bundle)"`
}

// PublishUpdateRequest registers an update; its files must be stored first.
type PublishUpdateRequest struct {
	ApplicationCode string                    `json:"applicationCode"`
	AppKey          string                    `json:"appKey" doc:"The phone app (lower-case letters, digits, hyphens; at most 40)"`
	Platform        string                    `json:"platform" enum:"android,ios"`
	RuntimeVersion  string                    `json:"runtimeVersion" doc:"The runtime version of the binaries this update is for"`
	Channel         string                    `json:"channel" doc:"The channel the app sends in its expo-channel-name header"`
	RolloutPercent  *int                      `json:"rolloutPercent,omitempty" minimum:"0" maximum:"100" doc:"Share of the branch's phones that receive it; omitted is 100"`
	Message         *string                   `json:"message,omitempty"`
	SourceCommit    *string                   `json:"sourceCommit,omitempty"`
	LaunchAsset     PublishLaunchAssetRequest `json:"launchAsset"`
	Assets          []PublishAssetRequest     `json:"assets" doc:"Every other file, in the export's order"`
	ExpoClient      json.RawMessage           `json:"expoClient,omitempty" doc:"The public app config (npx expo config --type public --json), served as the manifest's extra.expoClient"`
	RequireBinary   bool                      `json:"requireBinary,omitempty" doc:"Refuse (409 NO_BINARY_FOR_RUNTIME_VERSION) unless a binary of this runtime version is published on the channel; default false"`
}

func (r PublishUpdateRequest) toCommand() operations.PublishCommand {
	assets := make([]operations.AssetRef, 0, len(r.Assets))
	for _, a := range r.Assets {
		assets = append(assets, operations.AssetRef{SHA256: a.SHA256, FileExtension: a.FileExtension})
	}
	return operations.PublishCommand{
		ApplicationCode: r.ApplicationCode, AppKey: r.AppKey, Platform: r.Platform,
		RuntimeVersion: r.RuntimeVersion, Channel: r.Channel, RolloutPercent: r.RolloutPercent,
		Message: r.Message, SourceCommit: r.SourceCommit, LaunchAsset: r.LaunchAsset.SHA256,
		Assets: assets, ExpoClient: r.ExpoClient, RequireBinary: r.RequireBinary,
	}
}

// SetRolloutRequest changes a rollout.
type SetRolloutRequest struct {
	RolloutPercent int `json:"rolloutPercent" minimum:"0" maximum:"100"`
}

// RepublishRequest makes an earlier update live again.
type RepublishRequest struct {
	Message *string `json:"message,omitempty"`
}

// RollbackRequest rolls a branch back.
type RollbackRequest struct {
	ApplicationCode string  `json:"applicationCode"`
	AppKey          string  `json:"appKey"`
	Platform        string  `json:"platform" enum:"android,ios"`
	RuntimeVersion  string  `json:"runtimeVersion"`
	Channel         string  `json:"channel"`
	To              string  `json:"to" enum:"PREVIOUS,EMBEDDED" doc:"PREVIOUS republishes the update before the current one; EMBEDDED tells phones to run their embedded bundle"`
	Message         *string `json:"message,omitempty"`
}

// AppUpdateResponse is one update (or rollback) of a branch.
type AppUpdateResponse struct {
	ID              string        `json:"id"`
	ApplicationID   string        `json:"applicationId"`
	ApplicationCode string        `json:"applicationCode"`
	AppKey          string        `json:"appKey"`
	Platform        string        `json:"platform" enum:"android,ios"`
	RuntimeVersion  string        `json:"runtimeVersion"`
	Channel         string        `json:"channel"`
	Kind            string        `json:"kind" enum:"UPDATE,ROLLBACK_TO_EMBEDDED"`
	ManifestID      *string       `json:"manifestId,omitempty" doc:"The manifest's UUID (updates only)"`
	CreatedAt       jsontime.Time `json:"createdAt" doc:"The manifest's createdAt or the rollback's commitTime"`
	RolloutPercent  int           `json:"rolloutPercent"`
	ServedPercent   int           `json:"servedPercent" doc:"Share of the branch's phones this row is served to now: the newest row its rollout, the one before it the rest while the newest is partial, any other 0"`
	Live            bool          `json:"live" doc:"servedPercent > 0"`
	Message         *string       `json:"message,omitempty"`
	SourceCommit    *string       `json:"sourceCommit,omitempty"`
	RepublishedFrom *string       `json:"republishedFrom,omitempty"`
	PublishedBy     string        `json:"publishedBy" doc:"The principal that published it"`
	PublishedByName *string       `json:"publishedByName,omitempty"`
	UpdatedAt       jsontime.Time `json:"updatedAt"`
	// BinariesOnBranch is set only by a publish.
	BinariesOnBranch *int64 `json:"binariesOnBranch,omitempty" doc:"Only in a publish's answer: how many binaries of this runtime version are published on the channel (0: no phone installed from this platform runs it)"`
}

func updateResponse(u *appupdate.Update, served int) AppUpdateResponse {
	r := AppUpdateResponse{
		ID: u.ID, ApplicationID: u.ApplicationID, ApplicationCode: u.ApplicationCode, AppKey: u.AppKey,
		Platform: u.Platform, RuntimeVersion: u.RuntimeVersion, Channel: u.Channel, Kind: string(u.Kind),
		CreatedAt: jsontime.New(u.CreatedAt), RolloutPercent: u.RolloutPercent, ServedPercent: served, Live: served > 0,
		Message: u.Message, SourceCommit: u.SourceCommit, RepublishedFrom: u.RepublishedFrom,
		PublishedBy: u.PublishedBy, PublishedByName: u.PublishedName, UpdatedAt: jsontime.New(u.UpdatedAt),
	}
	if u.ManifestID != "" {
		m := u.ManifestID
		r.ManifestID = &m
	}
	return r
}

// AppUpdateAssetResponse is one file of an update.
type AppUpdateAssetResponse struct {
	SHA256        string `json:"sha256"`
	Key           string `json:"key" doc:"The manifest key (the file's MD5)"`
	FileExtension string `json:"fileExtension,omitempty"`
	ContentType   string `json:"contentType"`
	IsLaunchAsset bool   `json:"isLaunchAsset"`
}

// AppUpdateDetailResponse is one update with its files.
type AppUpdateDetailResponse struct {
	Update        AppUpdateResponse        `json:"update"`
	Assets        []AppUpdateAssetResponse `json:"assets"`
	TotalBytes    int64                    `json:"totalBytes"`
	HasExpoClient bool                     `json:"hasExpoClient" doc:"Whether the manifest carries the app config (extra.expoClient)"`
}

// AppUpdateBranchResponse is one branch with what is live on it.
type AppUpdateBranchResponse struct {
	AppKey         string `json:"appKey"`
	Platform       string `json:"platform" enum:"android,ios"`
	RuntimeVersion string `json:"runtimeVersion"`
	Channel        string `json:"channel"`
	// Current is the newest row; Previous the one before it, set only while
	// Current is a partial rollout (it reaches the rest of the phones).
	Current     AppUpdateResponse  `json:"current"`
	Previous    *AppUpdateResponse `json:"previous,omitempty" doc:"Served to the phones outside the current update's partial rollout"`
	UpdateCount int64              `json:"updateCount"`
}

// AppUpdateBranchesResponse lists an application's branches.
type AppUpdateBranchesResponse struct {
	Branches []AppUpdateBranchResponse `json:"branches"`
}

// RollbackResponse is a rollback's outcome.
type RollbackResponse struct {
	Update  AppUpdateResponse `json:"update" doc:"The branch's newest row after the rollback"`
	Created bool              `json:"created" doc:"false when the branch was already rolled back to the embedded bundle"`
}
