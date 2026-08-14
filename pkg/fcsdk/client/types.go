package client

// SyncResult is the response body returned by every per-resource sync
// endpoint.
type SyncResult struct {
	ApplicationCode string   `json:"applicationCode"`
	Created         uint32   `json:"created"`
	Updated         uint32   `json:"updated"`
	Deleted         uint32   `json:"deleted"`
	SyncedCodes     []string `json:"syncedCodes"`
}
