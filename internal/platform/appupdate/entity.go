// Package appupdate is the platform's self-hosted Expo Updates server: phone
// apps built with Expo fetch over-the-air updates from it over the Expo
// Updates protocol, version 1, with code signing (docs/app-updates-design.md).
//
// An Update belongs to one branch: (application, app key, platform, runtime
// version, channel). The public routes (package public) answer a phone with the
// branch's newest live update, subject to its staged rollout; the API (package
// api) publishes, rolls out and rolls back.
package appupdate

import (
	"regexp"
	"strings"
	"time"
)

// Kind is what a row of a branch tells a phone.
type Kind string

const (
	// KindUpdate is an update: a manifest and its files.
	KindUpdate Kind = "UPDATE"
	// KindRollbackToEmbedded tells phones to run the bundle embedded in their binary.
	KindRollbackToEmbedded Kind = "ROLLBACK_TO_EMBEDDED"
)

// The two platforms of the protocol (the expo-platform request header).
const (
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
)

// Limits (docs/app-updates-design.md section 9; the owner's to change).
const (
	// MaxAssets is how many files (launch asset included) one update may name.
	MaxAssets = 2000
	// MaxExpoClientBytes caps the public app config carried in the manifest.
	MaxExpoClientBytes = 256 << 10
	// MaxMessageLen caps an update's message, in characters.
	MaxMessageLen = 1000
	// DefaultMaxFileBytes is the per-file upload ceiling (FC_APP_UPDATES_MAX_FILE_BYTES).
	DefaultMaxFileBytes = 64 << 20
	// MaxMissingQuery is how many hashes one missing-files question may ask about.
	MaxMissingQuery = 5000
)

var (
	// applicationCodePattern admits every application code the platform creates
	// (validate.CodePattern and its underscore variant), bounded like the column.
	applicationCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)
	appKeyPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	channelPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	runtimeVersionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:+-]{0,127}$`)
	sha256Pattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
	fileExtensionPattern   = regexp.MustCompile(`^\.[A-Za-z0-9]{1,16}$`)
	sourceCommitPattern    = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,100}$`)
)

// ValidApplicationCode reports whether s can be an application code.
func ValidApplicationCode(s string) bool { return applicationCodePattern.MatchString(s) }

// ValidAppKey reports whether s is a phone app's key: lower-case letters,
// digits and hyphens, at most 40 characters.
func ValidAppKey(s string) bool { return appKeyPattern.MatchString(s) }

// ValidChannel reports whether s is a channel name.
func ValidChannel(s string) bool { return channelPattern.MatchString(s) }

// ValidRuntimeVersion reports whether s is a runtime version: an app version
// ("1.4.0"), a fingerprint hash, "exposdk:58.0.0" and the like.
func ValidRuntimeVersion(s string) bool { return runtimeVersionPattern.MatchString(s) }

// ValidPlatform reports whether s is android or ios.
func ValidPlatform(s string) bool { return s == PlatformAndroid || s == PlatformIOS }

// ValidSHA256 reports whether s is a lower-case hex SHA-256.
func ValidSHA256(s string) bool { return sha256Pattern.MatchString(s) }

// ValidSourceCommit reports whether s can be recorded as a source commit.
func ValidSourceCommit(s string) bool { return sourceCommitPattern.MatchString(s) }

// NormalizeFileExtension returns ext with its leading dot ("png" and ".png"
// are both ".png"); ok is false when it is not 1-16 letters and digits. Both
// clients require a non-launch asset's fileExtension and save the file under
// key+extension, so it must be filename-safe.
func NormalizeFileExtension(ext string) (string, bool) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext, fileExtensionPattern.MatchString(ext)
}

// LaunchAssetContentType is the launch asset's content type, as EAS serves it
// (a Hermes bytecode bundle included).
const LaunchAssetContentType = "application/javascript"

// contentTypes is a fixed table, so the manifest never depends on the host's
// MIME database. Anything else is application/octet-stream: the clients do not
// use an asset's content type to load it.
var contentTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".bmp": "image/bmp", ".ico": "image/x-icon", ".svg": "image/svg+xml",
	".heic": "image/heic", ".avif": "image/avif",
	".ttf": "font/ttf", ".otf": "font/otf", ".woff": "font/woff", ".woff2": "font/woff2",
	".json": "application/json", ".js": "application/javascript", ".hbc": "application/javascript",
	".html": "text/html", ".css": "text/css", ".txt": "text/plain", ".xml": "application/xml",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4", ".aac": "audio/aac", ".ogg": "audio/ogg",
	".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".pdf": "application/pdf", ".lottie": "application/zip", ".db": "application/octet-stream",
}

// ContentTypeFor is the content type of a file with extension ext (".png").
func ContentTypeFor(ext string) string {
	if ct, ok := contentTypes[strings.ToLower(ext)]; ok {
		return ct
	}
	return "application/octet-stream"
}

// Branch is what a phone asks about: one phone app of an application, on one
// platform, runtime version and channel.
type Branch struct {
	ApplicationID  string
	AppKey         string
	Platform       string
	RuntimeVersion string
	Channel        string
}

// Asset is one file of an update's manifest.
type Asset struct {
	SHA256 string
	MD5    string
	// FileExtension has its leading dot; empty for the launch asset (the
	// clients ignore it there and EAS omits it).
	FileExtension string
	ContentType   string
	IsLaunchAsset bool
}

// Update is one row of a branch: an update, or a rollback to the embedded bundle.
type Update struct {
	ID            string
	ApplicationID string
	// ApplicationCode is read with the row (not stored on it).
	ApplicationCode string
	Branch
	Kind Kind
	// ManifestID is the manifest's UUID; empty for a rollback.
	ManifestID string
	// CreatedAt is the manifest's createdAt or the directive's commitTime:
	// strictly increasing per branch, millisecond precision.
	CreatedAt       time.Time
	RolloutPercent  int
	Message         *string
	SourceCommit    *string
	RepublishedFrom *string
	// ExpoClient is the public app config, served as the manifest's extra.expoClient.
	ExpoClient    []byte
	PublishedBy   string
	PublishedName *string
	UpdatedAt     time.Time
	// Assets is the manifest's files in order, the launch asset first. Loaded
	// only where the caller needs them.
	Assets []Asset
	// TotalBytes is the sum of the files' sizes, when read with them.
	TotalBytes int64
}

// IDStr implements usecase.HasID.
func (u Update) IDStr() string { return u.ID }

// LaunchAsset is the update's entry bundle, or nil (a rollback, or assets not loaded).
func (u *Update) LaunchAsset() *Asset {
	for i := range u.Assets {
		if u.Assets[i].IsLaunchAsset {
			return &u.Assets[i]
		}
	}
	return nil
}

// File is one file an application has stored.
type File struct {
	SHA256    string
	MD5       string
	SizeBytes int64
}

// CommitTime truncates t to the millisecond, the precision of the wire format
// (FormatTime) and of what a phone compares.
func CommitTime(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

// FormatTime is the protocol's date: yyyy-MM-dd'T'HH:mm:ss.SSS'Z', the one
// shape Android parses (UpdatesUtils.parseDateString; anything else becomes
// "now" on the phone, which breaks ordering).
func FormatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// NextCommitTime is the commit time of a new row of a branch whose newest row
// was committed at prev (zero when the branch is empty): now, or one
// millisecond after prev when now is not later. A phone loads an update only
// when it is strictly newer than the one it runs.
func NextCommitTime(now, prev time.Time) time.Time {
	t := CommitTime(now)
	if !prev.IsZero() && !t.After(prev) {
		t = CommitTime(prev).Add(time.Millisecond)
	}
	return t
}
