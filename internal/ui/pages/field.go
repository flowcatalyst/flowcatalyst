package pages

import (
	"context"
	"slices"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// What the three field-identity pages (workers, devices, policy) share. The
// pages make the same checks as the admin API
// (internal/platform/fieldidentity/api): every page is client-boxed, a
// by-id resource of a client the caller cannot reach is "not found" (never
// "forbidden", so ids do not leak), and an application-confined service
// account is limited to workers and devices of its own applications.

// fieldClientParam is the URL parameter holding the chosen client's id.
const fieldClientParam = portalClientParam

// fieldApps indexes every application by id, for the grants shown.
type fieldApps map[string]application.Application

func loadFieldApps(ctx context.Context, store FieldStore) (fieldApps, error) {
	apps, err := store.Applications(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_applications failed", err)
	}
	out := make(fieldApps, len(apps))
	for _, a := range apps {
		out[string(a.ID)] = a
	}
	return out, nil
}

// ref names an application; one that is gone shows its id (the API's fallback).
func (idx fieldApps) ref(id string) (code, name string) {
	if a, ok := idx[id]; ok {
		return a.Code, a.Name
	}
	return id, id
}

// reachable are the applications the caller may work with, sorted by name.
func (idx fieldApps) reachable(ac *auth.AuthContext, activeOnly bool) []application.Application {
	var out []application.Application
	for id, a := range idx {
		if ac.CanAccessApplication(id) && (a.Active || !activeOnly) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b application.Application) int {
		if a.Name != b.Name {
			return compareStrings(a.Name, b.Name)
		}
		return compareStrings(a.Code, b.Code)
	})
	return out
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// fieldCallerApps is the application set an application-confined caller is
// limited to, or nil for an unconfined caller.
func fieldCallerApps(ac *auth.AuthContext) []string {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if ac.Applications == nil {
		return []string{}
	}
	return ac.Applications
}

// fieldGate applies one of the auth.CanXFieldY checks to the client of a
// loaded resource, as the API's loaders: a missing permission is a 403, a
// client the caller cannot reach (or a resource that is not there) a 404.
func fieldGate(gate func(*auth.AuthContext, string) error, ac *auth.AuthContext, clientID string, found bool, kind, id string) error {
	err := gate(ac, clientID)
	if err != nil {
		if ue := usecase.AsError(err); ue != nil && ue.Code == "SCOPE_FORBIDDEN" {
			return httperror.NotFound(kind, id)
		}
		return err
	}
	if !found {
		return httperror.NotFound(kind, id)
	}
	return nil
}

// appOptions are the checkboxes' choices: the applications' codes with names.
func fieldAppChoices(apps []application.Application) []kit.Option {
	out := make([]kit.Option, 0, len(apps))
	for _, a := range apps {
		out = append(out, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
	}
	return out
}

const fieldTimeFormat = "2006-01-02 15:04 UTC"

func fieldTime(t time.Time) string { return t.UTC().Format(fieldTimeFormat) }

// fieldCopyExpr copies a value (by element id) and says so in the page's
// status line. The id and label are page constants.
func fieldCopyExpr(id, label string) string {
	return "navigator.clipboard.writeText(document.getElementById('" + kit.ID(id) + "').textContent).then(() => $fieldCopied = '" + label + " copied to clipboard')"
}
