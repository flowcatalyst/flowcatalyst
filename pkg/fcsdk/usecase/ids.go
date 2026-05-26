package usecase

import "github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/tsid"

// newEventID returns a fresh 13-character Crockford Base32 TSID — the
// format msg_events.id expects (VARCHAR(13) per the schema). Internal
// helper used by NewEventMetadata.
func newEventID() string { return tsid.GenerateUntyped() }
