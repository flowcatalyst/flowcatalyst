package sync

// Options controls a Synchronizer.Sync call.
//
// RemoveUnlisted toggles ?removeUnlisted=true on every per-category endpoint
// that has it (and archiveUnlisted on the scheduled-jobs sync): the platform
// archives/removes the SDK-sourced rows that aren't in the submitted list.
// UI-sourced rows are never touched. Default false. The docs sync is always
// a full replace, and the OpenAPI sync publishes one document; neither reads
// it.
//
// SyncX flags are per-category skip switches. Empty categories on the
// DefinitionSet are also implicitly skipped regardless of these flags.
type Options struct {
	RemoveUnlisted    bool
	SyncRoles         bool
	SyncEventTypes    bool
	SyncDispatchPools bool
	SyncConnections   bool
	SyncSubscriptions bool
	SyncProcesses     bool
	SyncScheduledJobs bool
	SyncPrincipals    bool
	SyncOpenAPI       bool
	SyncDocs          bool
}

// DefaultOptions enables every category, RemoveUnlisted=false.
func DefaultOptions() Options {
	return Options{
		SyncRoles:         true,
		SyncEventTypes:    true,
		SyncDispatchPools: true,
		SyncConnections:   true,
		SyncSubscriptions: true,
		SyncProcesses:     true,
		SyncScheduledJobs: true,
		SyncPrincipals:    true,
		SyncOpenAPI:       true,
		SyncDocs:          true,
	}
}

// WithRemoveUnlisted returns DefaultOptions with RemoveUnlisted=true.
func WithRemoveUnlisted() Options {
	o := DefaultOptions()
	o.RemoveUnlisted = true
	return o
}

// NoneOptions disables every category. Use as a starting point for
// selectively enabling one or two.
func NoneOptions() Options { return Options{} }

// RolesOnly returns Options enabling only the roles category.
func RolesOnly() Options { return Options{SyncRoles: true} }

// EventTypesOnly returns Options enabling only the event types category.
func EventTypesOnly() Options { return Options{SyncEventTypes: true} }

// SubscriptionsOnly returns Options enabling only the subscriptions category.
func SubscriptionsOnly() Options { return Options{SyncSubscriptions: true} }

// ConnectionsOnly returns Options enabling only the connections category.
func ConnectionsOnly() Options { return Options{SyncConnections: true} }

// DispatchPoolsOnly returns Options enabling only the dispatch pools category.
func DispatchPoolsOnly() Options { return Options{SyncDispatchPools: true} }

// PrincipalsOnly returns Options enabling only the principals category.
func PrincipalsOnly() Options { return Options{SyncPrincipals: true} }

// ProcessesOnly returns Options enabling only the processes category.
func ProcessesOnly() Options { return Options{SyncProcesses: true} }

// ScheduledJobsOnly returns Options enabling only the scheduled jobs category.
func ScheduledJobsOnly() Options { return Options{SyncScheduledJobs: true} }

// OpenAPIOnly returns Options enabling only the OpenAPI document.
func OpenAPIOnly() Options { return Options{SyncOpenAPI: true} }

// DocsOnly returns Options enabling only the documentation pages.
func DocsOnly() Options { return Options{SyncDocs: true} }
