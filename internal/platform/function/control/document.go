package control

import (
	"context"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// buildDesired renders pool's desired document (docs/function-runner-plan.md
// §8.3): every function assigned to pool, with its settings decrypted, its
// webhook signing secret, and the versions a runner should hold. Only versions
// whose runtime is in runtimes are included (the runner's declared set); a
// function whose versions were ALL filtered out is left out, with its routes.
func (s *State) buildDesired(ctx context.Context, pool string, runtimes map[string]bool) (*fncontrol.Desired, error) {
	revision, _, err := s.Repo.GetPoolRevision(ctx, pool)
	if err != nil {
		return nil, usecase.Internal("REPO", "get_pool_revision failed", err)
	}
	functions, err := s.Repo.FindFunctionsByPool(ctx, pool)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_functions_by_pool failed", err)
	}
	routes, addresses, err := s.Repo.ListRoutesByPool(ctx, pool)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_routes_by_pool failed", err)
	}

	doc := &fncontrol.Desired{
		Pool:      pool,
		Revision:  revision,
		Auth:      s.Auth,
		Functions: make([]fncontrol.Function, 0, len(functions)),
		// Routes: the pool's public routes (plan §8, "public routes"). Address
		// comes from the owning function (joined in ListRoutesByPool), Alias
		// as stored ("" = live).
		Routes: make([]fncontrol.Route, 0, len(routes)),
	}
	for i, rt := range routes {
		alias := ""
		if rt.Alias != nil {
			alias = *rt.Alias
		}
		doc.Routes = append(doc.Routes, fncontrol.Route{
			Hostname:      rt.Hostname,
			PathPrefix:    rt.PathPrefix,
			Address:       addresses[i],
			Alias:         alias,
			AliasPrefixes: rt.AliasPrefixes,
		})
	}
	dropped := map[string]bool{} // addresses with nothing this runner can run
	for i := range functions {
		fdoc, hadVersions, err := s.buildFunction(ctx, &functions[i], runtimes)
		if err != nil {
			return nil, err
		}
		if hadVersions && len(fdoc.Versions) == 0 {
			dropped[fdoc.Address] = true
			continue
		}
		doc.Functions = append(doc.Functions, fdoc)
	}
	if len(dropped) > 0 {
		kept := doc.Routes[:0]
		for _, rt := range doc.Routes {
			if !dropped[rt.Address] {
				kept = append(kept, rt)
			}
		}
		doc.Routes = kept
	}
	return doc, nil
}

func (s *State) buildFunction(ctx context.Context, fn *function.Function, runtimes map[string]bool) (fdoc fncontrol.Function, hadVersions bool, err error) {
	settings, err := s.Repo.ResolveSettingsForControl(ctx, fn.ID)
	if err != nil {
		return fncontrol.Function{}, false, usecase.Internal("REPO", "resolve_settings_for_control failed", err)
	}
	all, err := s.buildVersions(ctx, fn)
	if err != nil {
		return fncontrol.Function{}, false, err
	}
	versions := make([]fncontrol.Version, 0, len(all))
	for _, v := range all {
		if runtimes[runtimeOf(v.Runtime)] {
			versions = append(versions, v)
		}
	}

	var webhookSecret string
	if s.Creds != nil {
		creds, err := s.Creds(ctx, fn.ApplicationID)
		if err != nil {
			return fncontrol.Function{}, false, usecase.Internal("SERVICE_ACCOUNT_CREDS", "outbound creds lookup failed", err)
		}
		webhookSecret = creds.SigningSecret
	}

	return fncontrol.Function{
		ID:            fn.ID,
		Address:       fn.Address,
		ApplicationID: fn.ApplicationID,
		ClientID:      fn.ClientID,
		Limits:        limitsOf(fn.Limits),
		Warm:          fn.Warm,
		WebhookSecret: webhookSecret,
		Config:        settings.Config,
		Secrets:       settings.Secrets,
		DB:            settings.DB,
		Versions:      versions,
	}, len(all) > 0, nil
}

// runtimeOf resolves a stored version runtime ("" means the default).
func runtimeOf(r string) string {
	if r == "" {
		return function.DefaultRuntime
	}
	return r
}

func limitsOf(l function.Limits) fncontrol.Limits {
	return fncontrol.Limits{
		MemoryMB:       int(l.MemoryMB),
		MaxConcurrency: int(l.MaxConcurrency),
		TimeoutMs:      int64(l.TimeoutMs),
		MaxBodyBytes:   l.MaxBodyBytes,
	}
}

// buildVersions selects the versions a runner should hold for fn: every
// version named by an alias (roles "live"/"alias:<name>"), plus every
// PUBLISHED version (role "candidate") so runners can prove them loadable
// before anyone promotes. A version named by neither is dropped — RETIRED
// and FAILED versions unless still aliased, and a READY version no alias
// names: it already proved itself loadable at PUBLISHED, so once it is no
// longer a candidate and nothing points at it, nothing needs it held.
func (s *State) buildVersions(ctx context.Context, fn *function.Function) ([]fncontrol.Version, error) {
	aliases, err := s.Repo.ListAliases(ctx, fn.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_aliases failed", err)
	}
	rolesByVersionID := map[string][]string{}
	for _, a := range aliases {
		role := fncontrol.RoleAlias + a.Name
		if a.Name == "live" {
			role = fncontrol.RoleLive
		}
		rolesByVersionID[a.VersionID] = append(rolesByVersionID[a.VersionID], role)
	}

	all, err := s.Repo.ListVersionsByFunction(ctx, fn.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_versions_by_function failed", err)
	}
	out := make([]fncontrol.Version, 0, len(all))
	for _, v := range all {
		roles := rolesByVersionID[v.ID]
		if v.Status == function.VersionPublished {
			roles = append(roles, fncontrol.RoleCandidate)
		}
		if len(roles) == 0 {
			continue
		}
		out = append(out, fncontrol.Version{
			Number:   int(v.Number),
			Digest:   v.Digest,
			Runtime:  v.Runtime,
			ABI:      int(v.ABI),
			Describe: v.Describe,
			Roles:    roles,
		})
	}
	return out, nil
}
