package apiroute

import "github.com/danielgtaylor/huma/v2"

// PlatformAPIConfig builds the huma config for the platform API, including the
// licence and contact the published spec carries.
//
// Both the running server (internal/server) and tools/dump-spec construct their
// config here rather than each calling huma.DefaultConfig, so the committed
// api/openapi.lock.json can never disagree with what the server actually
// serves — which is the drift `make api-diff` exists to catch, and which two
// hand-maintained copies of this metadata would eventually produce.
func PlatformAPIConfig(title, version string) huma.Config {
	cfg := huma.DefaultConfig(title, version)
	// Identifier is an SPDX expression and is mutually exclusive with URL in
	// OpenAPI 3.1, so set only that one.
	cfg.Info.License = &huma.License{
		Name:       "AGPL-3.0-or-later",
		Identifier: "AGPL-3.0-or-later",
	}
	cfg.Info.Contact = &huma.Contact{
		Name:  "FlowCatalyst",
		Email: "support@flowcatalyst.io",
	}
	return cfg
}
