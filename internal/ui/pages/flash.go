package pages

import "github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"

// Flash keys a native post/redirect/get may carry (shell.SetFlash). The
// texts are the SPA's toasts (summary "Success", the action as detail).
const (
	flashPoolCreated   = "pool-created"
	flashPoolUpdated   = "pool-updated"
	flashPoolSuspended = "pool-suspended"
	flashPoolActivated = "pool-activated"
	flashPoolDeleted   = "pool-deleted"

	flashConnCreated   = "connection-created"
	flashConnUpdated   = "connection-updated"
	flashConnPaused    = "connection-paused"
	flashConnActivated = "connection-activated"
	flashConnDeleted   = "connection-deleted"

	flashETCreated        = "event-type-created"
	flashETUpdated        = "event-type-updated"
	flashETArchived       = "event-type-archived"
	flashETDeleted        = "event-type-deleted"
	flashETSynced         = "event-types-synced"
	flashSchemaAdded      = "schema-added"
	flashSchemaFinalised  = "schema-finalised"
	flashSchemaDeprecated = "schema-deprecated"
	flashExportEmpty      = "schema-export-empty"

	flashSubCreated = "subscription-created"
	flashSubUpdated = "subscription-updated"
	flashSubPaused  = "subscription-paused"
	flashSubResumed = "subscription-resumed"
	flashSubDeleted = "subscription-deleted"

	flashAppCreated     = "application-created"
	flashAppUpdated     = "application-updated"
	flashAppActivated   = "application-activated"
	flashAppDeactivated = "application-deactivated"
	flashAppDeleted     = "application-deleted"

	flashCliCreated     = "client-created"
	flashCliUpdated     = "client-updated"
	flashCliActivated   = "client-activated"
	flashCliSuspended   = "client-suspended"
	flashCliDeactivated = "client-deactivated"
	flashCliApps        = "client-applications-updated"

	flashSjCreated  = "scheduled-job-created"
	flashSjUpdated  = "scheduled-job-updated"
	flashSjPaused   = "scheduled-job-paused"
	flashSjResumed  = "scheduled-job-resumed"
	flashSjArchived = "scheduled-job-archived"
	flashSjDeleted  = "scheduled-job-deleted"
	flashSjFired    = "scheduled-job-fired"
)

// FlashMessages is the table the shell maps flash keys through
// (shell.Shell.Flashes). A key not listed here is never rendered.
var FlashMessages = map[string]kit.FlashMessageProps{
	flashPoolCreated:   success("Dispatch pool created"),
	flashPoolUpdated:   success("Pool updated"),
	flashPoolSuspended: success("Pool suspended"),
	flashPoolActivated: success("Pool activated"),
	flashPoolDeleted:   success("Pool deleted"),

	flashConnCreated:   success("Connection created"),
	flashConnUpdated:   success("Connection updated"),
	flashConnPaused:    success("Connection paused"),
	flashConnActivated: success("Connection activated"),
	flashConnDeleted:   success("Connection deleted"),

	flashETCreated:        success("Event type created"),
	flashETUpdated:        success("Event type updated"),
	flashETArchived:       success("Event type archived"),
	flashETDeleted:        success("Event type deleted"),
	flashETSynced:         {Kind: kit.FlashSuccess, Summary: "Platform Events Synced", Detail: "The platform's event types are up to date."},
	flashSchemaAdded:      success("Schema added"),
	flashSchemaFinalised:  success("Schema finalised"),
	flashSchemaDeprecated: success("Schema deprecated"),
	flashExportEmpty:      {Kind: kit.FlashWarn, Summary: "Nothing to Export", Detail: "No event types with a CURRENT schema in the current view"},

	flashSubCreated: success("Subscription created"),
	flashSubUpdated: success("Subscription updated"),
	flashSubPaused:  success("Subscription paused"),
	flashSubResumed: success("Subscription resumed"),
	flashSubDeleted: success("Subscription deleted"),

	flashAppCreated:     success("Application created"),
	flashAppUpdated:     success("Application updated"),
	flashAppActivated:   success("Application activated"),
	flashAppDeactivated: success("Application deactivated"),
	flashAppDeleted:     success("Application deleted"),

	flashCliCreated:     success("Client created"),
	flashCliUpdated:     success("Client updated"),
	flashCliActivated:   success("Client activated"),
	flashCliSuspended:   success("Client suspended"),
	flashCliDeactivated: success("Client deactivated"),
	flashCliApps:        success("Applications updated"),

	// The SPA's scheduled-job toasts carry their own summaries.
	flashSjCreated:  {Kind: kit.FlashSuccess, Summary: "Created", Detail: "Scheduled job created"},
	flashSjUpdated:  success("Scheduled job updated"),
	flashSjPaused:   {Kind: kit.FlashSuccess, Summary: "Paused", Detail: "Scheduled job paused"},
	flashSjResumed:  {Kind: kit.FlashSuccess, Summary: "Resumed", Detail: "Scheduled job resumed"},
	flashSjArchived: {Kind: kit.FlashSuccess, Summary: "Archived", Detail: "Scheduled job archived"},
	flashSjDeleted:  {Kind: kit.FlashSuccess, Summary: "Deleted", Detail: "Scheduled job deleted"},
	flashSjFired:    {Kind: kit.FlashSuccess, Summary: "Fired", Detail: "An instance was created"},
}

// success is the SPA's toast.success("Success", detail).
func success(detail string) kit.FlashMessageProps {
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Success", Detail: detail}
}
