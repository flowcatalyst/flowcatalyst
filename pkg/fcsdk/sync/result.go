package sync

// CategoryResult is the per-category outcome of a Synchronizer.Sync.
//
// Failures are captured here rather than propagated as errors so the
// caller can see exactly which categories succeeded, which failed, and
// what was touched on the platform.
type CategoryResult struct {
	Created     uint32
	Updated     uint32
	Deleted     uint32 // archived, for scheduled jobs
	SyncedCodes []string
	// PasswordHashIgnored (principals): emails whose passwordHash was not
	// applied because the user already existed.
	PasswordHashIgnored []string
	// SkippedRoles (principals): role names that are not roles, so were
	// assigned to no one; the rest of the sync applied. Usually a typo or a
	// role not synced yet.
	SkippedRoles []string
	// Error is non-empty if this category's HTTP call (or local check)
	// failed.
	Error string
}

// IsError reports whether this category failed.
func (c *CategoryResult) IsError() bool { return c.Error != "" }

// Touched is Created + Updated + Deleted — useful for one-line summaries.
func (c *CategoryResult) Touched() uint32 { return c.Created + c.Updated + c.Deleted }

// Result is the aggregate outcome of syncing a DefinitionSet. Each
// per-category field is non-nil only if the Synchronizer called that
// endpoint (i.e. the category was enabled in Options AND non-empty).
type Result struct {
	ApplicationCode string
	Roles           *CategoryResult
	EventTypes      *CategoryResult
	DispatchPools   *CategoryResult
	Connections     *CategoryResult
	Subscriptions   *CategoryResult
	Processes       *CategoryResult
	ScheduledJobs   *CategoryResult
	Principals      *CategoryResult
	OpenAPI         *CategoryResult
	Docs            *CategoryResult
}

func (r *Result) categories() []struct {
	name string
	res  *CategoryResult
} {
	return []struct {
		name string
		res  *CategoryResult
	}{
		{"roles", r.Roles},
		{"event_types", r.EventTypes},
		{"dispatch_pools", r.DispatchPools},
		{"connections", r.Connections},
		{"subscriptions", r.Subscriptions},
		{"processes", r.Processes},
		{"scheduled_jobs", r.ScheduledJobs},
		{"principals", r.Principals},
		{"openapi", r.OpenAPI},
		{"docs", r.Docs},
	}
}

// HasErrors reports whether any category failed.
func (r *Result) HasErrors() bool {
	for _, c := range r.categories() {
		if c.res != nil && c.res.IsError() {
			return true
		}
	}
	return false
}

// Errors returns a (category → message) list of failed categories.
func (r *Result) Errors() map[string]string {
	out := map[string]string{}
	for _, c := range r.categories() {
		if c.res != nil && c.res.IsError() {
			out[c.name] = c.res.Error
		}
	}
	return out
}
