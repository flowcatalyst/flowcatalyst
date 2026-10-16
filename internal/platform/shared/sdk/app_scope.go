package sdk

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// AppScope confines an application-scoped caller (an application service
// account: !AllApplications) to the applications it is bound to on the
// SDK ingest endpoints. Callers holding all-applications access are never
// restricted. It memoises code → application id for the life of one request,
// so a 1000-item batch costs one SELECT per DISTINCT application code.
//
// Not safe for concurrent use; build one per request.
type AppScope struct {
	apps  *application.Repository
	cache map[string]*string // nil value = looked up, no such application
}

// NewAppScope builds a per-request resolver over the application repository.
func NewAppScope(apps *application.Repository) *AppScope {
	return &AppScope{apps: apps, cache: map[string]*string{}}
}

// FirstSegment returns the application code of a `{application}:...` code
// (an event type, a dispatch-job code): everything before the first colon, or
// the whole string when there is none. Matches the split_part(code, ':', 1)
// the platform's own application facet uses.
func FirstSegment(code string) string {
	seg, _, _ := strings.Cut(strings.TrimSpace(code), ":")
	return seg
}

// AppIDByCode resolves an application code to its id (nil when unknown).
func (s *AppScope) AppIDByCode(ctx context.Context, code string) (*string, error) {
	if id, ok := s.cache[code]; ok {
		return id, nil
	}
	var id *string
	if s.apps != nil {
		app, err := s.apps.FindByCode(ctx, code)
		if err != nil {
			return nil, usecase.Internal("REPO", "application find_by_code failed", err)
		}
		if app != nil {
			v := string(app.ID)
			id = &v
		}
	}
	s.cache[code] = id
	return id, nil
}

// CanAccessCode reports whether ac may act for the application with the
// given code. Non-scoped callers always may (no lookup). A scoped caller may
// only when the code names an existing application it can access; an unknown
// or empty code is refused.
func (s *AppScope) CanAccessCode(ctx context.Context, ac *auth.AuthContext, code string) (bool, error) {
	if !ac.IsApplicationScoped() {
		return true, nil
	}
	if code == "" {
		return false, nil
	}
	id, err := s.AppIDByCode(ctx, code)
	if err != nil {
		return false, err
	}
	return id != nil && ac.CanAccessApplication(*id), nil
}

// ReadScopeRefused is the message of the refusal an application-scoped read
// gives a caller bound to no (resolvable) application.
const ReadScopeRefused = "this credential is not bound to an application"

// ConfinedCodes answers which application codes a read is confined to for ac.
// confined is false for a caller that reaches every application (all
// applications), whose reads are not narrowed. For an application-scoped
// caller it returns the codes of the applications it is bound to. A caller
// bound to none (or to ids that no longer resolve) is refused with 403
// APPLICATION_ACCESS_REQUIRED rather than answered an empty page, so a
// mis-provisioned credential is loud, never silently blind (F20).
func (s *AppScope) ConfinedCodes(ctx context.Context, ac *auth.AuthContext) (codes []string, confined bool, err error) {
	if !ac.IsApplicationScoped() {
		return nil, false, nil
	}
	if s.apps != nil {
		for _, id := range ac.Applications {
			app, ferr := s.apps.FindByID(ctx, id)
			if ferr != nil {
				return nil, true, usecase.Internal("REPO", "application find_by_id failed", ferr)
			}
			if app != nil && app.Code != "" {
				codes = append(codes, app.Code)
			}
		}
	}
	if len(codes) == 0 {
		return nil, true, httperror.Forbidden(ReadScopeRefused)
	}
	return codes, true, nil
}
