package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	pcops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
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
	// Secrets are the values of secrets the entry names, by secret name. They
	// are written in the same unit of work as the entry, as SECRET values of
	// the application's secrets section at the ingestion's client. Each name
	// must be one the new value binds.
	Secrets map[string]string `json:"secrets,omitempty"`
	// SecretFields are secret values by the path of the setting they belong to
	// ("clientSecret", "auth.password"), for a caller who does not name secrets:
	// the platform names the secret (ingestion.SecretNameFor: the name the value
	// gives there, else the entry's, else one derived from the key and the
	// path), writes the name into the value and the secret under it.
	SecretFields map[string]string `json:"secretFields,omitempty"`
}

// AuditMaskedFields keeps the value and the secrets out of the audit row: the
// value is settings, and a person may put a secret in a text setting. The
// event names the settings and the secrets set, never a value.
func (SetConfigCommand) AuditMaskedFields() []string {
	return []string{"value", "secrets", "secretFields"}
}

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
// of any ingestion.
//
// The values of the secrets the entry names are written in that same unit of
// work (the command's Secrets and SecretFields), into the application's secrets
// section at the ingestion's client as SECRET values, so a person sets an
// ingestion's settings and its secrets in one step. A secret whose value the
// write supplies is the author's own: it does not need the bind-secrets
// permission, unless it names a secret that already has a value which no
// ingestion of the entry binds now (the write would replace someone else's
// secret). It needs the permission to update the ingestion (the
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
			// The refusals name the secret, never its value.
			for name, value := range cmd.Secrets {
				if !ingestion.ValidSecretName(name) {
					return usecase.Validation("INGESTION_CONFIG_INVALID", "secrets: "+name+" is not a secret name (lowercase letters, digits and '-')")
				}
				if err := pcops.ValidSecretValue(value); err != nil {
					return secretValueRefused("secrets: "+name, err)
				}
			}
			for path, value := range cmd.SecretFields {
				if !ingestion.ValidSecretPath(path) {
					return usecase.Validation("INGESTION_CONFIG_INVALID", "secretFields: "+path+" is not the path of a setting, such as password or auth.clientSecret")
				}
				if err := pcops.ValidSecretValue(value); err != nil {
					return secretValueRefused("secretFields: "+path, err)
				}
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
			def, _, _, err := repo.ConfigNamed(ctx, i)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_version failed", err)
			}
			old, err := repo.ConfigEntry(ctx, who.Application, who.Client, key)
			if err != nil {
				return nil, usecase.Internal("REPO", "config entry lookup failed", err)
			}
			named, _ := ingestion.ConfigOf(def)
			value, secrets, err := bindSecretFields(cmd, old, key, named.Base == ingestion.KindCustom)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, usecase.Validation("INGESTION_CONFIG_INVALID", "value does not render: "+err.Error())
			}
			next := &ingestion.ConfigEntry{Value: string(raw)}

			affected, err := repo.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
			if err != nil {
				return nil, usecase.Internal("REPO", "ingestions naming config failed", err)
			}
			if !slices.ContainsFunc(affected, func(a *ingestion.Ingestion) bool { return a.ID == i.ID }) {
				affected = append(affected, i)
			}
			judged, err := judgeEntry(ctx, repo, resolver, who, key, affected, old, next, secrets)
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

			settings := make([]string, 0, len(value))
			for name := range value {
				settings = append(settings, name)
			}
			slices.Sort(settings)
			secretsSet := slices.Sorted(maps.Keys(secrets))
			event := IngestionConfigSet{
				ingestionEvent: newEvent(ec, IngestionConfigSetType, i.ID),
				Code:           i.Code, Key: key, Settings: settings, SecretsSet: secretsSet, Ingestions: judged.codes, Warnings: judged.warnings,
			}
			if len(secrets) == 0 {
				return usecaseop.Save(c, configs, event), nil
			}
			// The entry and its secrets are one change: one transaction, a row
			// event for each config row written (names, never values), and the
			// ingestion's own event as the rollup.
			saves := []usecasepgx.SyncSaveItem[platformconfig.Config]{{Aggregate: c, Event: pcops.NewPropertySet(ec, c)}}
			for _, name := range secretsSet {
				sc, err := configs.FindByCoordinate(ctx, who.Application, ingestion.SecretsSection, name, platformconfig.ScopeClient, new(string(who.Client)))
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
				}
				if sc == nil {
					sc = platformconfig.NewConfig(who.Application, ingestion.SecretsSection, name, secrets[name])
					sc.Scope = platformconfig.ScopeClient
					clientID := who.Client
					sc.ClientID = &clientID
				} else {
					sc.Value = secrets[name]
				}
				sc.ValueType = platformconfig.ValueSecret
				saves = append(saves, usecasepgx.SyncSaveItem[platformconfig.Config]{Aggregate: sc, Event: pcops.NewPropertySet(ec, sc)})
			}
			return usecaseop.Sync(configs, saves, nil, event), nil
		},
	}
}

// bindSecretFields resolves the command's SecretFields into names: for each
// path the platform picks the secret's name (ingestion.SecretNameFor), writes
// it into a copy of the value (plain text for an extension kind, a reference
// for a built-in one) and adds the value under it to the secrets. The
// caller's value is not changed. It returns the value to write and the
// secrets, by name, to write beside it.
func bindSecretFields(cmd SetConfigCommand, old *ingestion.ConfigEntry, key string, plain bool) (map[string]any, map[string]string, error) {
	if len(cmd.SecretFields) == 0 {
		return cmd.Value, cmd.Secrets, nil
	}
	raw, err := json.Marshal(cmd.Value)
	if err != nil {
		return nil, nil, usecase.Validation("INGESTION_CONFIG_INVALID", "value does not render: "+err.Error())
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, nil, usecase.Validation("INGESTION_CONFIG_INVALID", "value does not render: "+err.Error())
	}
	var current map[string]any
	if old != nil && !old.Secret {
		_ = json.Unmarshal([]byte(old.Value), &current)
	}
	secrets := maps.Clone(cmd.Secrets)
	if secrets == nil {
		secrets = map[string]string{}
	}
	for _, path := range slices.Sorted(maps.Keys(cmd.SecretFields)) {
		name, err := ingestion.SecretNameFor(value, current, key, path)
		if err != nil {
			return nil, nil, usecase.Validation("INGESTION_CONFIG_INVALID", "secretFields: "+path+": "+err.Error())
		}
		given := cmd.SecretFields[path]
		if prev, dup := secrets[name]; dup && prev != given {
			return nil, nil, usecase.Validation("INGESTION_CONFIG_INVALID", "secrets: "+name+" is given two different values")
		}
		ingestion.BindSecretName(value, path, name, plain)
		secrets[name] = given
	}
	return value, secrets, nil
}

// secretValueRefused says which secret a refused value is about; the refusal
// itself never holds the value.
func secretValueRefused(which string, err error) error {
	ue := usecase.AsError(err)
	if ue == nil {
		return err
	}
	return usecase.Validation(ue.Code, which+": "+ue.Message)
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
//
// secrets are the values the write supplies, by name. Each must be a name the
// new value binds. The author's bind-secrets rule treats a supplied name as the
// author's own, except one that already has a value which no ingestion of the
// entry binds now: that write would replace a secret the author does not know.
func judgeEntry(ctx context.Context, repo *ingestion.Repository, resolver ingestion.Resolver, who identity, key string,
	affected []*ingestion.Ingestion, old, next *ingestion.ConfigEntry, secrets map[string]string,
) (judged, error) {
	var (
		out      judged
		problems ingestion.Problems
		warned   = map[string]bool{}
		bound    []string
	)
	// The live version under the old value is what runtimes run now; the save
	// is judged against it.
	lives := make(map[string]*ingestion.Definition, len(affected))
	var boundNow []string
	for _, a := range affected {
		if a.Version == 0 {
			continue
		}
		aWho := identity{Application: who.Application, Client: who.Client, Code: a.Code}
		res, err := resolveUnder(ctx, repo, aWho, key, a.Definition, old)
		if err != nil {
			return judged{}, err
		}
		if res.Runnable() {
			lives[a.ID] = &res.Definition
			names, err := repo.SecretNamesOf(ctx, res.Definition.Source)
			if err != nil {
				return judged{}, usecase.Internal("REPO", "live kinds failed", err)
			}
			boundNow = append(boundNow, names...)
		}
	}
	authored, err := authoredSecrets(ctx, repo, who, secrets, boundNow)
	if err != nil {
		return judged{}, err
	}
	for _, a := range affected {
		out.codes = append(out.codes, a.Code)
		defs, err := namingDefinitions(ctx, repo, a, key)
		if err != nil {
			return judged{}, err
		}
		live := lives[a.ID]
		for _, def := range defs {
			res := ingestion.ApplyConfig(def, a.Code, next)
			if res.Status == ingestion.ConfigInvalid {
				for _, p := range res.Problems {
					p.Message = "ingestion " + a.Code + ": " + p.Message
					problems = append(problems, p)
				}
				continue
			}
			names, err := repo.SecretNamesOf(ctx, res.Definition.Source)
			if err != nil {
				return judged{}, usecase.Internal("REPO", "live kinds failed", err)
			}
			bound = append(bound, names...)
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
			change := ingestion.Change{Current: &a.Definition, Live: live, Next: res.Definition, Supplied: authored}
			if err := repo.CheckAuthor(ctx, auth.FromContext(ctx), change); err != nil {
				return judged{}, withIngestion(a.Code, err)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(secrets)) {
		if !slices.Contains(bound, name) && len(problems) == 0 {
			problems = append(problems, ingestion.FieldError{Field: "secrets." + name, Code: "INVALID", Message: "not a secret this entry uses"})
		}
	}
	if len(problems) > 0 {
		return judged{}, problems.Err()
	}
	return out, nil
}

// authoredSecrets are the supplied secrets that are the author's own: those
// that have no value yet, and those an ingestion of the entry already binds
// (rotating a secret the entry uses).
func authoredSecrets(ctx context.Context, repo *ingestion.Repository, who identity, secrets map[string]string, boundNow []string) ([]string, error) {
	if len(secrets) == 0 {
		return nil, nil
	}
	names := slices.Sorted(maps.Keys(secrets))
	missing, err := repo.MissingSecrets(ctx, who.Application, who.Client, names)
	if err != nil {
		return nil, usecase.Internal("REPO", "secrets lookup failed", err)
	}
	return slices.DeleteFunc(names, func(n string) bool {
		return !slices.Contains(missing, n) && !slices.Contains(boundNow, n)
	}), nil
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
