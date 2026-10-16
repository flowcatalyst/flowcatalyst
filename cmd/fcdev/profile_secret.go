package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// keychainService is the OS secret-store service name; the account is the
// profile name.
const keychainService = "flowcatalyst-fcdev"

// Secret reference kinds.
const (
	refKeychain = "keychain"
	refEnv      = "env"
	refFile     = "file"
	refCommand  = "command"
)

// secretRef is a parsed secret_ref.
type secretRef struct {
	Kind string
	Arg  string   // env NAME or file path
	Argv []string // command
}

var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseSecretRef parses a reference. Errors never echo the input: a user
// who pasted a plaintext secret into secret_ref must not see it printed.
func parseSecretRef(s string) (secretRef, error) {
	const hint = "secret_ref must be keychain, env:NAME, file:/absolute/path or command:<argv...>"
	switch {
	case s == "" || s == refKeychain:
		return secretRef{Kind: refKeychain}, nil
	case strings.HasPrefix(s, "env:"):
		name := s[len("env:"):]
		if !envNameRE.MatchString(name) {
			return secretRef{}, errors.New("secret_ref env: needs a valid variable name (env:NAME)")
		}
		return secretRef{Kind: refEnv, Arg: name}, nil
	case strings.HasPrefix(s, "file:"):
		p := s[len("file:"):]
		if !filepath.IsAbs(p) {
			return secretRef{}, errors.New("secret_ref file: needs an absolute path (file:/path)")
		}
		return secretRef{Kind: refFile, Arg: filepath.Clean(p)}, nil
	case strings.HasPrefix(s, "command:"):
		argv, err := splitArgv(s[len("command:"):])
		if err != nil {
			return secretRef{}, fmt.Errorf("secret_ref command: %w", err)
		}
		if len(argv) == 0 {
			return secretRef{}, errors.New("secret_ref command: needs a program")
		}
		return secretRef{Kind: refCommand, Argv: argv}, nil
	}
	return secretRef{}, errors.New(hint)
}

// String is the canonical reference text (used for display; command shows
// only its program, since arguments could carry anything).
func (r secretRef) String() string {
	switch r.Kind {
	case refEnv:
		return "env:" + r.Arg
	case refFile:
		return "file:" + r.Arg
	case refCommand:
		return "command:" + r.Argv[0] + " …"
	}
	return refKeychain
}

// splitArgv splits a command into argv WITHOUT a shell: whitespace separates
// words; '...' and "..." group (backslash escapes only inside "..."); there
// is no expansion, globbing, piping or redirection — those characters are
// literal.
func splitArgv(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == 0 && (c == ' ' || c == '\t'):
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		case q == 0 && (c == '"' || c == '\''):
			q, in = c, true
		case q != 0 && c == q:
			q = 0
		case q == '"' && c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\'):
			i++
			cur.WriteByte(s[i])
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if q != 0 {
		return nil, errors.New("unterminated quote")
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

// --- Keychain ------------------------------------------------------------

// ErrKeychainNotFound means the account has no entry.
var ErrKeychainNotFound = errors.New("no secret stored in the keychain for this profile")

// Keychain is the OS secret store, keyed by profile name.
type Keychain interface {
	Get(account string) (string, error)
	Set(account, secret string) error
	Delete(account string) error
}

// fnKeychain is the store in use; tests replace it with a fake.
var fnKeychain Keychain = osKeychain{}

// osKeychain is macOS Keychain, Windows Credential Manager or Linux Secret
// Service via github.com/zalando/go-keyring.
type osKeychain struct{}

func (osKeychain) Get(account string) (string, error) {
	s, err := keyring.Get(keychainService, account)
	return s, mapKeyringErr(err)
}

func (osKeychain) Set(account, secret string) error {
	return mapKeyringErr(keyring.Set(keychainService, account, secret))
}

func (osKeychain) Delete(account string) error {
	return mapKeyringErr(keyring.Delete(keychainService, account))
}

func mapKeyringErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrKeychainNotFound
	case runtime.GOOS == "linux":
		return fmt.Errorf("the OS secret store (Secret Service / libsecret over D-Bus) is unavailable: %w — on a headless or CI machine use a secret_ref of env:NAME, file:/path or command:<argv...> instead of the keychain", err)
	}
	return fmt.Errorf("OS keychain: %w", err)
}

// --- resolving -----------------------------------------------------------

// fnRunSecretCommand runs argv without a shell and returns stdout. A var so
// tests can intercept it; the default is the real thing.
var fnRunSecretCommand = runSecretCommand

const secretCommandTimeout = 30 * time.Second

func runSecretCommand(ctx context.Context, argv []string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, secretCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- the profile owner chose this command; documented risk
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

// resolveSecret fetches the secret a profile's reference points at. The
// returned error never contains the secret.
func resolveSecret(ctx context.Context, p *Profile) (string, error) {
	ref, err := parseSecretRef(p.SecretRef)
	if err != nil {
		return "", fmt.Errorf("profile %s: %w", p.Name, err)
	}
	var secret string
	switch ref.Kind {
	case refKeychain:
		secret, err = fnKeychain.Get(p.Name)
		if errors.Is(err, ErrKeychainNotFound) {
			return "", fmt.Errorf("profile %s: no secret in the keychain — run: fcdev profile set-secret %s", p.Name, p.Name)
		}
		if err != nil {
			return "", fmt.Errorf("profile %s: %w", p.Name, err)
		}
	case refEnv:
		secret = os.Getenv(ref.Arg)
		if secret == "" {
			return "", fmt.Errorf("profile %s: environment variable %s is not set (or empty)", p.Name, ref.Arg)
		}
	case refFile:
		secret, err = readSecretFile(ref.Arg)
		if err != nil {
			return "", fmt.Errorf("profile %s: %w", p.Name, err)
		}
	case refCommand:
		stdout, stderr, runErr := fnRunSecretCommand(ctx, ref.Argv)
		if runErr != nil {
			msg := strings.TrimSpace(stderr)
			if msg != "" {
				msg = ": " + scrubText(msg, stdout)
			}
			return "", fmt.Errorf("profile %s: secret command %q failed (%s)%s", p.Name, ref.Argv[0], scrubText(runErr.Error(), stdout), msg)
		}
		secret = stdout
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("profile %s: the secret reference resolved to an empty value", p.Name)
	}
	return secret, nil
}

// readSecretFile reads a secret file, refusing one that group/other can
// access.
func readSecretFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret file %s is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret file %s is accessible by group/other (mode %04o); refusing — run: chmod 600 %s", path, info.Mode().Perm(), path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret file: %w", err)
	}
	return string(b), nil
}

// scrubText removes every occurrence of each secret from s.
func scrubText(s string, secrets ...string) string {
	for _, sec := range secrets {
		sec = strings.TrimSpace(sec)
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "***")
		}
	}
	return s
}

// scrubErr returns err with secrets removed from its message (the chain is
// kept only when nothing needed removing).
func scrubErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if clean := scrubText(msg, secrets...); clean != msg {
		return errors.New(clean)
	}
	return err
}
