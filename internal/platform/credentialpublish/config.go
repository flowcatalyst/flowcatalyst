// Package credentialpublish publishes a copy of a service account's or OAuth
// client's credentials to an external store, Parameter Store first
// (docs/credential-publishing.md). The credentials stay where they are; this
// package queues the copies in the same transaction as the change (QueueInTx)
// and a leader-gated worker writes them.
//
// It imports none of the aggregates it publishes for: they import it.
package credentialpublish

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// StoreAWSParameterStore is the only store built. It is kept in the
// publication row, so a later store does not move existing rows.
const StoreAWSParameterStore = "aws-ps"

// OwnerType is what a publication belongs to.
type OwnerType string

const (
	OwnerServiceAccount OwnerType = "SERVICE_ACCOUNT"
	OwnerOAuthClient    OwnerType = "OAUTH_CLIENT"
)

// Owner is one service account or OAuth client; ID is its sac_/oac_ row id.
type Owner struct {
	Type OwnerType
	ID   string
}

// The published credentials, which are also the last segment of each parameter.
const (
	CredClientID      = "client-id"
	CredClientSecret  = "client-secret"
	CredAuthToken     = "auth-token"
	CredSigningSecret = "signing-secret"
)

func validCredential(c string) bool {
	switch c {
	case CredClientID, CredClientSecret, CredAuthToken, CredSigningSecret:
		return true
	}
	return false
}

// Default templates (spec §2).
const (
	DefaultServiceAccountPath = "{prefix}service-accounts/{account}/{credential}"
	DefaultOAuthClientPath    = "{prefix}oauth-clients/{oauthClient}/{credential}"
)

const (
	// maxNameLen is Parameter Store's limit on a parameter name, path included.
	maxNameLen = 1011
	// maxDepth is its limit on the levels of a hierarchy.
	maxDepth = 15
	// longestCredential is the longest credential name, so a base path that
	// passes validation fits every parameter under it.
	longestCredential = len(CredSigningSecret)
)

// Config is the deployment's publishing settings (spec §2).
type Config struct {
	// Store is the store ("aws-ps"); empty means publishing is not offered.
	Store string
	// Prefix is what every published parameter must lie under, e.g.
	// "/flowcatalyst/np/". It starts and ends with '/'.
	Prefix string
	// ServiceAccountPath and OAuthClientPath are the path templates.
	ServiceAccountPath, OAuthClientPath string
	// DefaultOn is whether the option starts ticked on the create forms.
	DefaultOn bool
	// KMSKey is the key id or alias for the SecureStrings; empty is aws/ssm.
	KMSKey string
}

// ConfigFromEnv reads FC_CREDENTIAL_PUBLISH*. Unset FC_CREDENTIAL_PUBLISH is
// "not offered" and never an error. Once it is set, every other setting is
// checked and a bad one is an error: the server must refuse to start rather
// than publish somewhere the operator did not mean.
func ConfigFromEnv() (Config, error) {
	return configFromGetenv(os.Getenv)
}

func configFromGetenv(get func(string) string) (Config, error) {
	store := strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH"))
	if store == "" {
		return Config{}, nil
	}
	if store != StoreAWSParameterStore {
		return Config{}, fmt.Errorf("FC_CREDENTIAL_PUBLISH=%q: the only store is %q", store, StoreAWSParameterStore)
	}
	c := Config{
		Store:              store,
		Prefix:             strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH_PREFIX")),
		ServiceAccountPath: strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH")),
		OAuthClientPath:    strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH_OAUTH_CLIENT_PATH")),
		KMSKey:             strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH_KMS_KEY")),
	}
	if c.ServiceAccountPath == "" {
		c.ServiceAccountPath = DefaultServiceAccountPath
	}
	if c.OAuthClientPath == "" {
		c.OAuthClientPath = DefaultOAuthClientPath
	}
	switch strings.ToLower(strings.TrimSpace(get("FC_CREDENTIAL_PUBLISH_DEFAULT"))) {
	case "", "off":
	case "on":
		c.DefaultOn = true
	default:
		return Config{}, errors.New("FC_CREDENTIAL_PUBLISH_DEFAULT must be \"off\" or \"on\"")
	}
	if c.Prefix == "" {
		return Config{}, errors.New("FC_CREDENTIAL_PUBLISH_PREFIX is required when FC_CREDENTIAL_PUBLISH is set")
	}
	if err := checkPathShape(c.Prefix); err != nil {
		return Config{}, fmt.Errorf("FC_CREDENTIAL_PUBLISH_PREFIX: %w", err)
	}
	if err := checkTemplate(c.ServiceAccountPath, OwnerServiceAccount); err != nil {
		return Config{}, fmt.Errorf("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH: %w", err)
	}
	if err := checkTemplate(c.OAuthClientPath, OwnerOAuthClient); err != nil {
		return Config{}, fmt.Errorf("FC_CREDENTIAL_PUBLISH_OAUTH_CLIENT_PATH: %w", err)
	}
	return c, nil
}

// Enabled reports whether publishing is configured.
func (c Config) Enabled() bool { return c.Store != "" }

// Vars are the values a template's placeholders take.
type Vars struct {
	// Account is the service account code, App the code of its application
	// (empty when it has none), OAuthClient the OAuth client_id.
	Account, App, OAuthClient string
}

// placeholders are the names a template may use per owner type, besides
// {prefix}, which must open the template, and {credential}, which must end it.
var placeholders = map[OwnerType][]string{
	OwnerServiceAccount: {"account", "app"},
	OwnerOAuthClient:    {"oauthClient"},
}

// checkTemplate checks a template's shape: it starts with {prefix}, ends in
// {credential}, and every placeholder is a whole path segment that is known
// for the owner type. Same rule as FC_SECRET_REFERENCE_ALLOW's.
func checkTemplate(t string, ot OwnerType) error {
	rest, ok := strings.CutPrefix(t, "{prefix}")
	if !ok {
		return errors.New("must start with {prefix}, so every parameter lies under the prefix")
	}
	segs := strings.Split(rest, "/")
	if segs[len(segs)-1] != "{credential}" {
		return errors.New("must end in /{credential}")
	}
	for i, seg := range segs {
		switch {
		case seg == "":
			return errors.New("has an empty path segment")
		case strings.ContainsAny(seg, "{}"):
			name := strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "}")
			if seg != "{"+name+"}" || strings.ContainsAny(name, "{}") {
				return fmt.Errorf("placeholder segment %q must be one whole {name}", seg)
			}
			if name == "credential" && i == len(segs)-1 {
				continue
			}
			if !contains(placeholders[ot], name) {
				return fmt.Errorf("{%s} is not available here", name)
			}
		case !validSegment(seg):
			return fmt.Errorf("segment %q has characters Parameter Store does not allow", seg)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// BasePath renders the owner type's template without {credential}: the path
// before it, ending in '/'. It is an error when a placeholder the template
// uses has no value (an {app} template for an account without an application),
// or a value is not usable as a path segment.
func (c Config) BasePath(t OwnerType, v Vars) (string, error) {
	tmpl := c.ServiceAccountPath
	values := map[string]string{"account": v.Account, "app": v.App}
	if t == OwnerOAuthClient {
		tmpl = c.OAuthClientPath
		values = map[string]string{"oauthClient": v.OAuthClient}
	} else if t != OwnerServiceAccount {
		return "", fmt.Errorf("unknown owner type %q", t)
	}
	if tmpl == "" {
		tmpl = map[OwnerType]string{OwnerServiceAccount: DefaultServiceAccountPath, OwnerOAuthClient: DefaultOAuthClientPath}[t]
	}
	if err := checkTemplate(tmpl, t); err != nil {
		return "", err
	}
	rest, _ := strings.CutPrefix(tmpl, "{prefix}")
	rest = strings.TrimSuffix(rest, "{credential}")
	var b strings.Builder
	b.WriteString(c.Prefix)
	for seg := range strings.SplitSeq(strings.TrimSuffix(rest, "/"), "/") {
		if seg == "" {
			continue
		}
		if name, ok := strings.CutSuffix(strings.TrimPrefix(seg, "{"), "}"); ok && strings.HasPrefix(seg, "{") {
			val := values[name]
			if val == "" {
				return "", fmt.Errorf("the path needs {%s}, which has no value here: set a custom path", name)
			}
			if !validSegment(val) {
				return "", fmt.Errorf("{%s} %q cannot be used in a parameter name: set a custom path", name, val)
			}
			seg = val
		}
		b.WriteString(seg)
		b.WriteByte('/')
	}
	p := b.String()
	if err := c.ValidateBasePath(p); err != nil {
		return "", err
	}
	return p, nil
}

// ValidateBasePath checks a base path, a custom one or a rendered one: under
// Prefix, starts and ends with '/', only Parameter Store name characters, no
// empty segment, and short enough for the longest credential name.
func (c Config) ValidateBasePath(p string) error {
	if err := checkPathShape(p); err != nil {
		return err
	}
	if c.Prefix == "" || !strings.HasPrefix(p, c.Prefix) {
		return fmt.Errorf("the path must lie under %s", c.Prefix)
	}
	if len(p)+longestCredential > maxNameLen {
		return errors.New("the path is too long for a parameter name")
	}
	if strings.Count(p, "/") > maxDepth {
		return fmt.Errorf("the path is more than %d levels deep", maxDepth)
	}
	return nil
}

// checkPathShape checks what a prefix and a base path share.
func checkPathShape(p string) error {
	if !strings.HasPrefix(p, "/") || !strings.HasSuffix(p, "/") {
		return errors.New("must start and end with '/'")
	}
	for seg := range strings.SplitSeq(strings.Trim(p, "/"), "/") {
		if seg == "" {
			return errors.New("must not have an empty path segment")
		}
		if !validSegment(seg) {
			return fmt.Errorf("segment %q has characters Parameter Store does not allow (letters, digits, _ . -)", seg)
		}
	}
	return nil
}

// validSegment is Parameter Store's name alphabet for one path segment;
// "." and ".." are out, as a path traversal that AWS would refuse anyway.
func validSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
		default:
			return false
		}
	}
	return true
}

// Reference is the secret reference a function or ingestion uses to read the
// parameter at path back (docs/external-secret-references.md).
func Reference(path string) string { return StoreAWSParameterStore + "://" + path }
