package eventtype

import (
	"context"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ApplicationLookup is the one application read the visibility rules need:
// an event type names its application by code, a principal is bound to
// applications by id.
type ApplicationLookup interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// VisibleTo keeps the event types the caller may read, by GET
// /api/event-types's rules: client-scoped event types need access to their
// client; a caller without the messaging view permission (an application
// service account admitted by the application-service view) sees only the
// event types of the applications it is bound to. The read permission itself
// (auth.CanReadEventTypes) is the caller's to check first.
func VisibleTo(ctx context.Context, ac *auth.AuthContext, apps ApplicationLookup, rows []EventType) ([]EventType, error) {
	vis := auth.FilterClientScoped(ac, rows, func(et *EventType) *string { return ids.StringPtr(et.ClientID) })
	if auth.CanReadAllEventTypes(ac) {
		return vis, nil
	}
	seen := map[string]bool{}
	kept := vis[:0:0]
	for _, et := range vis {
		ok, err := appAccess(ctx, ac, apps, et.Application, seen)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, et)
		}
	}
	return kept, nil
}

// CheckReadable refuses an event type the caller may not read (GET
// /api/event-types/{id}'s rules, after the read permission).
func CheckReadable(ctx context.Context, ac *auth.AuthContext, apps ApplicationLookup, et *EventType) error {
	if et.ClientID != nil && !ac.CanAccessClient(string(*et.ClientID)) {
		return httperror.Forbidden("No access to this event type")
	}
	if auth.CanReadAllEventTypes(ac) {
		return nil
	}
	ok, err := appAccess(ctx, ac, apps, et.Application, map[string]bool{})
	if err != nil {
		return err
	}
	if !ok {
		return httperror.Forbidden("No access to this event type")
	}
	return nil
}

// MatchAny keeps the event types whose application, subdomain and aggregate
// are each one of the chosen values; an empty choice does not filter. The
// SPA's facets are multi-selects sent as repeated query parameters.
func MatchAny(rows []EventType, applications, subdomains, aggregates []string) []EventType {
	in := func(set []string, v string) bool { return len(set) == 0 || slices.Contains(set, v) }
	out := rows[:0:0]
	for _, et := range rows {
		if in(applications, et.Application) && in(subdomains, et.Subdomain) && in(aggregates, et.Aggregate) {
			out = append(out, et)
		}
	}
	return out
}

func appAccess(ctx context.Context, ac *auth.AuthContext, apps ApplicationLookup, code string, seen map[string]bool) (bool, error) {
	if ok, done := seen[code]; done {
		return ok, nil
	}
	if apps == nil {
		return false, usecase.Internal("WIRING", "application lookup not configured", nil)
	}
	app, err := apps.FindByCode(ctx, code)
	if err != nil {
		return false, usecase.Internal("REPO", "find application by code failed", err)
	}
	ok := app != nil && ac.CanAccessApplication(string(app.ID))
	seen[code] = ok
	return ok, nil
}
