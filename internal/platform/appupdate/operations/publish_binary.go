package operations

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// PublishBinaryCommand publishes an uploaded binary (PUT
// /api/app-updates/binary-files/...) of a phone app to a channel in a
// variant. Everything else about the binary is what the platform read from
// the file. The audit log keeps the command as it is.
type PublishBinaryCommand struct {
	ApplicationCode string  `json:"applicationCode"`
	AppKey          string  `json:"appKey"`
	Platform        string  `json:"platform"`
	Channel         string  `json:"channel"`
	Variant         string  `json:"variant"`
	SHA256          string  `json:"sha256"`
	Message         *string `json:"message,omitempty"`
	SourceCommit    *string `json:"sourceCommit,omitempty"`
}

// PublishBinaryResult is the published binary and whether this call wrote it
// (publishing the same file to the same place again writes nothing).
type PublishBinaryResult struct {
	Binary  *appupdate.Binary
	Created bool
}

func validatePublishBinary(cmd PublishBinaryCommand) error {
	switch {
	case !appupdate.ValidApplicationCode(cmd.ApplicationCode):
		return usecase.Validation("INVALID_APPLICATION_CODE", "applicationCode is not an application code")
	case !appupdate.ValidAppKey(cmd.AppKey):
		return usecase.Validation("INVALID_APP_KEY", "appKey must be 1-40 lower-case letters, digits and hyphens, starting with a letter or digit")
	case cmd.Platform != appupdate.PlatformAndroid:
		return usecase.Validation("INVALID_PLATFORM", `platform must be "android": binaries are Android APKs (an iOS app cannot be installed from a link)`)
	case !appupdate.ValidChannel(cmd.Channel):
		return usecase.Validation("INVALID_CHANNEL", "channel must be 1-64 lower-case letters, digits and . _ -, starting with a letter or digit")
	case !appupdate.ValidVariant(cmd.Variant):
		return usecase.Validation("INVALID_VARIANT", `variant must be "release", "debug" or another 1-20 lower-case letters, digits and hyphens, starting with a letter`)
	case !appupdate.ValidSHA256(cmd.SHA256):
		return usecase.Validation("INVALID_SHA256", "sha256 must be a lower-case hex SHA-256")
	}
	return ValidateMessage(cmd.Message, cmd.SourceCommit)
}

// PublishBinary publishes a binary the application has uploaded, under a lock
// on its place (application, app key, platform, channel, variant), in one
// transaction with its event and audit row. The binary must be what its place
// says it is, read from the file, not from the publisher:
//
//   - uploaded by this application (409 FILE_MISSING otherwise);
//   - the release variant is what real users install: not debuggable (409
//     RELEASE_IS_DEBUGGABLE) and not signed with an Android debug keystore,
//     whose key is public (409 RELEASE_SIGNED_WITH_DEBUG_KEY);
//   - when the binary names its update channel (expo-channel-name), it is the
//     channel published to (409 CHANNEL_MISMATCH); when its update URL is one
//     of this platform's, it names this application and app key (409
//     UPDATE_URL_MISMATCH);
//   - the place's binaries are one package signed with one certificate, so a
//     phone can always install the newest over the one it has (409
//     PACKAGE_MISMATCH, SIGNING_CERTIFICATE_CHANGED);
//   - a version code names one file per place (409 VERSION_CODE_TAKEN).
//
// The same file published to the same place again is a no-op: the existing
// binary, Created false, no event.
func PublishBinary(repo *appupdate.BinaryRepository, apps AppLookup) usecaseop.TxOperation[PublishBinaryCommand, PublishBinaryResult] {
	return usecaseop.TxOperation[PublishBinaryCommand, PublishBinaryResult]{
		Name:     "PublishAppBinary",
		Validate: func(_ context.Context, cmd PublishBinaryCommand) error { return validatePublishBinary(cmd) },
		Authorize: func(ctx context.Context, cmd PublishBinaryCommand) error {
			_, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PublishBinaryCommand, ec usecase.ExecutionContext) (PublishBinaryResult, error) {
			var zero PublishBinaryResult
			app, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			if err != nil {
				return zero, err
			}
			if !app.Active {
				return zero, usecase.BusinessRule("APPLICATION_INACTIVE", "the application is not active")
			}
			place := appupdate.Place{
				ApplicationID: string(app.ID), AppKey: cmd.AppKey, Platform: cmd.Platform, Channel: cmd.Channel, Variant: cmd.Variant,
			}
			var file *appupdate.BinaryFile
			var same, newest *appupdate.Binary
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if err := repo.LockPlace(ctx, tx, place); err != nil {
					return err
				}
				if file, err = repo.FindFileTx(ctx, tx, place.ApplicationID, cmd.SHA256); err != nil || file == nil {
					return err
				}
				same, newest, err = repo.PlaceStateTx(ctx, tx, place, cmd.SHA256, file.VersionCode)
				return err
			}); err != nil {
				return zero, usecase.Internal("REPO", "read binary place failed", err)
			}
			if file == nil {
				return zero, usecase.Conflict("FILE_MISSING",
					fmt.Sprintf("the application has not uploaded this binary; upload it first (PUT /api/app-updates/binary-files/%s/%s)", cmd.ApplicationCode, cmd.SHA256))
			}
			if same != nil && same.SHA256 == cmd.SHA256 {
				return PublishBinaryResult{Binary: same}, nil
			}
			if err := checkBinaryForPlace(app.Code, cmd, file, same, newest); err != nil {
				return zero, err
			}
			now := time.Now().UTC()
			b := &appupdate.Binary{
				ID: tsid.Generate(tsid.AppBinary), ApplicationID: place.ApplicationID, ApplicationCode: app.Code,
				AppKey: cmd.AppKey, Channel: cmd.Channel, Variant: cmd.Variant, SHA256: file.SHA256, SizeBytes: file.SizeBytes,
				BinaryFacts: file.BinaryFacts, Message: cmd.Message, SourceCommit: cmd.SourceCommit,
				PublishedBy: ec.PrincipalID, CreatedAt: now,
			}
			event := BinaryPublished{
				eventBase: baseFor(ec, BinaryPublishedType, binarySubjectPrefix+b.ID, b.ApplicationID, b.AppKey),
				BinaryID:  b.ID, ApplicationID: b.ApplicationID, ApplicationCode: b.ApplicationCode, AppKey: b.AppKey,
				Platform: b.Platform, Channel: b.Channel, Variant: b.Variant, SHA256: b.SHA256, SizeBytes: b.SizeBytes,
				PackageName: b.PackageName, VersionCode: b.VersionCode, VersionName: b.VersionName,
				RuntimeVersion: b.RuntimeVersion, SigningCertSHA256: b.SigningCertSHA256,
				Message: b.Message, SourceCommit: b.SourceCommit,
			}
			if r := usecasepgx.CommitScoped(ctx, s, b, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return PublishBinaryResult{Binary: b, Created: true}, nil
		},
	}
}

// checkBinaryForPlace is the publish rules of PublishBinary that read the file
// and the place.
func checkBinaryForPlace(appCode string, cmd PublishBinaryCommand, f *appupdate.BinaryFile, sameVersion, newest *appupdate.Binary) error {
	if f.Platform != cmd.Platform {
		return usecase.Conflict("PLATFORM_MISMATCH", fmt.Sprintf("the file is a %s binary", f.Platform))
	}
	if cmd.Variant == appupdate.VariantRelease {
		if f.Debuggable {
			return usecase.Conflict("RELEASE_IS_DEBUGGABLE",
				"the APK is debuggable (android:debuggable): real users get the release variant; publish it as the debug variant, or build the release variant")
		}
		if f.DebugCertificate {
			return usecase.Conflict("RELEASE_SIGNED_WITH_DEBUG_KEY",
				"the APK is signed with an Android debug keystore (certificate "+f.SigningCertSubject+"), whose key is public: sign the release build with the app's own release key, or publish this one as the debug variant")
		}
	}
	if f.UpdateChannel != nil && *f.UpdateChannel != cmd.Channel {
		return usecase.Conflict("CHANNEL_MISMATCH",
			fmt.Sprintf("the APK asks for updates of channel %q (expo-channel-name), not %q", *f.UpdateChannel, cmd.Channel))
	}
	if f.UpdateURL != nil {
		if code, key, ok := appupdate.UpdateURLNames(*f.UpdateURL); ok && (code != appCode || key != cmd.AppKey) {
			return usecase.Conflict("UPDATE_URL_MISMATCH",
				fmt.Sprintf("the APK's update URL names application %q and app key %q, not %q and %q", code, key, appCode, cmd.AppKey))
		}
	}
	if newest != nil {
		if newest.PackageName != f.PackageName {
			return usecase.Conflict("PACKAGE_MISMATCH",
				fmt.Sprintf("the %s binaries of this channel are package %s; this APK is %s", cmd.Variant, newest.PackageName, f.PackageName))
		}
		if newest.SigningCertSHA256 != f.SigningCertSHA256 {
			return usecase.Conflict("SIGNING_CERTIFICATE_CHANGED",
				fmt.Sprintf("the %s binaries of this channel are signed with certificate %s; this APK with %s, which a phone cannot install over them", cmd.Variant, newest.SigningCertSHA256, f.SigningCertSHA256))
		}
	}
	if sameVersion != nil {
		return usecase.Conflict("VERSION_CODE_TAKEN",
			fmt.Sprintf("binary %s of this channel and variant already has version code %d; give the build a new version code", sameVersion.ID, f.VersionCode))
	}
	return nil
}
