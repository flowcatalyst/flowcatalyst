package function

import "testing"

func TestValidHostname(t *testing.T) {
	cases := map[string]bool{
		"acme.example.com": true,
		"api.acme.com":     true,
		"localhost":        true,
		"":                 false,
		"Acme.example.com": false,
		"acme.com:8080":    false,
		"*.acme.com":       false,
	}
	for h, want := range cases {
		if got := ValidHostname(h); got != want {
			t.Errorf("ValidHostname(%q) = %v, want %v", h, got, want)
		}
	}
}

func TestNormalizeHostname(t *testing.T) {
	cases := map[string]string{
		"Acme.Example.COM.": "acme.example.com",
		"  api.acme.com  ":  "api.acme.com",
		"acme.com":          "acme.com",
	}
	for in, want := range cases {
		if got := NormalizeHostname(in); got != want {
			t.Errorf("NormalizeHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizePathPrefix(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		"webhooks":      "/webhooks",
		"/webhooks":     "/webhooks",
		"/webhooks/":    "/webhooks",
		"//webhooks//":  "/webhooks",
		"  /webhooks  ": "/webhooks",
	}
	for in, want := range cases {
		if got := NormalizePathPrefix(in); got != want {
			t.Errorf("NormalizePathPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
