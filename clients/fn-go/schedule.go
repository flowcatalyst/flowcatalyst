package fn

import "encoding/json"

// SchedOption configures a Schedule declaration.
type SchedOption func(*schedDef)

// Timezone sets the schedule's IANA timezone (default UTC).
func Timezone(tz string) SchedOption {
	return func(s *schedDef) { s.Timezone = tz }
}

// Payload sets the fixed JSON payload delivered on each firing.
func Payload(p json.RawMessage) SchedOption {
	return func(s *schedDef) { s.Payload = p }
}

// Schedule declares a cron-triggered firing at path, which must name a
// webhook endpoint (plan §5.4).
func Schedule(cron, path string, opts ...SchedOption) {
	s := schedDef{Cron: cron, Path: path}
	for _, o := range opts {
		o(&s)
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.scheds = append(reg.scheds, s)
}
