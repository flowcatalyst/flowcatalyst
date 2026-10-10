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
}

// success is the SPA's toast.success("Success", detail).
func success(detail string) kit.FlashMessageProps {
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Success", Detail: detail}
}
