// Package sync orchestrates declarative reconciliation of an
// application's FlowCatalyst definitions against the platform.
//
// Build a DefinitionSet with the per-category fluent builders, then
// hand it to Synchronizer.Sync. Each category becomes one HTTP call
// against the platform's per-resource sync endpoint
// (POST /api/applications/{appCode}/<resource>/sync). Failures on one
// category do NOT abort the rest — they're captured on the returned
// SyncResult so the caller sees a complete picture in one round-trip.
//
// The SDKs converge
// on one orchestration vocabulary for this package's shape.
package sync

import "encoding/json"

// RoleDefinition declares a role for an application. Name is the short
// name without the `<app>:` prefix — the platform adds it from the sync
// URL. Set ClientManaged true to let clients re-assign or remove the
// role per-tenant.
type RoleDefinition struct {
	Name          string
	DisplayName   string
	Description   string
	Permissions   []string
	ClientManaged bool
}

// MakeRole starts a RoleDefinition with the given short name.
func MakeRole(name string) RoleDefinition { return RoleDefinition{Name: name} }

func (r RoleDefinition) WithDisplayName(name string) RoleDefinition { r.DisplayName = name; return r }
func (r RoleDefinition) WithDescription(d string) RoleDefinition    { r.Description = d; return r }
func (r RoleDefinition) WithPermissions(perms ...string) RoleDefinition {
	r.Permissions = append(r.Permissions, perms...)
	return r
}
func (r RoleDefinition) ClientManagedEnabled() RoleDefinition { r.ClientManaged = true; return r }

// EventTypeDefinition declares an event type.
// Code must be the full four-segment identifier
// {app}:{subdomain}:{aggregate}:{event}.
type EventTypeDefinition struct {
	Code        string
	Name        string
	Description string
}

func MakeEventType(code, name string) EventTypeDefinition {
	return EventTypeDefinition{Code: code, Name: name}
}

func (e EventTypeDefinition) WithDescription(d string) EventTypeDefinition {
	e.Description = d
	return e
}

// SubscriptionBinding pairs an event type pattern with an optional filter.
type SubscriptionBinding struct {
	EventTypeCode string
	Filter        string
}

// SubscriptionDefinition declares a webhook subscription.
type SubscriptionDefinition struct {
	Code        string
	Name        string
	Description string
	Target      string // webhook endpoint URL
	// ConnectionCode names the delivering connection by code (stable across
	// environments; prefer it to ConnectionID). It resolves among this
	// application's connections, or the shared ones when SharedConnection
	// is set. Connections are synced before subscriptions in the same run.
	ConnectionCode   string
	SharedConnection bool
	ConnectionID     string // environment-specific; prefer ConnectionCode
	DispatchPoolCode string
	Mode             string // "Immediate" | "BlockOnError" — see platform docs
	MaxRetries       *uint32
	TimeoutSeconds   *uint32
	DataOnly         bool
	EventTypes       []SubscriptionBinding
}

func MakeSubscription(code, name, target string) SubscriptionDefinition {
	return SubscriptionDefinition{Code: code, Name: name, Target: target}
}

func (s SubscriptionDefinition) WithDescription(d string) SubscriptionDefinition {
	s.Description = d
	return s
}

// WithConnection names the delivering connection by id. Ids differ per
// environment: prefer WithConnectionCode for anything synced to more than one.
func (s SubscriptionDefinition) WithConnection(id string) SubscriptionDefinition {
	s.ConnectionID = id
	return s
}

// WithConnectionCode names the delivering connection by code, one of this
// application's connections.
func (s SubscriptionDefinition) WithConnectionCode(code string) SubscriptionDefinition {
	s.ConnectionCode = code
	return s
}

// WithSharedConnectionCode names a shared (application-less) connection by
// code. There is no fallback between the two namespaces.
func (s SubscriptionDefinition) WithSharedConnectionCode(code string) SubscriptionDefinition {
	s.ConnectionCode, s.SharedConnection = code, true
	return s
}

func (s SubscriptionDefinition) WithDispatchPool(code string) SubscriptionDefinition {
	s.DispatchPoolCode = code
	return s
}
func (s SubscriptionDefinition) WithMode(m string) SubscriptionDefinition { s.Mode = m; return s }
func (s SubscriptionDefinition) WithMaxRetries(n uint32) SubscriptionDefinition {
	s.MaxRetries = &n
	return s
}

func (s SubscriptionDefinition) WithTimeout(secs uint32) SubscriptionDefinition {
	s.TimeoutSeconds = &secs
	return s
}

func (s SubscriptionDefinition) DataOnlyEnabled() SubscriptionDefinition { s.DataOnly = true; return s }

func (s SubscriptionDefinition) Bind(eventTypeCode, filter string) SubscriptionDefinition {
	s.EventTypes = append(s.EventTypes, SubscriptionBinding{EventTypeCode: eventTypeCode, Filter: filter})
	return s
}

// DispatchPoolDefinition declares a dispatch pool.
type DispatchPoolDefinition struct {
	Code        string
	Name        string
	Description string
	Concurrency uint32
	// RateLimit is messages per minute; nil for concurrency-only.
	RateLimit *uint32
}

func MakeDispatchPool(code, name string) DispatchPoolDefinition {
	return DispatchPoolDefinition{Code: code, Name: name}
}

func (d DispatchPoolDefinition) WithDescription(s string) DispatchPoolDefinition {
	d.Description = s
	return d
}

func (d DispatchPoolDefinition) WithConcurrency(c uint32) DispatchPoolDefinition {
	d.Concurrency = c
	return d
}

// WithRateLimit caps the pool at perMinute messages per minute.
func (d DispatchPoolDefinition) WithRateLimit(perMinute uint32) DispatchPoolDefinition {
	d.RateLimit = &perMinute
	return d
}

// PrincipalDefinition declares a user principal. Matched by Email on
// sync. Active defaults to true (set Inactive() to override).
type PrincipalDefinition struct {
	Email  string
	Name   string
	Roles  []string
	Active bool
	// PasswordHash, when set, carries an already-hashed credential (e.g. an
	// upstream Laravel bcrypt/argon2i hash) so a migrated user keeps their
	// password. Sent verbatim; the platform verifies it and re-encodes it to
	// its native scheme on first login. Use WithPasswordHash to set it.
	PasswordHash *string
}

// MakePrincipal starts a PrincipalDefinition for the given email. The
// principal defaults to Active.
func MakePrincipal(email string) PrincipalDefinition {
	return PrincipalDefinition{Email: email, Active: true}
}
func (p PrincipalDefinition) WithName(n string) PrincipalDefinition { p.Name = n; return p }
func (p PrincipalDefinition) WithRoles(roles ...string) PrincipalDefinition {
	p.Roles = append(p.Roles, roles...)
	return p
}

// WithPasswordHash attaches an already-hashed credential (e.g. the value of a
// Laravel user's password column). Pass the HASH, not a plaintext password —
// it is stored verbatim and verified as-is at login.
func (p PrincipalDefinition) WithPasswordHash(hash string) PrincipalDefinition {
	p.PasswordHash = &hash
	return p
}

// Inactive marks the principal as inactive on sync.
func (p PrincipalDefinition) Inactive() PrincipalDefinition { p.Active = false; return p }

// ProcessDefinition declares a process documentation entry. Code is
// {application}:{subdomain}:{process-name}; Body is the diagram source
// (stored verbatim, typically Mermaid); DiagramType defaults to mermaid.
type ProcessDefinition struct {
	Code        string
	Name        string
	Description string
	Body        string
	DiagramType string
	Tags        []string
}

func MakeProcess(code, name string) ProcessDefinition {
	return ProcessDefinition{Code: code, Name: name}
}

func (p ProcessDefinition) WithDescription(s string) ProcessDefinition { p.Description = s; return p }

// WithBody sets the diagram source.
func (p ProcessDefinition) WithBody(body string) ProcessDefinition { p.Body = body; return p }

// WithDiagramType sets the diagram language (default mermaid).
func (p ProcessDefinition) WithDiagramType(t string) ProcessDefinition { p.DiagramType = t; return p }

// WithTags appends tags.
func (p ProcessDefinition) WithTags(tags ...string) ProcessDefinition {
	p.Tags = append(append([]string(nil), p.Tags...), tags...)
	return p
}

// ConnectionDefinition declares a connection. It carries nothing
// environment-specific (the platform assigns the application's own service
// account), so one definition serves every environment; it exists so a
// subscription's connection code resolves. Connections are synced before
// subscriptions.
type ConnectionDefinition struct {
	Code        string
	Name        string
	Description string
	ExternalID  string
}

func MakeConnection(code, name string) ConnectionDefinition {
	return ConnectionDefinition{Code: code, Name: name}
}

func (c ConnectionDefinition) WithDescription(d string) ConnectionDefinition {
	c.Description = d
	return c
}

// WithExternalID records your own system's identifier for the connection.
func (c ConnectionDefinition) WithExternalID(id string) ConnectionDefinition {
	c.ExternalID = id
	return c
}

// ScheduledJobDefinition declares a scheduled job. Crons are evaluated in
// Timezone (default UTC). Concurrent lets a new firing start while the
// previous one runs; TracksCompletion makes the platform wait for the
// application's completion report (see package scheduledjobs) instead of
// treating delivery as success. ClientID scopes the job to one client (omit
// for a platform-wide job, anchor only); jobs are sent one request per
// client.
type ScheduledJobDefinition struct {
	Code                string
	Name                string
	Description         string
	Crons               []string
	Timezone            string
	Payload             json.RawMessage
	Concurrent          bool
	TracksCompletion    bool
	TimeoutSeconds      *int32
	DeliveryMaxAttempts *int32 // platform default 3
	TargetURL           string // overrides the application's default callback URL
	ClientID            string
}

// MakeScheduledJob starts a job with its cron expressions.
func MakeScheduledJob(code, name string, crons ...string) ScheduledJobDefinition {
	return ScheduledJobDefinition{Code: code, Name: name, Crons: crons}
}

func (j ScheduledJobDefinition) WithDescription(d string) ScheduledJobDefinition {
	j.Description = d
	return j
}

func (j ScheduledJobDefinition) WithTimezone(tz string) ScheduledJobDefinition {
	j.Timezone = tz
	return j
}

func (j ScheduledJobDefinition) WithPayload(p json.RawMessage) ScheduledJobDefinition {
	j.Payload = p
	return j
}

func (j ScheduledJobDefinition) ConcurrentEnabled() ScheduledJobDefinition {
	j.Concurrent = true
	return j
}

func (j ScheduledJobDefinition) TracksCompletionEnabled() ScheduledJobDefinition {
	j.TracksCompletion = true
	return j
}

func (j ScheduledJobDefinition) WithTimeout(secs int32) ScheduledJobDefinition {
	j.TimeoutSeconds = &secs
	return j
}

func (j ScheduledJobDefinition) WithDeliveryMaxAttempts(n int32) ScheduledJobDefinition {
	j.DeliveryMaxAttempts = &n
	return j
}

func (j ScheduledJobDefinition) WithTargetURL(u string) ScheduledJobDefinition {
	j.TargetURL = u
	return j
}

func (j ScheduledJobDefinition) ForClient(id string) ScheduledJobDefinition {
	j.ClientID = id
	return j
}

// DocDefinition declares one documentation page, shown to administrators
// under Platform → Documentation. Slug is kebab-case and unique within the
// application; Content is Markdown; Title defaults to the first "# "
// heading. The docs sync is a full replace: the set's pages ARE the
// application's pages.
type DocDefinition struct {
	Slug    string
	Title   string
	Content string
}

func MakeDoc(slug, content string) DocDefinition { return DocDefinition{Slug: slug, Content: content} }

func (d DocDefinition) WithTitle(t string) DocDefinition { d.Title = t; return d }
