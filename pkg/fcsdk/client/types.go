package client

import "net/url"

// SyncResult is the response body returned by every per-resource sync
// endpoint.
type SyncResult struct {
	ApplicationCode string   `json:"applicationCode"`
	Created         uint32   `json:"created"`
	Updated         uint32   `json:"updated"`
	Deleted         uint32   `json:"deleted"`
	SyncedCodes     []string `json:"syncedCodes"`
	// PasswordHashIgnored (principal syncs only): the emails, lower-cased,
	// whose passwordHash was ignored because the user already exists (the
	// platform stores a hash only on a user the sync creates).
	PasswordHashIgnored []string `json:"passwordHashIgnored,omitempty"`
	// SkippedRoles (application principal sync only): role names that are
	// not roles and so were assigned to no one (lower-cased, each once, in
	// payload order). The rest of the sync applied.
	SkippedRoles []string `json:"skippedRoles,omitempty"`
}

// syncPath is POST /api/applications/{appCode}/<resource>/sync, with
// ?removeUnlisted=true when asked.
func syncPath(appCode, resource string, removeUnlisted bool) string {
	p := "/api/applications/" + url.PathEscape(appCode) + "/" + resource + "/sync"
	if removeUnlisted {
		p += "?removeUnlisted=true"
	}
	return p
}
