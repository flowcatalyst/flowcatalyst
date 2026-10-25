package pages

import (
	"net/http"
	"net/url"
	"testing"
)

// Answers that carry a secret are no-store. The Datastar stream helper sets
// no-cache when it starts a stream, which a cache may still keep.
func TestSecretBearingAnswersAreNoStore(t *testing.T) {
	check := func(t *testing.T, name string, got string) {
		t.Helper()
		if got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", name, got)
		}
	}
	t.Run("an application's provisioned credentials", func(t *testing.T) {
		h := newAppHarness()
		for _, datastar := range []bool{true, false} {
			rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", datastar: datastar, form: url.Values{}})
			check(t, "provision-service-account", rec.Header().Get("Cache-Control"))
		}
	})
	t.Run("a dispatch job's dry-run signature", func(t *testing.T) {
		h := newDJHarness()
		for _, datastar := range []bool{true, false} {
			rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/sign", datastar: datastar, form: url.Values{}})
			check(t, "sign", rec.Header().Get("Cache-Control"))
		}
	})
}
