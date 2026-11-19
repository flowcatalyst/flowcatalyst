package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// AppBinaryFactsResponse is what the platform read from a binary's file.
type AppBinaryFactsResponse struct {
	Platform           string  `json:"platform" enum:"android,ios"`
	PackageName        string  `json:"packageName" doc:"The manifest's package (application id)"`
	VersionCode        int64   `json:"versionCode" doc:"android:versionCode (with versionCodeMajor in the upper 32 bits)"`
	VersionName        *string `json:"versionName,omitempty"`
	MinSDK             int     `json:"minSdk" doc:"uses-sdk minSdkVersion; 0 when absent"`
	Debuggable         bool    `json:"debuggable" doc:"android:debuggable on the application"`
	RuntimeVersion     *string `json:"runtimeVersion,omitempty" doc:"The runtime version the binary sends to the update routes (expo-updates); absent when it does not use expo-updates"`
	UpdateURL          *string `json:"updateUrl,omitempty" doc:"The binary's expo-updates URL"`
	UpdateChannel      *string `json:"updateChannel,omitempty" doc:"The expo-channel-name request header the binary sends"`
	UpdateKeyID        *string `json:"updateKeyId,omitempty" doc:"The code-signing key id the binary expects updates to be signed with"`
	SigningCertSHA256  string  `json:"signingCertificateSha256" doc:"SHA-256 of the signing certificate (v3's when present, else v2's), lower-case hex"`
	SigningCertSubject string  `json:"signingCertificateSubject"`
	DebugCertificate   bool    `json:"debugCertificate" doc:"Signed with an Android debug keystore (certificate CN \"Android Debug\")"`
	SigningSchemes     []int   `json:"signingSchemes" doc:"APK signature schemes that verified (2, 3)"`
}

func factsResponse(f *appupdate.BinaryFacts) AppBinaryFactsResponse {
	schemes := f.SigningSchemes
	if schemes == nil {
		schemes = []int{}
	}
	return AppBinaryFactsResponse{
		Platform: f.Platform, PackageName: f.PackageName, VersionCode: f.VersionCode, VersionName: f.VersionName,
		MinSDK: f.MinSDK, Debuggable: f.Debuggable, RuntimeVersion: f.RuntimeVersion, UpdateURL: f.UpdateURL,
		UpdateChannel: f.UpdateChannel, UpdateKeyID: f.UpdateKeyID, SigningCertSHA256: f.SigningCertSHA256,
		SigningCertSubject: f.SigningCertSubject, DebugCertificate: f.DebugCertificate, SigningSchemes: schemes,
	}
}

// AppBinaryFileResponse is an uploaded binary with what was read from it.
type AppBinaryFileResponse struct {
	SHA256    string                 `json:"sha256"`
	SizeBytes int64                  `json:"sizeBytes"`
	Created   bool                   `json:"created" doc:"false when the application had already uploaded this file"`
	Facts     AppBinaryFactsResponse `json:"facts"`
}

// PublishBinaryRequest publishes an uploaded binary.
type PublishBinaryRequest struct {
	ApplicationCode string  `json:"applicationCode"`
	AppKey          string  `json:"appKey" doc:"The phone app (lower-case letters, digits, hyphens; at most 40)"`
	Platform        string  `json:"platform" enum:"android" doc:"Only android: an iOS app cannot be installed from a link"`
	Channel         string  `json:"channel" doc:"The channel; when the binary names its expo-channel-name it must be this"`
	Variant         string  `json:"variant" doc:"release (the build real users get: not debuggable, not signed with a debug keystore), debug, or another name"`
	SHA256          string  `json:"sha256" doc:"Lower-case hex SHA-256 of the uploaded file"`
	Message         *string `json:"message,omitempty"`
	SourceCommit    *string `json:"sourceCommit,omitempty"`
}

// AppBinaryResponse is one published binary.
type AppBinaryResponse struct {
	ID              string                 `json:"id"`
	ApplicationID   string                 `json:"applicationId"`
	ApplicationCode string                 `json:"applicationCode"`
	AppKey          string                 `json:"appKey"`
	Channel         string                 `json:"channel"`
	Variant         string                 `json:"variant"`
	SHA256          string                 `json:"sha256"`
	SizeBytes       int64                  `json:"sizeBytes"`
	FileName        string                 `json:"fileName" doc:"The name it downloads under"`
	Facts           AppBinaryFactsResponse `json:"facts"`
	Newest          bool                   `json:"newest" doc:"The newest (highest version code) binary of its channel and variant: what an install link gives"`
	Message         *string                `json:"message,omitempty"`
	SourceCommit    *string                `json:"sourceCommit,omitempty"`
	PublishedBy     string                 `json:"publishedBy"`
	PublishedByName *string                `json:"publishedByName,omitempty"`
	CreatedAt       jsontime.Time          `json:"createdAt"`
}

func binaryResponse(b *appupdate.Binary, newest bool) AppBinaryResponse {
	return AppBinaryResponse{
		ID: b.ID, ApplicationID: b.ApplicationID, ApplicationCode: b.ApplicationCode, AppKey: b.AppKey,
		Channel: b.Channel, Variant: b.Variant, SHA256: b.SHA256, SizeBytes: b.SizeBytes, FileName: b.FileName(),
		Facts: factsResponse(&b.BinaryFacts), Newest: newest, Message: b.Message, SourceCommit: b.SourceCommit,
		PublishedBy: b.PublishedBy, PublishedByName: b.PublishedName, CreatedAt: jsontime.New(b.CreatedAt),
	}
}

// PublishBinaryResponse is a publish's outcome.
type PublishBinaryResponse struct {
	Binary  AppBinaryResponse `json:"binary"`
	Created bool              `json:"created" doc:"false when this file was already published to this channel and variant (nothing changed)"`
}

// AppBinaryCheckResponse answers whether a runtime version has a binary on a channel.
type AppBinaryCheckResponse struct {
	Published bool               `json:"published" doc:"A binary of this runtime version is published on the channel (any variant)"`
	Count     int64              `json:"count"`
	Newest    *AppBinaryResponse `json:"newest,omitempty" doc:"The one with the highest version code"`
}

// CreateInstallLinkRequest makes a channel's install link.
type CreateInstallLinkRequest struct {
	ApplicationCode string `json:"applicationCode"`
	AppKey          string `json:"appKey"`
	Channel         string `json:"channel"`
}

// AppInstallLinkResponse is one install link (never its token).
type AppInstallLinkResponse struct {
	ID              string        `json:"id"`
	ApplicationID   string        `json:"applicationId"`
	ApplicationCode string        `json:"applicationCode"`
	AppKey          string        `json:"appKey"`
	Channel         string        `json:"channel"`
	Generation      int           `json:"generation" doc:"1 when created; one more at every replacement"`
	CreatedBy       string        `json:"createdBy"`
	CreatedAt       jsontime.Time `json:"createdAt"`
	UpdatedBy       string        `json:"updatedBy"`
	UpdatedAt       jsontime.Time `json:"updatedAt"`
}

func linkResponse(l *appupdate.InstallLink) AppInstallLinkResponse {
	return AppInstallLinkResponse{
		ID: l.ID, ApplicationID: l.ApplicationID, ApplicationCode: l.ApplicationCode, AppKey: l.AppKey,
		Channel: l.Channel, Generation: l.Generation, CreatedBy: l.CreatedBy, CreatedAt: jsontime.New(l.CreatedAt),
		UpdatedBy: l.UpdatedBy, UpdatedAt: jsontime.New(l.UpdatedAt),
	}
}

// InstallLinkWithTokenResponse is a link just created or replaced, with its
// address: the only answer that carries the token (the platform keeps only
// its hash).
type InstallLinkWithTokenResponse struct {
	Link  AppInstallLinkResponse `json:"link"`
	URL   string                 `json:"url" doc:"The address to send: anyone holding it can download the channel's binaries. Shown once."`
	Token string                 `json:"token" doc:"The token in the address. Shown once."`
}

// AppInstallLinksResponse lists install links.
type AppInstallLinksResponse struct {
	Links []AppInstallLinkResponse `json:"links"`
}
