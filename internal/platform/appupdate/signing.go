package appupdate

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
)

// AlgRSASHA256 is the protocol's only signature algorithm: RSASSA-PKCS1-v1_5
// with SHA-256 (CodeSigningAlgorithm.RSA_SHA256 in both clients).
const AlgRSASHA256 = "rsa-v1_5-sha256"

// DefaultClientKeyID is the key id a client asks for when its
// codeSigningMetadata names none (CODE_SIGNING_METADATA_DEFAULT_KEY_ID).
const DefaultClientKeyID = "root"

// MinRSABits is the smallest signing key accepted.
const MinRSABits = 2048

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidKeyID reports whether s can name a signing key: it travels as an SFV
// string in two headers, so it is kept to a plain alphabet.
func ValidKeyID(s string) bool { return keyIDPattern.MatchString(s) }

// Signing errors the public routes map to fixed answers.
var (
	// ErrNoKeys: no signing key is configured; the routes fail closed.
	ErrNoKeys = errors.New("app updates: no signing key configured")
	// ErrUnknownKey: the client asked for a key id the keyring does not hold.
	ErrUnknownKey = errors.New("app updates: unknown signing key id")
)

// keySource is where one key's PEM comes from.
type keySource struct {
	id string
	// kind is "file", "env" or "ref"; value the path, variable name or reference.
	kind, value string
}

// Keyring holds the platform's update-signing keys by key id. A key from a
// file or an environment variable is read once; one behind a secret reference
// (aws-sm://, aws-ps://) is resolved at each use through the platform's
// resolver, which caches it, so a value rotated in the secret store is picked
// up without a restart (a phone still verifies only against its embedded
// certificate: docs/app-updates-design.md section 7).
type Keyring struct {
	sources   map[string]keySource
	order     []string
	defaultID string
	refs      secretref.Resolver

	mu     sync.Mutex
	parsed map[string]parsedKey // key id -> last parsed PEM
}

type parsedKey struct {
	pemSum [32]byte
	key    *rsa.PrivateKey
}

// ParseKeySpec parses FC_APP_UPDATES_SIGNING_KEYS: a comma-separated list of
// keyid=source, where source is file:/path, env:NAME, aws-sm://... or
// aws-ps://... . defaultID names the default key ("" = the first). refs
// resolves secret references (nil = the process's resolver).
func ParseKeySpec(spec, defaultID string, refs secretref.Resolver) (*Keyring, error) {
	k := &Keyring{sources: map[string]keySource{}, refs: refs, parsed: map[string]parsedKey{}}
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, src, ok := strings.Cut(entry, "=")
		id, src = strings.TrimSpace(id), strings.TrimSpace(src)
		if !ok || !ValidKeyID(id) || src == "" {
			return nil, fmt.Errorf("app updates: signing key entry %q is not keyid=source (key ids: letters, digits, '.', '_', '-')", redactEntry(entry))
		}
		if _, dup := k.sources[id]; dup {
			return nil, fmt.Errorf("app updates: signing key id %q is listed twice", id)
		}
		var s keySource
		switch {
		case strings.HasPrefix(src, "file:"):
			s = keySource{id: id, kind: "file", value: strings.TrimPrefix(src, "file:")}
		case strings.HasPrefix(src, "env:"):
			s = keySource{id: id, kind: "env", value: strings.TrimPrefix(src, "env:")}
		case secretref.IsReference(src):
			s = keySource{id: id, kind: "ref", value: src}
		default:
			return nil, fmt.Errorf("app updates: signing key %q: source must be file:/path, env:NAME, or an aws-sm:// or aws-ps:// reference", id)
		}
		if s.value == "" {
			return nil, fmt.Errorf("app updates: signing key %q names no source", id)
		}
		k.sources[id] = s
		k.order = append(k.order, id)
	}
	if defaultID != "" {
		if _, ok := k.sources[defaultID]; !ok {
			return nil, fmt.Errorf("app updates: default signing key id %q is not in the keyring", defaultID)
		}
		k.defaultID = defaultID
	} else if len(k.order) > 0 {
		k.defaultID = k.order[0]
	}
	return k, nil
}

// redactEntry keeps a malformed entry out of an error when it looks like it
// holds key material instead of a source.
func redactEntry(e string) string {
	if strings.Contains(e, "PRIVATE KEY") || len(e) > 200 {
		return "(redacted)"
	}
	return e
}

// NewKeyringFromKeys is a keyring over keys already in memory (tests, and the
// dev server's generated key).
func NewKeyringFromKeys(defaultID string, keys map[string]*rsa.PrivateKey) *Keyring {
	k := &Keyring{sources: map[string]keySource{}, parsed: map[string]parsedKey{}, defaultID: defaultID}
	for id, key := range keys {
		k.sources[id] = keySource{id: id, kind: "memory"}
		k.parsed[id] = parsedKey{key: key}
		k.order = append(k.order, id)
	}
	return k
}

// Configured reports whether the keyring holds any key.
func (k *Keyring) Configured() bool { return k != nil && len(k.sources) > 0 }

// DefaultKeyID is the key used when a client asks for none.
func (k *Keyring) DefaultKeyID() string { return k.defaultID }

// Has reports whether the keyring holds key id.
func (k *Keyring) Has(id string) bool {
	if k == nil {
		return false
	}
	_, ok := k.sources[id]
	return ok
}

// Validate loads every key once, so a deployment learns at boot that a key is
// unreadable or too small rather than from the first phone.
func (k *Keyring) Validate(ctx context.Context) error {
	for _, id := range k.order {
		if _, err := k.key(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Sign signs body with key id and returns the expo-signature header value:
// sig="<base64>", keyid="<id>", alg="rsa-v1_5-sha256" (standard base64, the
// encoding Android's Base64.DEFAULT and iOS's Data(base64Encoded:) decode).
func (k *Keyring) Sign(ctx context.Context, id string, body []byte) (string, error) {
	key, err := k.key(ctx, id)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("app updates: sign: %w", err)
	}
	return SignatureHeader(base64.StdEncoding.EncodeToString(sig), id), nil
}

// SignatureHeader builds the expo-signature dictionary.
func SignatureHeader(sigB64, keyID string) string {
	return "sig=" + SFString(sigB64) + ", keyid=" + SFString(keyID) + ", alg=" + SFString(AlgRSASHA256)
}

// PublicKey is key id's public half (for tests and the certificate command).
func (k *Keyring) PublicKey(ctx context.Context, id string) (*rsa.PublicKey, error) {
	key, err := k.key(ctx, id)
	if err != nil {
		return nil, err
	}
	return &key.PublicKey, nil
}

func (k *Keyring) key(ctx context.Context, id string) (*rsa.PrivateKey, error) {
	if !k.Configured() {
		return nil, ErrNoKeys
	}
	src, ok := k.sources[id]
	if !ok {
		return nil, ErrUnknownKey
	}
	k.mu.Lock()
	cached, have := k.parsed[id]
	k.mu.Unlock()
	if src.kind == "memory" || (have && src.kind != "ref") {
		return cached.key, nil
	}
	pemText, err := k.read(ctx, src)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(pemText))
	if have && cached.pemSum == sum {
		return cached.key, nil
	}
	key, err := ParsePrivateKeyPEM([]byte(pemText))
	if err != nil {
		return nil, fmt.Errorf("app updates: signing key %q: %w", id, err)
	}
	k.mu.Lock()
	k.parsed[id] = parsedKey{pemSum: sum, key: key}
	k.mu.Unlock()
	return key, nil
}

func (k *Keyring) read(ctx context.Context, src keySource) (string, error) {
	switch src.kind {
	case "file":
		b, err := os.ReadFile(src.value)
		if err != nil {
			return "", fmt.Errorf("app updates: signing key %q: read %s: %w", src.id, src.value, err)
		}
		return string(b), nil
	case "env":
		v := os.Getenv(src.value)
		if v == "" {
			return "", fmt.Errorf("app updates: signing key %q: environment variable %s is empty", src.id, src.value)
		}
		return normalizePEM(v), nil
	case "ref":
		refs := k.refs
		if refs == nil {
			refs = secretref.Default()
		}
		v, err := refs.Resolve(ctx, src.value, secretref.Owner{})
		if err != nil {
			return "", fmt.Errorf("app updates: signing key %q: %w", src.id, err)
		}
		return normalizePEM(v), nil
	}
	return "", fmt.Errorf("app updates: signing key %q: unknown source", src.id)
}

// normalizePEM repairs a PEM carried in an environment variable or a secret
// string with literal "\n" escapes or surrounding quotes.
func normalizePEM(s string) string {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if strings.Contains(s, `\n`) {
		s = strings.ReplaceAll(strings.ReplaceAll(s, `\r\n`, "\n"), `\n`, "\n")
	}
	return s
}

// ParsePrivateKeyPEM reads an RSA private key (PKCS#1 or PKCS#8) of at least
// MinRSABits bits.
func ParsePrivateKeyPEM(b []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		key = k
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA key (the protocol signs with rsa-v1_5-sha256 only)")
		}
		key = rk
	default:
		return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
	}
	if key.N.BitLen() < MinRSABits {
		return nil, fmt.Errorf("RSA key of %d bits; at least %d are required", key.N.BitLen(), MinRSABits)
	}
	return key, nil
}

// ExpectSignature is what a client's expo-expect-signature header asks for.
type ExpectSignature struct {
	// KeyID is the key the client will verify with; "" when it named none.
	KeyID string
}

// Header parse errors the public routes map to fixed answers.
var (
	ErrBadExpectSignature = errors.New("app updates: malformed expo-expect-signature")
	ErrUnsupportedAlg     = errors.New("app updates: unsupported signature algorithm")
)

// ParseExpectSignature reads expo-expect-signature. ok is false when the
// header is absent. A key id outside the key-id alphabet is malformed; an
// algorithm other than rsa-v1_5-sha256 is unsupported.
func ParseExpectSignature(h string) (ExpectSignature, bool, error) {
	if strings.TrimSpace(h) == "" {
		return ExpectSignature{}, false, nil
	}
	d, err := ParseSFVDictionary(h)
	if err != nil {
		return ExpectSignature{}, true, ErrBadExpectSignature
	}
	var out ExpectSignature
	if v, ok := d["keyid"]; ok {
		if v.Kind != "string" || !ValidKeyID(v.Str) {
			return ExpectSignature{}, true, ErrBadExpectSignature
		}
		out.KeyID = v.Str
	}
	if v, ok := d["alg"]; ok {
		if v.Kind != "string" {
			return ExpectSignature{}, true, ErrBadExpectSignature
		}
		if v.Str != AlgRSASHA256 {
			return ExpectSignature{}, true, ErrUnsupportedAlg
		}
	}
	return out, true, nil
}
