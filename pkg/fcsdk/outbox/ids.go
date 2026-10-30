package outbox

import "github.com/flowcatalyst/go-sdk/tsid"

// newRowID returns a 13-character Crockford Base32 TSID, the id every SDK
// generates for rows that have none of their own.
func newRowID() string { return tsid.GenerateUntyped() }
