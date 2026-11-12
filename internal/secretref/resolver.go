// Package secretref resolves external secret references ("aws-sm://...",
// "aws-ps://...") to the secret they point at, at read time, with a short
// cache (docs/external-secret-references.md). The platform stores the
// reference, never the secret, and resolves it each time it hands the secret
// to a consumer, so rotating the secret in AWS reaches every consumer without
// touching the platform.
//
// A reference is not a secret and may be logged and audited; a resolved value
// never is: nothing in this package logs or returns one in an error.
package secretref

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// The resolvable schemes. The write-side list of external schemes is
// encryption.EncryptSecretRef's; only these two resolve in this build.
const (
	SchemeSecretsManager = "aws-sm://"
	SchemeParameterStore = "aws-ps://"
)

// fetchTimeout bounds one AWS call. The call outlives the caller that started
// it (concurrent callers share its result), so the caller's own deadline
// cannot be the bound.
const fetchTimeout = 30 * time.Second

// Resolver turns a reference into the secret it points at. *Service is the
// production implementation; consumers take this so tests supply a fake.
type Resolver interface {
	// Resolve returns the secret ref points at, if the allow-list admits ref
	// for owner (the application and client the secret belongs to).
	Resolve(ctx context.Context, ref string, owner Owner) (string, error)
}

// IsReference reports whether v is a resolvable secret reference.
func IsReference(v string) bool {
	return strings.HasPrefix(v, SchemeSecretsManager) || strings.HasPrefix(v, SchemeParameterStore)
}

// source is one AWS object: a secret or a parameter, by name or ARN. Several
// references (the "#key" forms) share one.
type source struct {
	sm   bool // Secrets Manager, else Parameter Store
	name string
}

// parsed is a reference taken apart.
type parsed struct {
	source
	key    string
	hasKey bool
}

// parse takes a resolvable reference apart. An empty name is KEY_MISSING-free
// but NOT_FOUND: there is nothing to look up.
func parse(ref string) (parsed, *Error) {
	var p parsed
	switch {
	case strings.HasPrefix(ref, SchemeSecretsManager):
		p.sm = true
		p.name = strings.TrimPrefix(ref, SchemeSecretsManager)
		if i := strings.Index(p.name, "#"); i >= 0 {
			p.name, p.key, p.hasKey = p.name[:i], p.name[i+1:], true
		}
	case strings.HasPrefix(ref, SchemeParameterStore):
		p.name = strings.TrimPrefix(ref, SchemeParameterStore)
	default:
		return p, &Error{Code: CodeUnsupported, Ref: ref, Msg: "not an aws-sm:// or aws-ps:// reference"}
	}
	if p.name == "" {
		return p, &Error{Code: CodeNotFound, Ref: ref, Msg: "the reference names nothing"}
	}
	return p, nil
}

// region is the region the object lives in when its name is an ARN, else ""
// (the default chain's region).
func (s source) region() string { return RegionFromARN(s.name) }

// entry is the cache's record of one source.
type entry struct {
	value   string
	has     bool
	fetched time.Time
	// err and retryAfter record the last failed refresh: until retryAfter the
	// source is not asked again, and err is what a caller without a servable
	// value gets.
	err        *Error
	retryAfter time.Time
}

// flight is one in-progress AWS call, shared by every caller that missed on
// its source meanwhile.
type flight struct {
	done chan struct{}
	val  string
	err  *Error
}

// Service is the resolver: the allow-list, the AWS clients and the cache.
type Service struct {
	cfg     Config
	clients Clients
	now     func() time.Time

	mu      sync.Mutex
	entries map[source]entry
	flights map[source]*flight
}

// New builds a resolver. clients is only used when a reference misses the
// cache; production passes NewAWSClients().
func New(cfg Config, clients Clients) *Service {
	return &Service{
		cfg: cfg.withDefaults(), clients: clients, now: time.Now,
		entries: map[source]entry{}, flights: map[source]*flight{},
	}
}

var (
	defaultOnce sync.Once
	defaultSvc  *Service
	defaultMu   sync.RWMutex
)

// Default is the process's resolver, built on first use from the environment
// (ConfigFromEnv) and the default AWS credential chain; no AWS client is
// created until a reference is resolved.
func Default() *Service {
	defaultOnce.Do(func() {
		defaultMu.Lock()
		defer defaultMu.Unlock()
		if defaultSvc == nil {
			defaultSvc = New(ConfigFromEnv(), NewAWSClients())
		}
	})
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultSvc
}

// SetDefault replaces the process's resolver and returns a function that
// restores the previous one. For tests.
func SetDefault(s *Service) (restore func()) {
	prev := Default()
	defaultMu.Lock()
	defaultSvc = s
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		defaultSvc = prev
		defaultMu.Unlock()
	}
}

// TTL is how long a resolved value is served without asking AWS again.
func (s *Service) TTL() time.Duration { return s.cfg.TTL }

// Allowed reports whether the allow-list (FC_SECRET_REFERENCE_ALLOW) admits ref
// for owner: ref starts with a plain entry, or matches a scoped entry (one with
// {app}/{client}) at owner's own application and client (see Owner). It is the
// guard of the write (CheckWrite) and of every resolve, with the same owner, so
// a list narrowed after the write stops the old reference too, and a reference
// stored under one row does not resolve for another. An ARN reference matches
// on the ARN text: allowing by name does not allow by ARN.
func (s *Service) Allowed(ref string, owner Owner) error {
	if e := s.allowed(ref, owner); e != nil {
		return e
	}
	return nil
}

func (s *Service) allowed(ref string, owner Owner) *Error {
	return s.allowedBy(ref, func(entry string) bool { return matchEntry(entry, ref, owner, false) })
}

// allowedBy is the refusal when no entry satisfies match.
func (s *Service) allowedBy(ref string, match func(entry string) bool) *Error {
	scoped := false
	for _, p := range s.cfg.Allow {
		if match(p) {
			return nil
		}
		scoped = scoped || hasPlaceholder(p)
	}
	msg := "the platform allows no secret references (FC_SECRET_REFERENCE_ALLOW is empty)"
	if len(s.cfg.Allow) > 0 {
		msg = "a reference must start with one of: " + strings.Join(s.cfg.Allow, ", ")
		if scoped {
			msg += " ({app} and {client} stand for this application's and client's own path segment, so a reference outside what this application/client may use is refused)"
		}
	}
	return &Error{Code: CodeNotAllowed, Ref: ref, Msg: msg}
}

// Resolve returns the secret ref points at. The value is served from the
// cache while fresh; concurrent misses for one object share one AWS call; a
// failed refresh serves the last good value for up to MaxStale (WARN-logged)
// and a failure with nothing to serve is remembered for NegativeTTL. Errors
// are *Error.
func (s *Service) Resolve(ctx context.Context, ref string, owner Owner) (string, error) {
	ref = strings.TrimSpace(ref)
	p, perr := parse(ref)
	if perr != nil {
		resolveErrors.WithLabelValues(perr.Code).Inc()
		return "", perr
	}
	if e := s.allowed(ref, owner); e != nil {
		resolveErrors.WithLabelValues(CodeNotAllowed).Inc()
		return "", e
	}
	raw, ferr := s.get(ctx, ref, p.source)
	if ferr != nil {
		return "", ferr
	}
	if !p.hasKey {
		return raw, nil
	}
	v, kerr := pickKey(raw, p.key, ref)
	if kerr != nil {
		resolveErrors.WithLabelValues(kerr.Code).Inc()
		return "", kerr
	}
	return v, nil
}

// pickKey is field key of raw parsed as a JSON object: a string as is, a
// number or bool as its JSON text; a missing key, null, object or array is an
// error. The message never carries the field's value.
func pickKey(raw, key, ref string) (string, *Error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", &Error{Code: CodeKeyMissing, Ref: ref, Msg: "the secret is not a JSON object, so it has no key " + key}
	}
	field, ok := obj[key]
	if !ok {
		return "", &Error{Code: CodeKeyMissing, Ref: ref, Msg: "the secret has no key " + key}
	}
	text := strings.TrimSpace(string(field))
	switch {
	case strings.HasPrefix(text, `"`):
		var str string
		if err := json.Unmarshal(field, &str); err != nil {
			return "", &Error{Code: CodeKeyMissing, Ref: ref, Msg: "key " + key + " is not a readable string"}
		}
		return str, nil
	case text == "true" || text == "false":
		return text, nil
	case text != "" && (text[0] == '-' || text[0] >= '0' && text[0] <= '9'):
		return text, nil
	}
	return "", &Error{Code: CodeKeyMissing, Ref: ref, Msg: "key " + key + " is null, an object or an array, not a string, number or bool"}
}

// get returns the object's raw value through the cache.
func (s *Service) get(ctx context.Context, ref string, src source) (string, *Error) {
	s.mu.Lock()
	e := s.entries[src]
	now := s.now()
	if e.has && now.Sub(e.fetched) < s.cfg.TTL {
		s.mu.Unlock()
		resolves.WithLabelValues(outcomeHit).Inc()
		return e.value, nil
	}
	if now.Before(e.retryAfter) {
		// A refresh failed lately: do not ask AWS again yet.
		s.mu.Unlock()
		if e.has && now.Sub(e.fetched) <= s.cfg.MaxStale {
			resolves.WithLabelValues(outcomeStale).Inc()
			return e.value, nil
		}
		resolveErrors.WithLabelValues(e.err.Code).Inc()
		return "", &Error{Code: e.err.Code, Ref: ref, Msg: e.err.Msg, Err: e.err.Err}
	}
	fl, running := s.flights[src]
	if !running {
		fl = &flight{done: make(chan struct{})}
		s.flights[src] = fl
		go s.fetch(context.WithoutCancel(ctx), ref, src, fl)
	}
	s.mu.Unlock()

	select {
	case <-fl.done:
		if fl.err != nil {
			return "", &Error{Code: fl.err.Code, Ref: ref, Msg: fl.err.Msg, Err: fl.err.Err}
		}
		return fl.val, nil
	case <-ctx.Done():
		resolveErrors.WithLabelValues(CodeUnavailable).Inc()
		return "", &Error{Code: CodeUnavailable, Ref: ref, Msg: "the caller's deadline passed while the secret was being fetched", Err: ctx.Err()}
	}
}

// fetch asks AWS once for src and records the outcome for every waiter.
func (s *Service) fetch(ctx context.Context, ref string, src source, fl *flight) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	val, err := s.backend(ctx, ref, src)
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[src]
	switch {
	case err == nil:
		s.entries[src] = entry{value: val, has: true, fetched: now}
		fl.val = val
		resolves.WithLabelValues(outcomeMiss).Inc()
	case e.has && now.Sub(e.fetched) <= s.cfg.MaxStale:
		e.err, e.retryAfter = err, now.Add(s.cfg.NegativeTTL)
		s.entries[src] = e
		fl.val = e.value
		resolves.WithLabelValues(outcomeStale).Inc()
		slog.Warn("secret reference refresh failed; serving the last value",
			"reference", ref, "code", err.Code, "age", now.Sub(e.fetched).Round(time.Second).String(), "err", err)
	default:
		// Nothing to serve, or what there was is older than MaxStale.
		s.entries[src] = entry{err: err, retryAfter: now.Add(s.cfg.NegativeTTL)}
		fl.err = err
		resolveErrors.WithLabelValues(err.Code).Inc()
	}
	delete(s.flights, src)
	close(fl.done)
}

// backend reads src from AWS.
func (s *Service) backend(ctx context.Context, ref string, src source) (string, *Error) {
	if src.sm {
		c, err := s.clients.SecretsManager(ctx, src.region())
		if err != nil {
			return "", &Error{Code: CodeUnavailable, Ref: ref, Msg: "no Secrets Manager client", Err: err}
		}
		out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &src.name})
		if err != nil {
			return "", &Error{Code: classify(err), Ref: ref, Msg: "Secrets Manager refused or failed the read", Err: err}
		}
		if out.SecretString == nil {
			return "", &Error{Code: CodeNotFound, Ref: ref, Msg: "the secret has no string value"}
		}
		return *out.SecretString, nil
	}
	c, err := s.clients.SSM(ctx, src.region())
	if err != nil {
		return "", &Error{Code: CodeUnavailable, Ref: ref, Msg: "no Parameter Store client", Err: err}
	}
	decrypt := true
	out, err := c.GetParameter(ctx, &ssm.GetParameterInput{Name: &src.name, WithDecryption: &decrypt})
	if err != nil {
		return "", &Error{Code: classify(err), Ref: ref, Msg: "Parameter Store refused or failed the read", Err: err}
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", &Error{Code: CodeNotFound, Ref: ref, Msg: "the parameter has no value"}
	}
	return *out.Parameter.Value, nil
}
