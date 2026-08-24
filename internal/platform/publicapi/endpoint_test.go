package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// fakeConfigs returns canned theme rows keyed by scope. A missing key
// mimics a config miss; err forces every lookup to fail.
type fakeConfigs struct {
	global string
	client map[string]string
	err    error
}

func (f fakeConfigs) FindByCoordinate(_ context.Context, _, _, _ string, scope platformconfig.Scope, clientID *string) (*platformconfig.Config, error) {
	if f.err != nil {
		return nil, f.err
	}
	if scope == platformconfig.ScopeClient {
		if clientID == nil {
			return nil, nil
		}
		v, ok := f.client[*clientID]
		if !ok {
			return nil, nil
		}
		return &platformconfig.Config{Value: v}, nil
	}
	if f.global == "" {
		return nil, nil
	}
	return &platformconfig.Config{Value: f.global}, nil
}

// resolver maps identifiers to client IDs, mimicking the client repository.
func resolver(byIdentifier map[string]string) ClientIDResolver {
	return func(_ context.Context, identifier string) (string, error) {
		return byIdentifier[identifier], nil
	}
}

const globalTheme = `{"brandName":"FlowCatalyst","brandSubtitle":"Platform Administration","primaryColor":"#102a43","accentColor":"#0967d2","logoUrl":"https://cdn/platform.png"}`

func getTheme(t *testing.T, e *Endpoint, query string) loginThemeResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	e.handleLoginTheme(rec, httptest.NewRequest(http.MethodGet, "/api/public/login-theme"+query, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — branding must never fail the login page", rec.Code)
	}
	var out loginThemeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not valid JSON: %v (body %q)", err, rec.Body.String())
	}
	return out
}

func str(v *string) string {
	if v == nil {
		return "<nil>"
	}
	return *v
}

func TestLoginTheme_GlobalWhenNoClientRequested(t *testing.T) {
	e := New(fakeConfigs{global: globalTheme})

	got := getTheme(t, e, "")

	if str(got.BrandName) != "FlowCatalyst" {
		t.Errorf("BrandName = %s, want FlowCatalyst", str(got.BrandName))
	}
	if str(got.AccentColor) != "#0967d2" {
		t.Errorf("AccentColor = %s, want the platform accent", str(got.AccentColor))
	}
}

// The headline behaviour: a client overrides only the fields it sets, and
// everything else still shows through from the platform theme.
func TestLoginTheme_ClientOverridesLayerOverGlobal(t *testing.T) {
	e := New(fakeConfigs{
		global: globalTheme,
		client: map[string]string{
			"cli_1": `{"brandName":"Acme","accentColor":"#ff6600"}`,
		},
	}).WithClientResolver(resolver(map[string]string{"acme": "cli_1"}))

	got := getTheme(t, e, "?client=acme")

	if str(got.BrandName) != "Acme" {
		t.Errorf("BrandName = %s, want the client override Acme", str(got.BrandName))
	}
	if str(got.AccentColor) != "#ff6600" {
		t.Errorf("AccentColor = %s, want the client override #ff6600", str(got.AccentColor))
	}
	// Not overridden by the client — must fall through to the platform theme.
	if str(got.BrandSubtitle) != "Platform Administration" {
		t.Errorf("BrandSubtitle = %s, want the inherited platform value", str(got.BrandSubtitle))
	}
	if str(got.PrimaryColor) != "#102a43" {
		t.Errorf("PrimaryColor = %s, want the inherited platform value", str(got.PrimaryColor))
	}
}

// An explicit null is how the admin clears an inherited field, and must be
// distinguishable from simply not overriding it.
func TestLoginTheme_ExplicitNullClearsInheritedField(t *testing.T) {
	e := New(fakeConfigs{
		global: globalTheme,
		client: map[string]string{"cli_1": `{"logoUrl":null}`},
	}).WithClientResolver(resolver(map[string]string{"acme": "cli_1"}))

	got := getTheme(t, e, "?client=acme")

	if got.LogoURL != nil {
		t.Errorf("LogoURL = %s, want it cleared by the explicit null", str(got.LogoURL))
	}
	if str(got.BrandName) != "FlowCatalyst" {
		t.Errorf("BrandName = %s, want the untouched platform value", str(got.BrandName))
	}
}

func TestLoginTheme_ClientIDParamSkipsResolution(t *testing.T) {
	// The admin preview names the client by ID directly; no resolver needed.
	e := New(fakeConfigs{
		global: globalTheme,
		client: map[string]string{"cli_1": `{"brandName":"Acme"}`},
	})

	got := getTheme(t, e, "?clientId=cli_1")

	if str(got.BrandName) != "Acme" {
		t.Errorf("BrandName = %s, want Acme", str(got.BrandName))
	}
}

func TestLoginTheme_UnknownClientFallsBackToGlobal(t *testing.T) {
	e := New(fakeConfigs{global: globalTheme}).
		WithClientResolver(resolver(map[string]string{}))

	got := getTheme(t, e, "?client=does-not-exist")

	if str(got.BrandName) != "FlowCatalyst" {
		t.Errorf("BrandName = %s, want the platform theme", str(got.BrandName))
	}
}

func TestLoginTheme_IgnoresClientParamWithoutResolver(t *testing.T) {
	e := New(fakeConfigs{
		global: globalTheme,
		client: map[string]string{"cli_1": `{"brandName":"Acme"}`},
	})

	got := getTheme(t, e, "?client=acme")

	if str(got.BrandName) != "FlowCatalyst" {
		t.Errorf("BrandName = %s, want the platform theme when no resolver is wired", str(got.BrandName))
	}
}

func TestLoginTheme_MalformedClientBlobKeepsGlobal(t *testing.T) {
	e := New(fakeConfigs{
		global: globalTheme,
		client: map[string]string{"cli_1": `{"brandName":`},
	}).WithClientResolver(resolver(map[string]string{"acme": "cli_1"}))

	got := getTheme(t, e, "?client=acme")

	if str(got.BrandName) != "FlowCatalyst" {
		t.Errorf("BrandName = %s, want the platform theme intact", str(got.BrandName))
	}
	if str(got.PrimaryColor) != "#102a43" {
		t.Errorf("PrimaryColor = %s, want the platform theme intact", str(got.PrimaryColor))
	}
}

func TestLoginTheme_LookupFailureStillServes200(t *testing.T) {
	e := New(fakeConfigs{err: errors.New("db down")})

	got := getTheme(t, e, "?clientId=cli_1")

	if got.BrandName != nil {
		t.Errorf("BrandName = %s, want an empty theme so the SPA uses its own defaults", str(got.BrandName))
	}
}

func TestLoginTheme_NilConfigsServesEmptyTheme(t *testing.T) {
	got := getTheme(t, New(nil), "")

	if got.BrandName != nil || got.PrimaryColor != nil {
		t.Errorf("want an empty theme, got brand=%s primary=%s", str(got.BrandName), str(got.PrimaryColor))
	}
}
