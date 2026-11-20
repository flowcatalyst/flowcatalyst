package payloadstore

import (
	"context"
	"fmt"
)

// FromJoin builds the Stored a reader selected beside its own row: the row's
// payload_id and the msg_payloads columns it joined by primary key (data,
// ref, size; all NULL when the join found no row). Nil when payloadID is nil:
// a row of the legacy shape, whose payload is in its own column
// (docs/payload-offload-design.md, section 10, step 1).
//
// A payload_id whose row was not found comes back with neither Data nor Ref,
// which Resolve answers ErrMissing.
func FromJoin(payloadID, data, ref *string, size *int32) *Stored {
	if payloadID == nil {
		return nil
	}
	s := &Stored{ID: *payloadID, Data: data, Ref: ref}
	if size != nil {
		s.Size = int(*size)
	}
	return s
}

// Offloaded reports whether the payload is a reference into the payload
// store, not bytes the row already holds.
func (p Stored) Offloaded() bool { return p.Ref != nil }

// ResolveWith resolves p through s, as Service.Resolve does, for a reader
// whose service may be nil (a repository built without one, in a test or a
// tool). Without a service a row's data is still returned; a reference is
// ErrStoreNotConfigured and a row with neither data nor ref is ErrMissing.
func ResolveWith(ctx context.Context, s *Service, p Stored) ([]byte, error) {
	if s != nil {
		return s.Resolve(ctx, p)
	}
	switch {
	case p.Data != nil:
		return []byte(*p.Data), nil
	case p.Ref != nil:
		return nil, fmt.Errorf("%w: payload %s is offloaded and this reader has no payload service", ErrStoreNotConfigured, p.ID)
	default:
		return nil, fmt.Errorf("%w: payload %s: no payload row under its created_at", ErrMissing, p.ID)
	}
}
