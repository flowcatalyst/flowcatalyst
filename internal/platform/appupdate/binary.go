package appupdate

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// App binaries and install links (docs/app-updates-design.md section 14): the
// Android build of a phone app, published by its pipeline, and the link a
// person opens to install it.

// The two variants every phone app is built in (the owner's ruling: a person
// may get either; real users get the release build).
const (
	// VariantRelease is the properly signed build real users get.
	VariantRelease = "release"
	// VariantDebug is the build for testers and developers.
	VariantDebug = "debug"
)

// Limits of binaries (docs/app-updates-design.md section 14.3).
const (
	// DefaultMaxBinaryBytes is the per-binary upload ceiling
	// (FC_APP_UPDATES_MAX_BINARY_BYTES). A universal APK of a React Native app
	// with every ABI and an unminified debug bundle is 150-250 MB; 512 MiB keeps
	// headroom while bounding the upload's spool file and its duration.
	DefaultMaxBinaryBytes = 512 << 20
	// InstallTokenBytes is how much randomness an install link's token
	// carries: 256 bits, from crypto/rand.
	InstallTokenBytes = 32
)

var (
	variantPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
	// installTokenPattern is a token as NewInstallToken makes it: 32 bytes in
	// unpadded base64url.
	installTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

// ValidVariant reports whether s is a build variant: release, debug, or
// another lower-case name of at most 20 characters.
func ValidVariant(s string) bool { return variantPattern.MatchString(s) }

// ValidInstallToken reports whether s has the shape of an install token.
func ValidInstallToken(s string) bool { return installTokenPattern.MatchString(s) }

// VariantOrder sorts variants as the install page lists them: release first
// (the one most people want), then debug, then any other by name.
func VariantOrder(a, b string) int {
	rank := func(v string) int {
		switch v {
		case VariantRelease:
			return 0
		case VariantDebug:
			return 1
		}
		return 2
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	return strings.Compare(a, b)
}

// BinaryFacts is what a binary says about itself: read by the platform from
// the file when it is uploaded (package apk), never taken from the publisher.
type BinaryFacts struct {
	Platform    string
	PackageName string
	VersionCode int64
	VersionName *string
	MinSDK      int
	Debuggable  bool
	// RuntimeVersion is the runtime version the binary sends to the update
	// routes (expo-updates' configuration), nil when it does not use expo-updates.
	RuntimeVersion *string
	// UpdateURL and UpdateChannel are the binary's update URL and the value of
	// its expo-channel-name request header, nil when absent.
	UpdateURL     *string
	UpdateChannel *string
	// UpdateKeyID is the code-signing key id the binary asks the update
	// routes to sign with, nil when absent.
	UpdateKeyID *string
	// SigningCertSHA256 is the SHA-256 of the certificate the APK is signed
	// with (v3's when present, else v2's); Android installs a binary over
	// another of the same package only when these are equal.
	SigningCertSHA256  string
	SigningCertSubject string
	// DebugCertificate: the certificate is an Android debug keystore's
	// (subject CN "Android Debug"), whose key is public.
	DebugCertificate bool
	// SigningSchemes are the APK signature schemes that verified (2, 3).
	SigningSchemes []int
}

// BinaryFile is one binary an application has uploaded, with what the
// platform read from it.
type BinaryFile struct {
	SHA256    string
	SizeBytes int64
	BinaryFacts
}

// Binary is one published binary of a phone app: an APK on a channel, in a
// variant. Immutable.
type Binary struct {
	ID              string
	ApplicationID   string
	ApplicationCode string
	AppKey          string
	Channel         string
	Variant         string
	SHA256          string
	SizeBytes       int64
	BinaryFacts
	Message       *string
	SourceCommit  *string
	PublishedBy   string
	PublishedName *string
	CreatedAt     time.Time
}

// IDStr implements usecase.HasID.
func (b Binary) IDStr() string { return b.ID }

// FileName is the name a person sees for the downloaded file:
// <app key>-<version>-<variant>.apk, the version made file-name safe.
func (b *Binary) FileName() string {
	v := ""
	if b.VersionName != nil {
		v = *b.VersionName
	}
	var sb strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	name := b.AppKey
	if s := sb.String(); s != "" && len(s) <= 64 {
		name += "-" + s
	} else {
		name += "-" + strconv.FormatInt(b.VersionCode, 10)
	}
	return name + "-" + b.Variant + ".apk"
}

// UpdateURLNames reports the application code and app key an update URL of
// this platform names (".../app-updates/<code>/<key>/manifest"); ok is false
// for a URL of another shape (another update server, or none).
func UpdateURLNames(raw string) (applicationCode, appKey string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	n := len(parts)
	if n < 4 || "/"+parts[n-4] != PublicPrefix || parts[n-1] != "manifest" {
		return "", "", false
	}
	return parts[n-3], parts[n-2], true
}

// InstallLink is the address a person opens to install a phone app's newest
// build of a channel. One per application, app key and channel; its token is
// kept only as a hash and is replaced (Generation + 1) to cut old copies off.
type InstallLink struct {
	ID              string
	ApplicationID   string
	ApplicationCode string
	AppKey          string
	Channel         string
	// TokenHash is the lower-case hex SHA-256 of the token.
	TokenHash  string
	Generation int
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedBy  string
	UpdatedAt  time.Time
}

// IDStr implements usecase.HasID.
func (l InstallLink) IDStr() string { return l.ID }

// NewInstallToken returns a new token (256 bits from crypto/rand, unpadded
// base64url) and the hash that is stored in its place.
func NewInstallToken() (token, hash string, err error) {
	b := make([]byte, InstallTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashInstallToken(token), nil
}

// HashInstallToken is the stored form of a token: its SHA-256 in lower-case
// hex. A token has 256 bits of entropy, so an unsalted fast hash is enough.
func HashInstallToken(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

// InstallTokenMatches compares a stored hash with the hash of a presented
// token in constant time.
func InstallTokenMatches(storedHash, presentedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(presentedHash)) == 1
}

// InstallPath is the path of a link's page: /app-updates/<code>/<key>/install/<token>.
func InstallPath(applicationCode, appKey, token string) string {
	return PublicPrefix + "/" + applicationCode + "/" + appKey + "/install/" + token
}

// InstallURL is a link's address under base (the platform's external URL).
func InstallURL(base, applicationCode, appKey, token string) string {
	return strings.TrimRight(base, "/") + InstallPath(applicationCode, appKey, token)
}
