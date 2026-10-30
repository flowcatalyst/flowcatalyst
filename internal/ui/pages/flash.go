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

	flashDjRequeued         = "dispatch-jobs-requeued"
	flashDjRequeuedByFilter = "dispatch-jobs-requeued-by-filter"
	flashDjPayloadSaved     = "dispatch-job-payload-saved"
	flashDjCancelled        = "dispatch-job-cancelled"
	flashDjIgnored          = "dispatch-jobs-ignored"
	flashDjCompleted        = "dispatch-job-completed"

	flashProcCreated  = "process-created"
	flashProcUpdated  = "process-updated"
	flashProcArchived = "process-archived"
	flashProcDeleted  = "process-deleted"

	flashCfgSaved   = "config-saved"
	flashCfgDeleted = "config-deleted"

	flashTraceOn  = "trace-enabled"
	flashTraceOff = "trace-disabled"

	flashFnCreated        = "function-created"
	flashFnUpdated        = "function-updated"
	flashFnDeleted        = "function-deleted"
	flashFnPromoted       = "function-promoted"
	flashFnAliasSaved     = "function-alias-saved"
	flashFnAliasRemoved   = "function-alias-removed"
	flashFnRetired        = "function-version-retired"
	flashFnSettingSaved   = "function-setting-saved"
	flashFnSettingRemoved = "function-setting-removed"
	flashFnRouteSaved     = "function-route-saved"
	flashFnRouteRemoved   = "function-route-removed"

	flashUsrCreated        = "user-created"
	flashUsrUpdated        = "user-updated"
	flashUsrActivated      = "user-activated"
	flashUsrDeactivated    = "user-deactivated"
	flashUsrDeleted        = "user-deleted"
	flashUsrResetSent      = "user-reset-sent"
	flashUsrPasswordReset  = "user-password-reset"
	flashUsrTwoFactorReset = "user-2fa-reset"
	flashUsrRoles          = "user-roles-updated"
	flashUsrApps           = "user-apps-updated"
	flashUsrAllApps        = "user-all-apps-granted"
	flashUsrRestricted     = "user-apps-restricted"
	flashUsrClientGranted  = "user-client-granted"
	flashUsrClientRevoked  = "user-client-revoked"
	flashUsrImported       = "users-imported"

	flashDevGranted = "developer-role-granted"
	flashDevRemoved = "developer-role-removed"
	flashDevRevoked = "developer-credential-revoked"

	flashPortalAppCreated      = "portal-app-created"
	flashPortalAppUpdated      = "portal-app-updated"
	flashPortalAppDeleted      = "portal-app-deleted"
	flashPortalAppAssigned     = "portal-app-assigned"
	flashPortalUserSuspended   = "portal-user-suspended"
	flashPortalUserReactivated = "portal-user-reactivated"
	flashPortalUserDeleted     = "portal-user-deleted"
	flashPortalAppGranted      = "portal-app-granted"
	flashPortalAccessRevoked   = "portal-access-revoked"

	flashIdpCreated = "identity-provider-created"
	flashIdpUpdated = "identity-provider-updated"
	flashIdpDeleted = "identity-provider-deleted"

	flashEdmCreated = "email-domain-created"
	flashEdmUpdated = "email-domain-updated"
	flashEdmMoved   = "email-domain-moved"
	flashEdmDeleted = "email-domain-deleted"

	flashCorsAdded   = "cors-origin-added"
	flashCorsRemoved = "cors-origin-removed"

	flashIngSetReapplied = "ingestion-set-reapplied"
	flashIngSetReleased  = "ingestion-set-released"
)

// The ingestions page's keys.
const (
	flashIngCreated      = "ingestion-created"
	flashIngVersionSaved = "ingestion-version-saved"
	flashIngVersionLive  = "ingestion-version-live"
	// A save that matched a version already saved: nothing new was written.
	flashIngDraftExists = "ingestion-draft-exists"
	flashIngUnchanged   = "ingestion-unchanged"
	flashIngPaused      = "ingestion-paused"
	flashIngResumed     = "ingestion-resumed"
	flashIngDeleted     = "ingestion-deleted"
	// The environment's settings of an ingestion (platform config ingest/<key>).
	flashIngTraceOn       = "ingestion-trace-on"
	flashIngTraceOff      = "ingestion-trace-off"
	flashIngConfigSet     = "ingestion-config-set"
	flashIngConfigDeleted = "ingestion-config-deleted"
)

// The platform settings pages' keys (theme, client login branding, names).
const (
	flashThemeSaved       = "theme-saved"
	flashThemeClientSaved = "client-theme-saved"
	flashThemeClientReset = "client-theme-reset"
	flashNameSaved        = "platform-name-saved"
)

// FlashMessages is the table the shell maps flash keys through
// (shell.Shell.Flashes). A key not listed here is never rendered.
var FlashMessages = map[string]kit.FlashMessageProps{
	// The SPA's toasts of the theme, client branding and names pages.
	flashThemeSaved:       success("Theme saved successfully"),
	flashThemeClientSaved: success("Login branding saved"),
	flashThemeClientReset: success("Login branding reset to the platform theme"),
	flashNameSaved:        {Kind: kit.FlashSuccess, Summary: "Saved", Detail: "Platform name updated."},

	// The Nuxt control plane's ingestion toasts name the code; a Datastar
	// answer gets the same from the table, a flash key after a native post
	// carries no request text.
	flashIngCreated:      success("Ingestion created"),
	flashIngVersionSaved: {Kind: kit.FlashSuccess, Summary: "Saved as a draft", Detail: "Someone with promote makes it live from the Versions tab"},
	flashIngDraftExists:  {Kind: kit.FlashInfo, Summary: "Nothing saved", Detail: "A draft already says this; someone with promote makes it live"},
	flashIngUnchanged:    {Kind: kit.FlashInfo, Summary: "Nothing changed", Detail: "The live version already says this"},
	flashIngVersionLive:  {Kind: kit.FlashSuccess, Summary: "Version live", Detail: "Runtimes run it on their next poll"},
	flashIngPaused:       success("Ingestion paused"),
	flashIngResumed:      success("Ingestion resumed"),
	flashIngDeleted:      success("Ingestion removed"),
	// Each runtime starts capturing when it next syncs.
	flashIngTraceOn:  {Kind: kit.FlashSuccess, Summary: "Tracing on", Detail: "Each runtime starts capturing when it next syncs"},
	flashIngTraceOff: {Kind: kit.FlashSuccess, Summary: "Tracing off"},

	flashIngConfigSet:     success("Settings saved for this environment"),
	flashIngConfigDeleted: {Kind: kit.FlashSuccess, Summary: "Settings deleted", Detail: "The ingestions that use them wait until they are set again"},

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

	flashDjRequeued:         {Kind: kit.FlashSuccess, Summary: "Requeued", Detail: "The dispatch jobs were reset to PENDING"},
	flashDjRequeuedByFilter: {Kind: kit.FlashSuccess, Summary: "Requeued", Detail: "The matching dispatch jobs were reset to PENDING"},
	flashDjPayloadSaved:     {Kind: kit.FlashSuccess, Summary: "Payload saved", Detail: "Requeue the job to deliver it with the new payload"},
	flashDjCancelled:        {Kind: kit.FlashSuccess, Summary: "Cancelled", Detail: "The job will not be delivered or retried"},
	flashDjIgnored:          {Kind: kit.FlashSuccess, Summary: "Ignored", Detail: "The dispatch jobs were marked CANCELLED"},
	flashDjCompleted:        {Kind: kit.FlashSuccess, Summary: "Marked completed", Detail: "The job is COMPLETED; nothing was sent"},

	// The SPA's process toasts carry the code as their detail; a flash key
	// carries no request text, so the summary stands alone.
	flashProcCreated:  {Kind: kit.FlashSuccess, Summary: "Process created"},
	flashProcUpdated:  {Kind: kit.FlashSuccess, Summary: "Process updated"},
	flashProcArchived: {Kind: kit.FlashSuccess, Summary: "Process archived"},
	flashProcDeleted:  {Kind: kit.FlashSuccess, Summary: "Process deleted"},

	// The SPA's toasts name the property; a flash key carries no request text.
	flashCfgSaved:   {Kind: kit.FlashSuccess, Summary: "Saved", Detail: "The property was saved"},
	flashCfgDeleted: {Kind: kit.FlashSuccess, Summary: "Deleted", Detail: "The property was deleted"},

	// The SPA's trace toasts name the context; a Datastar answer does too, a
	// flash key after a native post carries no request text.
	flashTraceOn:  {Kind: kit.FlashSuccess, Summary: "Tracing on", Detail: "The context is on"},
	flashTraceOff: {Kind: kit.FlashSuccess, Summary: "Tracing off", Detail: "The context is off"},

	// The SPA's function toasts. A Datastar answer replaces the detail with
	// text the server built (the version promoted, the wiring changed, the key
	// set); a flash key after a native post carries none.
	flashFnCreated:        success("Function created"),
	flashFnUpdated:        success("Function updated"),
	flashFnDeleted:        success("Function deleted"),
	flashFnPromoted:       {Kind: kit.FlashSuccess, Summary: "Promoted", Detail: "The version is now live"},
	flashFnAliasSaved:     {Kind: kit.FlashSuccess, Summary: "Alias updated", Detail: "The alias was updated"},
	flashFnAliasRemoved:   success("Alias removed"),
	flashFnRetired:        success("Version retired"),
	flashFnSettingSaved:   success("Setting saved"),
	flashFnSettingRemoved: success("Setting removed"),
	flashFnRouteSaved:     success("Route saved"),
	flashFnRouteRemoved:   success("Route removed"),

	// The SPA's user toasts. A Datastar answer replaces the detail with text
	// the server built (the roles added, the user's name); a flash key after a
	// native post carries none.
	flashUsrCreated:        {Kind: kit.FlashSuccess, Summary: "User created", Detail: "The user was added"},
	flashUsrUpdated:        success("User updated successfully"),
	flashUsrActivated:      success("User activated"),
	flashUsrDeactivated:    success("User deactivated"),
	flashUsrDeleted:        success("User deleted"),
	flashUsrResetSent:      {Kind: kit.FlashSuccess, Summary: "Reset email sent", Detail: "Password reset email sent"},
	flashUsrPasswordReset:  {Kind: kit.FlashSuccess, Summary: "Password reset", Detail: "Password reset successfully"},
	flashUsrTwoFactorReset: {Kind: kit.FlashSuccess, Summary: "2FA reset", Detail: "Two-factor authentication reset"},
	flashUsrRoles:          success("Roles updated"),
	flashUsrApps:           success("Application access updated"),
	flashUsrAllApps:        success("Granted access to all applications"),
	flashUsrRestricted:     success("Restricted to specific applications"),
	flashUsrClientGranted:  success("Client access granted"),
	flashUsrClientRevoked:  success("Client access revoked"),
	flashUsrImported:       {Kind: kit.FlashSuccess, Summary: "Import complete", Detail: "The import finished"},

	// The SPA's developer user toasts name the user; a Datastar answer does
	// too, a flash key after a native post carries no request text.
	flashDevGranted: {Kind: kit.FlashSuccess, Summary: "Granted", Detail: "Developer role granted"},
	flashDevRemoved: {Kind: kit.FlashSuccess, Summary: "Removed", Detail: "Developer role removed"},
	flashDevRevoked: {Kind: kit.FlashSuccess, Summary: "Revoked", Detail: "Developer credential revoked"},

	// The SPA's portal toasts name the app or the user; a Datastar answer does
	// too, a flash key after a native post carries no request text.
	flashPortalAppCreated:      success("Portal app created"),
	flashPortalAppUpdated:      success("Portal app updated"),
	flashPortalAppDeleted:      success("Portal app deleted"),
	flashPortalAppAssigned:     success("Portal users assigned"),
	flashPortalUserSuspended:   success("Portal user suspended"),
	flashPortalUserReactivated: success("Portal user reactivated"),
	flashPortalUserDeleted:     success("Portal user deleted"),
	flashPortalAppGranted:      success("Portal app access granted"),
	flashPortalAccessRevoked:   success("Portal app access removed"),

	// The SPA's identity provider and email domain toasts name the record; a
	// Datastar answer does too, a flash key after a native post carries no
	// request text.
	flashIdpCreated: success("Identity provider created successfully"),
	flashIdpUpdated: success("Identity provider updated successfully"),
	flashIdpDeleted: success("Identity provider deleted"),
	flashEdmCreated: success("Email domain mapping created successfully"),
	flashEdmUpdated: success("Email domain mapping updated successfully"),
	flashEdmMoved:   {Kind: kit.FlashSuccess, Summary: "Provider changed", Detail: "The domain now signs in through the new provider"},
	flashEdmDeleted: success("Email domain mapping deleted"),

	// The SPA's CORS toasts name the origin; a Datastar answer does too, a
	// flash key after a native post carries no request text.
	flashCorsAdded:   success("CORS origin added successfully"),
	flashCorsRemoved: success("CORS origin removed"),

	// The Datastar answers of the sets name the set and the apply; a flash key
	// after a native post carries no request text.
	flashIngSetReapplied: {Kind: kit.FlashSuccess, Summary: "Success", Detail: "The set was applied again as the earlier apply left it"},
	flashIngSetReleased:  {Kind: kit.FlashSuccess, Summary: "Success", Detail: "The set was released; its ingestions can be edited one at a time again"},
}

// success is the SPA's toast.success("Success", detail).
func success(detail string) kit.FlashMessageProps {
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Success", Detail: detail}
}
