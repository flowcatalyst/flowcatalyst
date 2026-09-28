package fn

import (
	"context"
	"time"
)

// Info describes the in-flight invocation (plan §5.2 request meta).
type Info struct {
	ID       string
	Address  string
	Version  int
	Deadline time.Time
}

// Invocation returns the Info for the in-flight invocation. Outside a
// dispatched request it returns the zero Info.
func Invocation(ctx context.Context) Info {
	if i, ok := ctx.Value(ctxKeyInvocation).(Info); ok {
		return i
	}
	return Info{}
}
