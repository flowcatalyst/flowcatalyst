package middleware

import (
	"context"
	"testing"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestRequireSameOriginForSessionRead(t *testing.T) {
	cookie := WithSessionCookieAuth(context.Background())
	for _, site := range []string{"", "cross-site", "same-site", "none"} {
		ue := usecase.AsError(RequireSameOriginForSessionRead(cookie, site))
		if ue == nil || ue.Code != CodeCrossOriginRequest || ue.Kind != usecase.KindAuthorization {
			t.Errorf("cookie, Sec-Fetch-Site %q: got %v, want a CROSS_ORIGIN_REQUEST refusal", site, ue)
		}
	}
	if err := RequireSameOriginForSessionRead(cookie, "same-origin"); err != nil {
		t.Errorf("cookie, same-origin: %v", err)
	}
	for name, ctx := range map[string]context.Context{
		"bearer": WithBearerAuth(context.Background()),
		"plain":  context.Background(),
	} {
		if err := RequireSameOriginForSessionRead(ctx, "cross-site"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
