package scheduledjob

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser: seconds are REQUIRED, so the one accepted (and firing) form is
// the 6-field "sec min hour dom mon dow". robfig/cron does not model an
// optional year field, so 7-field expressions do not parse; a bare 5-field
// POSIX expression fails too (seconds required).
var cronParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ValidateCron accepts exactly what fires: a 6-field, seconds-first cron
// expression that the firing parser (cronParser) can parse. Used on
// create/update/sync and function publish, so a stored expression always
// produces fire slots — a 5- or 7-field expression is rejected up front
// instead of being saved and then silently never firing. The wire contract and
// every SDK document 6-field only; nothing is auto-converted at the API.
func ValidateCron(expr string) error {
	trimmed := strings.TrimSpace(expr)
	if n := len(strings.Fields(trimmed)); n != 6 {
		return fmt.Errorf("cron expression must have exactly 6 whitespace-separated fields "+
			"(seconds first: sec min hour dom mon dow), got %d: '%s'", n, trimmed)
	}
	if _, err := cronParser.Parse(trimmed); err != nil {
		return fmt.Errorf("invalid cron expression '%s': %w", trimmed, err)
	}
	return nil
}

// ValidateCronFires is retained for function publish; same rule as ValidateCron.
func ValidateCronFires(expr string) error { return ValidateCron(expr) }

// NextSlotAfter returns the earliest slot strictly after `after` across all of
// a job's cron expressions, evaluated in tzName (unparseable expressions are
// skipped). ok=false when none exists. The poller sleeps until this instant.
func NextSlotAfter(crons []string, tzName string, after time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}
	var best time.Time
	found := false
	for _, expr := range crons {
		sched, err := cronParser.Parse(expr)
		if err != nil {
			continue
		}
		if slot := sched.Next(after.In(loc)); !slot.IsZero() && (!found || slot.Before(best)) {
			best, found = slot, true
		}
	}
	if !found {
		return time.Time{}, false
	}
	return best.UTC(), true
}

// LatestSlotInWindow returns the latest cron slot in the half-open window
// (after, upTo] across all of a job's cron expressions, evaluated in tzName.
//
// "Skip-missed" (AWS-style)
// semantics: when a long downtime means several slots elapsed, only the
// LATEST fires; older missed slots are dropped. Returns ok=false when no slot
// falls in the window (including after >= upTo). Unparseable expressions are
// skipped (a legacy 5/7-field expression that predates ValidateCron yields no slot).
func LatestSlotInWindow(crons []string, tzName string, after, upTo time.Time) (time.Time, bool) {
	if !after.Before(upTo) {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}
	afterLoc := after.In(loc)
	upToLoc := upTo.In(loc)

	var best time.Time
	found := false
	for _, expr := range crons {
		sched, err := cronParser.Parse(expr)
		if err != nil {
			continue
		}
		// Walk forward from `after` (exclusive) collecting slots <= upTo,
		// keeping the latest. Next() is strictly increasing, so this
		// terminates at the window edge.
		for slot := sched.Next(afterLoc); !slot.IsZero() && !slot.After(upToLoc); slot = sched.Next(slot) {
			if !found || slot.After(best) {
				best = slot
				found = true
			}
		}
	}
	if !found {
		return time.Time{}, false
	}
	return best.UTC(), true
}
