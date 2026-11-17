package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// AssetRef names one stored file of an update and the extension it is saved under.
type AssetRef struct {
	SHA256        string `json:"sha256"`
	FileExtension string `json:"fileExtension"`
}

// PublishCommand registers an update. Its files must already be stored
// (PUT /api/app-updates/files/...). The audit log records a summary
// (MarshalJSON), not the asset list or the app config.
type PublishCommand struct {
	ApplicationCode string
	AppKey          string
	Platform        string
	RuntimeVersion  string
	Channel         string
	// RolloutPercent is 0..100; nil is 100.
	RolloutPercent *int
	Message        *string
	SourceCommit   *string
	// LaunchAsset is the SHA-256 of the entry bundle.
	LaunchAsset string
	Assets      []AssetRef
	// ExpoClient is the public app config (a JSON object), served as the
	// manifest's extra.expoClient (Constants.expoConfig in the running update).
	ExpoClient json.RawMessage
}

// MarshalJSON is what the audit log keeps of a publish.
func (c PublishCommand) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationCode string  `json:"applicationCode"`
		AppKey          string  `json:"appKey"`
		Platform        string  `json:"platform"`
		RuntimeVersion  string  `json:"runtimeVersion"`
		Channel         string  `json:"channel"`
		RolloutPercent  *int    `json:"rolloutPercent,omitempty"`
		Message         *string `json:"message,omitempty"`
		SourceCommit    *string `json:"sourceCommit,omitempty"`
		LaunchAsset     string  `json:"launchAssetSha256"`
		AssetCount      int     `json:"assetCount"`
		HasExpoClient   bool    `json:"hasExpoClient"`
	}{
		c.ApplicationCode, c.AppKey, c.Platform, c.RuntimeVersion, c.Channel, c.RolloutPercent, c.Message,
		c.SourceCommit, c.LaunchAsset, len(c.Assets), len(c.ExpoClient) > 0,
	})
}

// ValidateBranch checks the four coordinates a phone sends plus the application code.
func ValidateBranch(code, appKey, platform, runtimeVersion, channel string) error {
	switch {
	case !appupdate.ValidApplicationCode(code):
		return usecase.Validation("INVALID_APPLICATION_CODE", "applicationCode is not an application code")
	case !appupdate.ValidAppKey(appKey):
		return usecase.Validation("INVALID_APP_KEY", "appKey must be 1-40 lower-case letters, digits and hyphens, starting with a letter or digit")
	case !appupdate.ValidPlatform(platform):
		return usecase.Validation("INVALID_PLATFORM", `platform must be "android" or "ios"`)
	case !appupdate.ValidRuntimeVersion(runtimeVersion):
		return usecase.Validation("INVALID_RUNTIME_VERSION", "runtimeVersion must be 1-128 letters, digits and . _ : + -, starting with a letter or digit")
	case !appupdate.ValidChannel(channel):
		return usecase.Validation("INVALID_CHANNEL", "channel must be 1-64 lower-case letters, digits and . _ -, starting with a letter or digit")
	}
	return nil
}

// ValidateMessage checks an update's optional message and source commit.
func ValidateMessage(message, sourceCommit *string) error {
	if message != nil && (utf8.RuneCountInString(*message) > appupdate.MaxMessageLen || !utf8.ValidString(*message)) {
		return usecase.Validation("INVALID_MESSAGE", fmt.Sprintf("message must be valid text of at most %d characters", appupdate.MaxMessageLen))
	}
	if sourceCommit != nil && !appupdate.ValidSourceCommit(*sourceCommit) {
		return usecase.Validation("INVALID_SOURCE_COMMIT", "sourceCommit must be 1-100 letters, digits and . _ / -")
	}
	return nil
}

// ValidateRollout checks a rollout percentage.
func ValidateRollout(p int) error {
	if p < 0 || p > 100 {
		return usecase.Validation("INVALID_ROLLOUT", "rolloutPercent must be from 0 to 100")
	}
	return nil
}

func validatePublish(cmd PublishCommand) error {
	if err := ValidateBranch(cmd.ApplicationCode, cmd.AppKey, cmd.Platform, cmd.RuntimeVersion, cmd.Channel); err != nil {
		return err
	}
	if cmd.RolloutPercent != nil {
		if err := ValidateRollout(*cmd.RolloutPercent); err != nil {
			return err
		}
	}
	if err := ValidateMessage(cmd.Message, cmd.SourceCommit); err != nil {
		return err
	}
	if !appupdate.ValidSHA256(cmd.LaunchAsset) {
		return usecase.Validation("INVALID_LAUNCH_ASSET", "launchAsset.sha256 must be a lower-case hex SHA-256")
	}
	if len(cmd.Assets) >= appupdate.MaxAssets {
		return usecase.Validation("TOO_MANY_ASSETS", fmt.Sprintf("an update may name at most %d files", appupdate.MaxAssets))
	}
	for i, a := range cmd.Assets {
		if !appupdate.ValidSHA256(a.SHA256) {
			return usecase.Validation("INVALID_ASSET", fmt.Sprintf("assets[%d].sha256 must be a lower-case hex SHA-256", i))
		}
		if _, ok := appupdate.NormalizeFileExtension(a.FileExtension); !ok {
			return usecase.Validation("INVALID_ASSET", fmt.Sprintf("assets[%d].fileExtension must be 1-16 letters and digits (with or without a leading dot)", i))
		}
	}
	if len(cmd.ExpoClient) > 0 {
		if len(cmd.ExpoClient) > appupdate.MaxExpoClientBytes {
			return usecase.Validation("INVALID_EXPO_CLIENT", fmt.Sprintf("expoClient may be at most %d bytes", appupdate.MaxExpoClientBytes))
		}
		trimmed := bytes.TrimSpace(cmd.ExpoClient)
		var obj map[string]json.RawMessage
		if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &obj) != nil {
			return usecase.Validation("INVALID_EXPO_CLIENT", "expoClient must be a JSON object (the public app config)")
		}
	}
	return nil
}

// Publish registers an update and makes it live for its rollout, atomically:
// under a lock on the branch it checks every file is in the application's
// registry (409 FILES_MISSING otherwise), refuses while the branch's newest
// update is a partial rollout (409 ROLLOUT_IN_PROGRESS), gives the update a
// commit time later than every row of the branch, and writes it, its assets,
// the event and the audit row in one transaction. A phone reads committed rows
// only, so it is never told about an update whose files are not all stored.
func Publish(repo *appupdate.Repository, apps AppLookup) usecaseop.TxOperation[PublishCommand, *appupdate.Update] {
	return usecaseop.TxOperation[PublishCommand, *appupdate.Update]{
		Name:     "PublishAppUpdate",
		Validate: func(_ context.Context, cmd PublishCommand) error { return validatePublish(cmd) },
		Authorize: func(ctx context.Context, cmd PublishCommand) error {
			_, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PublishCommand, ec usecase.ExecutionContext) (*appupdate.Update, error) {
			app, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			if err != nil {
				return nil, err
			}
			if !app.Active {
				return nil, usecase.BusinessRule("APPLICATION_INACTIVE", "the application is not active; its phones are told no update is available")
			}
			branch := appupdate.Branch{
				ApplicationID: string(app.ID), AppKey: cmd.AppKey, Platform: cmd.Platform,
				RuntimeVersion: cmd.RuntimeVersion, Channel: cmd.Channel,
			}

			assets := orderedAssets(cmd)
			shas := make([]string, 0, len(assets))
			for _, a := range assets {
				shas = append(shas, a.SHA256)
			}

			var head []appupdate.Update
			var files map[string]appupdate.File
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if err := repo.LockBranch(ctx, tx, branch); err != nil {
					return err
				}
				if head, err = repo.HeadTx(ctx, tx, branch, 1); err != nil {
					return err
				}
				files, err = repo.FilesTx(ctx, tx, branch.ApplicationID, shas)
				return err
			}); err != nil {
				return nil, usecase.Internal("REPO", "read branch failed", err)
			}
			if err := checkNoPartialRollout(head); err != nil {
				return nil, err
			}
			var missing []string
			for _, sha := range shas {
				if _, ok := files[sha]; !ok {
					missing = append(missing, sha)
				}
			}
			if len(missing) > 0 {
				return nil, usecase.Conflict("FILES_MISSING",
					fmt.Sprintf("%d of the update's files are not stored for this application; upload them first (PUT /api/app-updates/files/%s/{sha256})", len(missing), cmd.ApplicationCode)).
					WithDetails(map[string]any{"missing": firstN(missing, 50)})
			}
			for i := range assets {
				assets[i].MD5 = files[assets[i].SHA256].MD5
			}

			now := time.Now().UTC()
			var prev time.Time
			if len(head) > 0 {
				prev = head[0].CreatedAt
			}
			rollout := 100
			if cmd.RolloutPercent != nil {
				rollout = *cmd.RolloutPercent
			}
			u := &appupdate.Update{
				ID: tsid.Generate(tsid.AppUpdate), ApplicationID: branch.ApplicationID, ApplicationCode: app.Code,
				Branch: branch, Kind: appupdate.KindUpdate, ManifestID: uuid.NewString(),
				CreatedAt: appupdate.NextCommitTime(now, prev), RolloutPercent: rollout,
				Message: cmd.Message, SourceCommit: cmd.SourceCommit, ExpoClient: compactJSON(cmd.ExpoClient),
				PublishedBy: ec.PrincipalID, UpdatedAt: now, Assets: assets,
			}
			event := UpdatePublished{
				eventBase: newBase(ec, UpdatePublishedType, u), BranchData: branchData(u),
				ManifestID: u.ManifestID, CreatedAt: appupdate.FormatTime(u.CreatedAt), RolloutPercent: rollout,
				LaunchAssetSHA256: cmd.LaunchAsset, AssetCount: len(assets), Message: cmd.Message, SourceCommit: cmd.SourceCommit,
			}
			if r := usecasepgx.CommitScoped(ctx, s, u, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return nil, e
			}
			return u, nil
		},
	}
}

// orderedAssets is the manifest's asset list: the launch asset first, then
// the assets in the order given, an exact duplicate (same file and extension)
// dropped.
func orderedAssets(cmd PublishCommand) []appupdate.Asset {
	out := []appupdate.Asset{{SHA256: cmd.LaunchAsset, ContentType: appupdate.LaunchAssetContentType, IsLaunchAsset: true}}
	seen := map[string]bool{}
	for _, a := range cmd.Assets {
		ext, _ := appupdate.NormalizeFileExtension(a.FileExtension)
		ext = strings.ToLower(ext)
		k := a.SHA256 + ext
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, appupdate.Asset{SHA256: a.SHA256, FileExtension: ext, ContentType: appupdate.ContentTypeFor(ext)})
	}
	return out
}

// checkNoPartialRollout refuses a new row on a branch whose newest update is
// partially rolled out: only the newest row of a branch may be partial.
func checkNoPartialRollout(head []appupdate.Update) error {
	if len(head) > 0 && head[0].Kind == appupdate.KindUpdate && head[0].RolloutPercent < 100 {
		return usecase.Conflict("ROLLOUT_IN_PROGRESS",
			fmt.Sprintf("update %s is rolled out to %d%% of this branch's phones; finish it (rollout 100) or roll back before publishing another",
				head[0].ID, head[0].RolloutPercent))
	}
	return nil
}

func compactJSON(b json.RawMessage) []byte {
	if len(b) == 0 {
		return nil
	}
	var out bytes.Buffer
	if json.Compact(&out, b) != nil {
		return nil
	}
	return out.Bytes()
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
