package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Profiles name a remote platform plus HOW to obtain its client secret.
// The profile file never holds a secret value: only a reference (see
// secretref.go). docs/fcdev-profiles.md.

// fnProfilesEnv overrides the profile file's location.
const fnProfilesEnv = "FC_PROFILES_PATH"

// Environment tags a profile carries. prod gates mutating commands behind a
// typed confirmation.
const (
	envLocal = "local"
	envNP    = "np"
	envProd  = "prod"
)

// How a profile authenticates.
const (
	// authClientCredentials is the default: an OAuth client id plus a secret
	// held by reference (CI, service accounts).
	authClientCredentials = "client_credentials"
	// authLogin is a person: `fcdev login` runs the browser sign-in and the
	// resulting session lives in the OS keychain. The profile holds no client
	// id and no secret reference.
	authLogin = "login"
)

// Profile is one named connection target.
type Profile struct {
	Name                string
	Platform            string // platform base URL
	Runner              string // optional; how functions are invoked
	Env                 string // local | np | prod
	Auth                string // "" / client_credentials | login
	ClientID            string // client_credentials only
	SecretRef           string // client_credentials only: keychain | env:NAME | file:/path | command:argv...
	AllowNoninteractive bool   // prod: allow --yes without a terminal (CI)
}

// IsLogin reports a person-login profile.
func (p *Profile) IsLogin() bool { return p.Auth == authLogin }

var profileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// fnProfilesPath is where the profile file lives.
func fnProfilesPath() (string, error) {
	if p := os.Getenv(fnProfilesEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the user config directory (set %s): %w", fnProfilesEnv, err)
	}
	return filepath.Join(dir, "flowcatalyst", "profiles.toml"), nil
}

// validateProfile checks a profile's fields. Errors never echo a secret-ref
// value (it might be a pasted plaintext secret).
func validateProfile(p *Profile) error {
	if !profileNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid profile name %q: use letters, digits, '-' and '_' (max 64)", p.Name)
	}
	if err := validateEndpointURL(p.Platform); err != nil {
		return fmt.Errorf("profile %s: platform: %w", p.Name, err)
	}
	if p.Runner != "" {
		if err := validateEndpointURL(p.Runner); err != nil {
			return fmt.Errorf("profile %s: runner: %w", p.Name, err)
		}
	}
	switch p.Env {
	case envLocal, envNP, envProd:
	default:
		return fmt.Errorf("profile %s: env must be local, np or prod", p.Name)
	}
	switch p.Auth {
	case "", authClientCredentials:
	case authLogin:
		if p.ClientID != "" || p.SecretRef != "" {
			return fmt.Errorf("profile %s: a login profile (auth = \"login\") takes no client_id or secret_ref: the session comes from `fcdev login --profile %s`", p.Name, p.Name)
		}
		return nil
	default:
		return fmt.Errorf("profile %s: auth must be %q or %q", p.Name, authClientCredentials, authLogin)
	}
	if p.ClientID == "" {
		return fmt.Errorf("profile %s: client_id is required", p.Name)
	}
	if _, err := parseSecretRef(p.SecretRef); err != nil {
		return fmt.Errorf("profile %s: %w", p.Name, err)
	}
	return nil
}

// validateEndpointURL requires an absolute http(s) URL with no credentials
// in it, and https unless the host is loopback.
func validateEndpointURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("not an absolute URL")
	}
	if u.User != nil {
		return errors.New("must not embed credentials (user:password@)")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return errors.New("https is required for a non-loopback host")
		}
	default:
		return errors.New("scheme must be https (or http for loopback)")
	}
	return nil
}

// isLoopbackHost reports localhost and loopback IP literals.
func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// isLoopbackURL reports whether a URL's host is loopback. Unparseable or
// host-less values are NOT loopback.
func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// redactURL strips any userinfo so a URL can go into a message.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable URL)"
	}
	u.User = nil
	return u.String()
}

// checkProfileFileMode refuses a profile file that other users can write:
// it can name a command: reference, so whoever can edit it can run code as
// us. No-op on Windows (POSIX mode bits are not meaningful there).
func checkProfileFileMode(path string, info os.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is group/other-writable (mode %04o); it can name commands to run, so refusing it — run: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	return nil
}

// profileSet is the loaded file.
type profileSet map[string]*Profile

func (s profileSet) names() []string {
	out := make([]string, 0, len(s))
	for n := range s {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// loadProfiles reads the profile file. A missing file is an empty set.
func loadProfiles() (profileSet, string, error) {
	path, err := fnProfilesPath()
	if err != nil {
		return nil, "", err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return profileSet{}, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	if !info.Mode().IsRegular() {
		return nil, path, fmt.Errorf("%s is not a regular file", path)
	}
	if err := checkProfileFileMode(path, info); err != nil {
		return nil, path, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	set, err := parseProfiles(string(b))
	if err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return set, path, nil
}

// saveProfiles rewrites the file atomically (0600, directory 0700). Comments
// in a hand-edited file are not preserved.
func saveProfiles(path string, set profileSet) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := formatProfiles(set)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".profiles-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// --- a deliberately tiny TOML subset -------------------------------------
//
// Decision: no TOML parser is in go.mod, and the file needs only tables,
// strings and booleans, so a strict ~100 line subset parser is cheaper and
// safer than a new dependency. It accepts:
//
//	# comment
//	[profiles.NAME]
//	key = "basic string"   (escapes: \\ \" \n \t \r)
//	key = 'literal string'
//	key = true | false
//
// and rejects everything else (arrays, inline tables, numbers, dotted keys,
// multi-line strings) with a line number but never the offending text.

const profileHeaderPrefix = "profiles."

var profileKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var secretishKeyRE = regexp.MustCompile(`(?i)(secret|passw|token|credential|apikey|api_key|private)`)

// allowed keys and whether they hold a boolean.
var profileKeys = map[string]bool{
	"platform": false, "runner": false, "env": false, "auth": false, "client_id": false,
	"secret_ref": false, "allow_noninteractive": true,
}

func parseProfiles(src string) (profileSet, error) {
	set := profileSet{}
	var cur *Profile
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			rest, _, err := stripTrailingComment(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n, err)
			}
			if !strings.HasSuffix(rest, "]") || !strings.HasPrefix(rest, "["+profileHeaderPrefix) {
				return nil, fmt.Errorf("line %d: only [profiles.NAME] tables are supported", n)
			}
			name := rest[len("["+profileHeaderPrefix) : len(rest)-1]
			if !profileNameRE.MatchString(name) {
				return nil, fmt.Errorf("line %d: invalid profile name in table header", n)
			}
			if seen[name] {
				return nil, fmt.Errorf("line %d: duplicate profile %q", n, name)
			}
			seen[name] = true
			cur = &Profile{Name: name}
			set[name] = cur
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !profileKeyRE.MatchString(key) {
			return nil, fmt.Errorf("line %d: expected key = value", n)
		}
		if cur == nil {
			return nil, fmt.Errorf("line %d: key %q outside a [profiles.NAME] table", n, key)
		}
		isBool, known := profileKeys[key]
		if !known {
			if secretishKeyRE.MatchString(key) {
				return nil, fmt.Errorf("line %d: key %q looks like a secret; profiles hold only a secret_ref (keychain, env:NAME, file:/path or command:...), never a value", n, key)
			}
			return nil, fmt.Errorf("line %d: unknown key %q", n, key)
		}
		val, _, err := stripTrailingComment(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if isBool {
			switch val {
			case "true":
				cur.AllowNoninteractive = true
			case "false":
				cur.AllowNoninteractive = false
			default:
				return nil, fmt.Errorf("line %d: %s must be true or false", n, key)
			}
			continue
		}
		s, err := parseTOMLString(val)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", n, key, err)
		}
		switch key {
		case "platform":
			cur.Platform = s
		case "runner":
			cur.Runner = s
		case "env":
			cur.Env = s
		case "auth":
			cur.Auth = s
		case "client_id":
			cur.ClientID = s
		case "secret_ref":
			cur.SecretRef = s
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for _, name := range set.names() {
		p := set[name]
		if p.SecretRef == "" && !p.IsLogin() {
			p.SecretRef = refKeychain
		}
		if err := validateProfile(p); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// stripTrailingComment removes a "# comment" that follows a value, leaving
// '#' inside a quoted string alone.
func stripTrailingComment(s string) (string, bool, error) {
	var quote rune
	esc := false
	for i, r := range s {
		switch {
		case quote == '"' && esc:
			esc = false
		case quote == '"' && r == '\\':
			esc = true
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote = r
		case quote == 0 && r == '#':
			return strings.TrimSpace(s[:i]), true, nil
		}
	}
	if quote != 0 {
		return "", false, errors.New("unterminated string")
	}
	return s, false, nil
}

func parseTOMLString(v string) (string, error) {
	if len(v) < 2 {
		return "", errors.New("expected a quoted string")
	}
	q := v[0]
	if (q != '"' && q != '\'') || v[len(v)-1] != q {
		return "", errors.New("expected a quoted string")
	}
	body := v[1 : len(v)-1]
	if q == '\'' {
		if strings.ContainsRune(body, '\'') {
			return "", errors.New("malformed string")
		}
		return body, nil
	}
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"':
			return "", errors.New("malformed string")
		case c != '\\':
			b.WriteByte(c)
		case i+1 >= len(body):
			return "", errors.New("malformed string")
		default:
			i++
			switch body[i] {
			case '\\', '"':
				b.WriteByte(body[i])
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", errors.New("unsupported escape")
			}
		}
	}
	return b.String(), nil
}

func quoteTOMLString(s string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			return "", errors.New("control characters are not allowed in profile values")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

func formatProfiles(set profileSet) (string, error) {
	var b strings.Builder
	b.WriteString("# fcdev profiles. Holds secret REFERENCES only, never a secret value.\n# See docs/fcdev-profiles.md. Keep this file owner-writable only (chmod 600).\n")
	for _, name := range set.names() {
		p := set[name]
		fmt.Fprintf(&b, "\n[%s%s]\n", profileHeaderPrefix, name)
		for _, kv := range [][2]string{{"platform", p.Platform}, {"runner", p.Runner}, {"env", p.Env}, {"auth", p.Auth}, {"client_id", p.ClientID}, {"secret_ref", p.SecretRef}} {
			if kv[1] == "" {
				continue
			}
			q, err := quoteTOMLString(kv[1])
			if err != nil {
				return "", fmt.Errorf("profile %s: %s: %w", name, kv[0], err)
			}
			fmt.Fprintf(&b, "%s = %s\n", kv[0], q)
		}
		if p.AllowNoninteractive {
			b.WriteString("allow_noninteractive = true\n")
		}
	}
	return b.String(), nil
}
