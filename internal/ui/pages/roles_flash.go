package pages

import "github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"

// Flash keys of the roles and permissions pages (shell.SetFlash); the texts
// are the SPA's toasts. The sync and permission ones carry text built on the
// server in a Datastar answer (the tally, the code).
const (
	flashRoleCreated  = "role-created"
	flashRoleUpdated  = "role-updated"
	flashRoleDeleted  = "role-deleted"
	flashRolesSynced  = "roles-synced"
	flashRoleNotAdmin = "role-not-admin-created"
	flashRoleNoUpdate = "role-no-update"
	flashPermCreated  = "permission-created"
)

func init() {
	FlashMessages[flashRoleCreated] = success("Role created successfully")
	FlashMessages[flashRoleUpdated] = kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Saved", Detail: "Role updated successfully"}
	FlashMessages[flashRoleDeleted] = success("Role deleted successfully")
	FlashMessages[flashRolesSynced] = kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform Roles Synced", Detail: "The platform's roles are up to date."}
	FlashMessages[flashRoleNotAdmin] = kit.FlashMessageProps{Kind: kit.FlashWarn, Summary: "Not Editable", Detail: "Only admin-created roles can be edited"}
	FlashMessages[flashRoleNoUpdate] = kit.FlashMessageProps{Kind: kit.FlashWarn, Summary: "Not Editable", Detail: "You do not have permission to edit roles"}
	FlashMessages[flashPermCreated] = kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Created", Detail: "The permission was created"}
}
