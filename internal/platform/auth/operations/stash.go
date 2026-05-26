package operations

import "sync"

// secretStash is a process-local map for the create/rotate handlers to
// recover the plaintext after the UoW commit. See note in oauth_client.go.
var secretStash sync.Map
