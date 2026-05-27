// dump-spec emits the platform's huma-generated OpenAPI spec to stdout
// without booting a database. Used by `make api-bump` to refresh
// api/openapi.lock.json and by the CI parity-spec job to diff against
// the committed lockfile.
//
// Routes are registered against nil-dep state objects — the spec
// generator inspects the Input/Output struct types, not the handlers
// themselves, so no live pool or repo is required.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	applicationapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/api"
	auditapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit/api"
	authapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/api"
	clientapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/api"
	connectionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/api"
	corsapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/api"
	dispatchjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/api"
	dispatchpoolapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/api"
	emaildomainapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/api"
	eventapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/event/api"
	eventtypeapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/api"
	identityproviderapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/api"
	platformconfigapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/api"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	processapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/api"
	roleapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/api"
	scheduledjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/api"
	serviceaccountapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/api"
	subscriptionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/api"
	webauthnapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
)

func main() {
	httpcompat.Init()

	r := chi.NewMux()
	api := humachi.New(r, huma.DefaultConfig("FlowCatalyst Platform API", "dev"))

	// Register every aggregate that's been migrated to huma. Keep this
	// list in sync with WirePlatform; the parity-spec CI job dumps from
	// here, so a missing line means the route is missing from the
	// committed openapi.lock.json.
	applicationapi.Register(api, &applicationapi.State{})
	auditapi.Register(api, &auditapi.State{})
	authapi.Register(api, &authapi.State{})
	clientapi.Register(api, &clientapi.State{})
	connectionapi.Register(api, &connectionapi.State{})
	corsapi.Register(api, &corsapi.State{})
	dispatchjobapi.Register(api, &dispatchjobapi.State{})
	dispatchpoolapi.Register(api, &dispatchpoolapi.State{})
	emaildomainapi.Register(api, &emaildomainapi.State{})
	eventapi.Register(api, &eventapi.State{})
	eventtypeapi.Register(api, &eventtypeapi.State{})
	identityproviderapi.Register(api, &identityproviderapi.State{})
	platformconfigapi.Register(api, &platformconfigapi.State{})
	principalapi.Register(api, &principalapi.State{})
	processapi.Register(api, &processapi.State{})
	roleapi.Register(api, &roleapi.State{})
	scheduledjobapi.Register(api, &scheduledjobapi.State{})
	serviceaccountapi.Register(api, &serviceaccountapi.State{})
	subscriptionapi.Register(api, &subscriptionapi.State{})
	webauthnapi.Register(api, &webauthnapi.State{})

	spec := api.OpenAPI()
	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal spec:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintln(os.Stderr, "write spec:", err)
		os.Exit(1)
	}
	fmt.Println()
}
