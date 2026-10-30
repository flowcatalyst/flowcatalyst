package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// SetConfigCommand is the input DTO of writing the config entry an ingestion
// names (docs/ingestion-design.md section 13).
type SetConfigCommand struct {
	// ID is the ingestion the caller writes the entry through. The entry is
	// the application's, at the ingestion's client, under the key the
	// ingestion's source names.
	ID string `json:"id"`
	// Value is the entry: the kind's settings, as an inline source holds them,
	// and optionally targetUrl.
	Value map[string]any `json:"value"`
}

// AuditMaskedFields keeps the value out of the audit row: it is settings, and
// a person may put a secret in a text setting. The event names the settings.
func (SetConfigCommand) AuditMaskedFields() []string { return []string{"value"} }

// SetIngestionConfig writes the config entry the ingestion names, and emits
// [IngestionConfigSet] on the ingestion.
//
// The entry is shared by every ingestion of the application and client whose
// live or latest saved version names its key, so the new value must suit each
// of them: for each, the effective definition under the new value passes
// Validate, the delivery-URL guard, and (for an extension kind a live runtime
// reports) the kind's schema, and the caller may save it over what is live
// under the old value (ingestion.Repository.CheckAuthor: the kind's
// permission, the runtime identity, bind-secrets when it points secrets
// somewhere new). Nothing is written unless all of them pass. The entry is
// written through the platform config repository in the same unit of work
// (not through SetProperty, which refuses the section), and is not a version
// of any ingestion. It needs the permission to update the ingestion (the
// controller's), also for an ingestion a set owns: the entry is the
// environment's part of it, not the set's.
func SetIngestionConfig(repo *ingestion.Repository, configs *platformconfig.Repository, resolver ingestion.Resolver) usecaseop.Operation[SetConfigCommand, IngestionConfigSet] {
	return usecaseop.Operation[SetConfigCommand, IngestionConfigSet]{
		Name: "SetIngestionConfig",
		Validate: func(_ context.Context, cmd SetConfigCommand) error {
			if err := requireID(cmd.ID); err != nil {
				return err
			}
			if cmd.Value == nil {
				return usecase.Validation("INGESTION_CONFIG_INVALID", "value is required: a JSON object of the kind's settings")
			}
			return nil
		},
		// Per-resource authz (the ingestion's reach, the kind's permission, the
		// bind-secrets rule) runs in Execute, once the ingestions the entry
		// serves are known; the coarse "may update ingestions" permission is
		// the controller's.
		Authorize: usecaseop.Public[SetConfigCommand],
		Execute: func(ctx context.Context, cmd SetConfigCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionConfigSet], error) {
			i, who, key, err := loadConfigured(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(cmd.Value)
			if err != nil {
				return nil, usecase.Validation("INGESTION_CONFIG_INVALID", "value does not render: "+err.Error())
			}
			old, err := repo.ConfigEntry(ctx, who.Application, who.Client, key)
			if err != nil {
				return nil, usecase.Internal("REPO", "config entry lookup failed", err)
			}
			next := &ingestion.ConfigEntry{Value: string(raw)}

			affected, err := repo.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
			if err != nil {
				return nil, usecase.Internal("REPO", "ingestions naming config failed", err)
			}
			if !slices.ContainsFunc(affected, func(a *ingestion.Ingestion) bool { return a.ID == i.ID }) {
				affected = append(affected, i)
			}
			judged, err := judgeEntry(ctx, repo, resolver, who, key, affected, old, next)
			if err != nil {
				return nil, err
			}

			c, err := configs.FindByCoordinate(ctx, who.Application, ingestion.ConfigSection, key, platformconfig.ScopeClient, new(string(who.Client)))
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
			}
			if c == nil {
				c = platformconfig.NewConfig(who.Application, ingestion.ConfigSection, key, string(raw))
				c.Scope = platformconfig.ScopeClient
				clientID := who.Client
				c.ClientID = &clientID
			} else {
				c.Value = string(raw)
			}
			c.ValueType = platformconfig.ValuePlain

			settings := make([]string, 0, len(cmd.Value))
			for name := range cmd.Value {
				settings = append(settings, name)
			}
			slices.Sort(settings)
			event := IngestionConfigSet{
				ingestionEvent: newEvent(ec, IngestionConfigSetType, i.ID),
				Code:           i.Code, Key: key, Settings: settings, Ingestions: judged.codes, Warnings: judged.warnings,
			}
			return usecaseop.Save(c, configs, event), nil
		},
	}
}

// DeleteConfigCommand is the input DTO of deleting the entry an ingestion names.
type DeleteConfigCommand struct {
	ID string `json:"id"`
}

// DeleteIngestionConfig deletes the config entry the ingestion names and emits
// [IngestionConfigDeleted]. Every ingestion that names it is then waiting for
// it (MISSING) and stops running; the event lists them. It needs the same
// permission as writing the entry, and no more: nothing is saved over
// anything, so there is no definition to judge.
func DeleteIngestionConfig(repo *ingestion.Repository, configs *platformconfig.Repository) usecaseop.Operation[DeleteConfigCommand, IngestionConfigDeleted] {
	return usecaseop.Operation[DeleteConfigCommand, IngestionConfigDeleted]{
		Name: "DeleteIngestionConfig",
		Validate: func(_ context.Context, cmd DeleteConfigCommand) error {
			return requireID(cmd.ID)
		},
		// Per-resource authz runs in Execute; the coarse "may update
		// ingestions" permission is the controller's.
		Authorize: usecaseop.Public[DeleteConfigCommand],
		Execute: func(ctx context.Context, cmd DeleteConfigCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionConfigDeleted], error) {
			i, who, key, err := loadConfigured(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			c, err := configs.FindByCoordinate(ctx, who.Application, ingestion.ConfigSection, key, platformconfig.ScopeClient, new(string(who.Client)))
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
			}
			if c == nil {
				return nil, usecase.NotFound("INGESTION_CONFIG_NOT_FOUND", "the config entry "+key+" is not set")
			}
			affected, err := repo.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
			if err != nil {
				return nil, usecase.Internal("REPO", "ingestions naming config failed", err)
			}
			codes := make([]string, 0, len(affected))
			for _, a := range affected {
				codes = append(codes, a.Code)
			}
			if !slices.Contains(codes, i.Code) {
				codes = append(codes, i.Code)
			}
			event := IngestionConfigDeleted{
				ingestionEvent: newEvent(ec, IngestionConfigDeletedType, i.ID),
				Code:           i.Code, Key: key, Ingestions: codes,
			}
			return usecaseop.Delete(c, configs, event), nil
		},
	}
}

// loadConfigured loads the ingestion the caller writes an entry through, which
// the caller reaches, and the key its source names (the live version's, else
// the latest saved one's).
func loadConfigured(ctx context.Context, repo *ingestion.Repository, id string) (*ingestion.Ingestion, identity, string, error) {
	i, err := load(ctx, repo, id)
	if err != nil {
		return nil, identity{}, "", err
	}
	_, key, ok, err := repo.ConfigNamed(ctx, i)
	if err != nil {
		return nil, identity{}, "", usecase.Internal("REPO", "find_version failed", err)
	}
	if !ok {
		return nil, identity{}, "", usecase.Validation("INGESTION_CONFIG_NOT_NAMED",
			"the ingestion's source holds its settings inline; name an entry with source.config first")
	}
	who, err := identityOf(ctx, repo, i)
	if err != nil {
		return nil, identity{}, "", err
	}
	return i, who, key, nil
}

// judged is what judgeEntry found out about a new value.
type judged struct {
	codes    []string
	warnings []string
}

// judgeEntry holds the new value against every ingestion that names the entry
// (the live and the latest saved version of each that does): the effective
// definition must validate and pass the schema and the delivery-URL guard, and
// the caller may save it over the live one under the old value. Problems of
// all of them are returned together, naming the ingestion.
func judgeEntry(ctx context.Context, repo *ingestion.Repository, resolver ingestion.Resolver, who identity, key string,
	affected []*ingestion.Ingestion, old, next *ingestion.ConfigEntry,
) (judged, error) {
	var (
		out      judged
		problems ingestion.Problems
		warned   = map[string]bool{}
	)
	for _, a := range affected {
		out.codes = append(out.codes, a.Code)
		aWho := identity{Application: who.Application, Client: who.Client, Code: a.Code}
		defs, err := namingDefinitions(ctx, repo, a, key)
		if err != nil {
			return judged{}, err
		}
		// The live version under the old value is what runtimes run now; the
		// save is judged against it.
		var live *ingestion.Definition
		if a.Version > 0 {
			res, err := resolveUnder(ctx, repo, aWho, key, a.Definition, old)
			if err != nil {
				return judged{}, err
			}
			if res.Runnable() {
				live = &res.Definition
			}
		}
		for _, def := range defs {
			res := ingestion.ApplyConfig(def, a.Code, next)
			if res.Status == ingestion.ConfigInvalid {
				for _, p := range res.Problems {
					p.Message = "ingestion " + a.Code + ": " + p.Message
					problems = append(problems, p)
				}
				continue
			}
			if custom, ok := def.Source.(ingestion.Configured); ok && custom.Base == ingestion.KindCustom {
				schema, err := repo.SchemaOf(ctx, custom.Type)
				if err != nil {
					return judged{}, usecase.Internal("REPO", "live kinds failed", err)
				}
				switch {
				case !schema.Checked:
					if !warned[custom.Type] {
						warned[custom.Type] = true
						out.warnings = append(out.warnings, ingestion.WarnSchemaUnchecked+": no live runtime reports the settings of "+
							custom.Type+", so the value was not checked against them; the runtime checks it before it builds the route")
					}
				default:
					settings, _ := res.Definition.Source.(ingestion.Custom)
					for _, p := range ingestion.CheckSettings(schema.Fields, settings.Settings) {
						p.Message = "ingestion " + a.Code + ": " + p.Message
						problems = append(problems, p)
					}
				}
			}
			if res.Definition.Target.TargetURL != def.Target.TargetURL {
				// The value gave the delivery URL: it passes the guard as a
				// target in the definition does.
				if _, err := resolver.Target(ctx, who.Application, who.Client, res.Definition.Target); err != nil {
					return judged{}, withIngestion(a.Code, err)
				}
			}
			change := ingestion.Change{Current: &a.Definition, Live: live, Next: res.Definition}
			if err := repo.CheckAuthor(ctx, auth.FromContext(ctx), change); err != nil {
				return judged{}, withIngestion(a.Code, err)
			}
		}
	}
	if len(problems) > 0 {
		return judged{}, problems.Err()
	}
	return out, nil
}

// namingDefinitions are the definitions of the ingestion, live and latest saved
// (once when they are one version), that name the key.
func namingDefinitions(ctx context.Context, repo *ingestion.Repository, a *ingestion.Ingestion, key string) ([]ingestion.Definition, error) {
	var out []ingestion.Definition
	if ingestion.ConfigKeyOf(a.Definition, a.Code) == key {
		out = append(out, a.Definition)
	}
	if a.LatestVersion > a.Version {
		v, err := repo.FindVersion(ctx, a.ID, a.LatestVersion)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_version failed", err)
		}
		if v != nil && ingestion.ConfigKeyOf(v.Definition, a.Code) == key {
			out = append(out, v.Definition)
		}
	}
	return out, nil
}

// resolveUnder resolves def with the entry of key taken to be entry (nil: not
// set) instead of what is stored; a definition that names another entry reads
// that one as it is.
func resolveUnder(ctx context.Context, repo *ingestion.Repository, who identity, key string, def ingestion.Definition, entry *ingestion.ConfigEntry) (ingestion.Resolved, error) {
	if ingestion.ConfigKeyOf(def, who.Code) == key {
		return ingestion.ApplyConfig(def, who.Code, entry), nil
	}
	res, err := repo.Resolve(ctx, who.Application, who.Client, who.Code, def)
	if err != nil {
		return ingestion.Resolved{}, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	return res, nil
}

// withIngestion says which ingestion a refusal is about, keeping its kind and
// code so the caller still sees the right status.
func withIngestion(code string, err error) error {
	ue := usecase.AsError(err)
	if ue == nil {
		return err
	}
	at := *ue
	at.Message = fmt.Sprintf("ingestion %s: %s", code, ue.Message)
	return &at
}
