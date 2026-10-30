package dispatchjob

import (
	"fmt"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// The time window of a dispatch-job list read.
//
// msg_dispatch_jobs_read is partitioned by created_at, so a created_at range is
// what lets a read touch a few partitions instead of all of them. Every
// list-shaped read (the list, list-raw, filter-options, the requeue-by-filter
// preview and run) therefore carries a window: the caller's since/until, or,
// when the caller names neither, the last DefaultWindow. A window wider than
// the configured maximum is refused. Lookups keyed by one job or one event
// (by id, by event, attempts, sign) are exempt: their key already bounds them.
const (
	// DefaultWindow is applied to a read that names no time bound.
	DefaultWindow = 24 * time.Hour
	// DefaultMaxWindowDays is the widest window a read may ask for unless
	// FC_DISPATCH_JOB_MAX_WINDOW_DAYS says otherwise.
	DefaultMaxWindowDays = 31

	// WindowTooLargeCode is the 400 code for a window beyond the maximum.
	WindowTooLargeCode = "WINDOW_TOO_LARGE"
)

// Window is the created_at range a read was held to, echoed to the caller.
type Window struct {
	From time.Time
	To   time.Time
	// Defaulted is true when the server supplied a bound the caller did not.
	Defaulted bool
}

// MaxWindow is the widest window for maxDays (non-positive: the default).
func MaxWindow(maxDays int) time.Duration {
	if maxDays < 1 {
		maxDays = DefaultMaxWindowDays
	}
	return time.Duration(maxDays) * 24 * time.Hour
}

// EnforceWindow holds p to a created_at window and reports it.
//
//   - No since, no until: the last DefaultWindow (since = now - 24h; no upper
//     bound in SQL, the reported "to" is now).
//   - since only: since .. now. until only: until-24h .. until (Defaulted is
//     true: the server supplied the missing bound).
//   - Both: as given; until before since is a 400 INVALID_WINDOW.
//
// A window wider than maxDays days (non-positive: DefaultMaxWindowDays) is a
// 400 WINDOW_TOO_LARGE. The returned params always carry Since; Until is the
// caller's (nil when they gave none: the reported "to" is then now).
func EnforceWindow(p FilterParams, now time.Time, maxDays int) (FilterParams, Window, error) {
	w := Window{To: now}
	switch {
	case p.Since == nil && p.Until == nil:
		w.From, w.Defaulted = now.Add(-DefaultWindow), true
	case p.Since != nil && p.Until == nil:
		w.From = *p.Since
	case p.Since == nil && p.Until != nil:
		w.To = *p.Until
		w.From, w.Defaulted = p.Until.Add(-DefaultWindow), true
	default:
		w.From, w.To = *p.Since, *p.Until
		if w.To.Before(w.From) {
			return p, Window{}, usecase.Validation("INVALID_WINDOW", "until must not be before since")
		}
	}
	w.From, w.To = w.From.UTC(), w.To.UTC()
	if max := MaxWindow(maxDays); w.To.Sub(w.From) > max {
		return p, Window{}, usecase.Validation(WindowTooLargeCode,
			fmt.Sprintf("the time window is %s; at most %d days may be read at once (narrow since/until)",
				w.To.Sub(w.From).Round(time.Minute), int(max/(24*time.Hour))))
	}
	from := w.From
	p.Since = &from // Until stays as the caller gave it (nil: open at the top)
	return p, w, nil
}
