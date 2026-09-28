package fn

import "time"

// SubOption configures a Subscribe declaration.
type SubOption func(*subDef)

// Mode sets the subscription's delivery mode (e.g. "IMMEDIATE").
func Mode(m string) SubOption {
	return func(s *subDef) { s.Mode = m }
}

// MaxRetries sets the subscription's retry ceiling.
func MaxRetries(n int) SubOption {
	return func(s *subDef) { s.MaxRetries = &n }
}

// DataOnly marks the subscription as delivering only the event's data
// payload rather than the full envelope.
func DataOnly() SubOption {
	return func(s *subDef) { s.DataOnly = true }
}

// SubTimeout sets the subscription's per-delivery timeout.
func SubTimeout(d time.Duration) SubOption {
	return func(s *subDef) {
		sec := int(d / time.Second)
		s.TimeoutSeconds = &sec
	}
}

// Subscribe declares that this function handles eventType deliveries at
// path, which must name a webhook endpoint (plan §5.4 validation, enforced
// by the platform at publish).
func Subscribe(eventType, path string, opts ...SubOption) {
	s := subDef{EventType: eventType, Path: path}
	for _, o := range opts {
		o(&s)
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.subs = append(reg.subs, s)
}
