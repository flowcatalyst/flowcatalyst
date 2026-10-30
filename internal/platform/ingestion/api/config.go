package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// registerConfig mounts the routes of the config entry an ingestion's source
// names (docs/ingestion-design.md section 13): the per-environment settings,
// written only through the ingestion.
func registerConfig(g apiroute.Group, s *State) {
	apiroute.Get(g, "getIngestionConfig", "/api/ingestions/{id}/config", "Get the config entry the ingestion's source names (its settings in this environment)", s.getConfig)
	apiroute.Put(g, "setIngestionConfig", "/api/ingestions/{id}/config", "Write the config entry the ingestion's source names, after checking it against every ingestion that names it", http.StatusOK, s.setConfig)
	apiroute.Delete(g, "deleteIngestionConfig", "/api/ingestions/{id}/config", "Delete the config entry the ingestion's source names; the ingestions that name it wait for it", http.StatusOK, s.deleteConfig)
}

// IngestionConfigResponse is the config entry an ingestion's source names.
type IngestionConfigResponse struct {
	IngestionConfigStatus
	Value map[string]any `json:"value,omitempty" doc:"The entry: the kind's settings as an inline source holds them (an extension kind's settings are the object itself), and optionally targetUrl. Absent when there is no entry, when it is not an object, and when valueHidden"`
	// ValueHidden: the entry exists but its value is shown only to a caller who
	// can update ingestions; the status and problems are shown to every viewer.
	ValueHidden bool `json:"valueHidden,omitempty" doc:"The entry exists but its value is withheld: it is shown only to a caller who can update ingestions (ingestion:update). The status and problems are shown regardless"`
	// SchemaChecked says whether a value is checked against the kind's schema
	// when written: always for a built-in kind (its definition is the schema),
	// for an extension kind only while a live runtime reports its settings.
	SchemaChecked bool `json:"schemaChecked" doc:"A value is checked against the kind's schema when written: always for a built-in kind, for an extension's only while a live runtime reports its settings"`
	// Secrets are the secret settings the entry (and, for an extension kind, its
	// schema) has: their names and whether a value is set, never a value.
	Secrets []IngestionConfigSecret `json:"secrets,omitempty" doc:"The secret settings of the entry, for a caller who can update ingestions: the path of each setting, the name of its secret and whether the secret has a value. Never a value. Write a value with secretFields (by field) or secrets (by name)"`
}

// IngestionConfigSecret is the state of one secret setting of an entry.
type IngestionConfigSecret struct {
	Field     string  `json:"field" doc:"The path of the setting in the entry, such as password or auth.clientSecret"`
	Name      string  `json:"name,omitempty" doc:"The name of the secret the entry gives the setting; absent when it names none"`
	Set       bool    `json:"set" doc:"The secret has a value at this client"`
	UpdatedAt *string `json:"updatedAt,omitempty" doc:"When the value was last set, RFC 3339; absent when not set"`
}

// SetIngestionConfigRequest is the wire body of PUT /api/ingestions/{id}/config.
type SetIngestionConfigRequest struct {
	Value map[string]any `json:"value" doc:"The entry: the kind's settings as an inline source holds them, and optionally targetUrl. A secret setting holds the secret's name"`
	// Secrets are written with the entry, in the same transaction.
	Secrets map[string]string `json:"secrets,omitempty" doc:"Secret values by secret name, written with the entry as SECRET values of the application's secrets section at the ingestion's client. Each name must be one the value names. Needs no bind-secrets permission for a name the write gives a value"`
	// SecretFields lets a caller leave the naming to the platform.
	SecretFields map[string]string `json:"secretFields,omitempty" doc:"Secret values by the path of the setting they belong to (password, credential.secretAccessKey, auth.clientSecret). The platform names the secret: the name the value gives the setting, else the one the entry has now, else the entry's key and the path in kebab case (mix-positions-client-secret), writes the name into the value and the secret under it"`
}

// IngestionConfigWritten is the answer of writing the entry.
type IngestionConfigWritten struct {
	IngestionConfigResponse
	Ingestions []string `json:"ingestions" doc:"The codes of every ingestion of the application and client that names the entry, which the value was checked against"`
	Warnings   []string `json:"warnings,omitempty" doc:"SCHEMA_UNCHECKED when no live runtime reports the kind's settings, so the value was not checked against them"`
}

// IngestionConfigDeletedResponse is the answer of deleting the entry.
type IngestionConfigDeletedResponse struct {
	Key        string   `json:"key"`
	Ingestions []string `json:"ingestions" doc:"The codes of every ingestion that names the entry; each is now MISSING and does not run until the entry is set again"`
}

type setConfigInput struct {
	ID   string `path:"id"`
	Body SetIngestionConfigRequest
}

// configResponse reads the entry the ingestion's source names, as the GET
// answers it. The entry's value is left out unless the caller (taken from ctx)
// can update ingestions: the settings name accounts and endpoints, and a
// viewer is told the entry exists and its status, not its content.
func (s *State) configResponse(ctx context.Context, i *ingestion.Ingestion) (*IngestionConfigResponse, error) {
	def, _, named, err := s.Repo.ConfigNamed(ctx, i)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_version failed", err)
	}
	if !named {
		inline := &IngestionConfigResponse{}
		inline.Status = string(ingestion.ConfigInline)
		return inline, nil
	}
	app, err := s.Repo.ApplicationCode(ctx, i.ApplicationID)
	if err != nil {
		return nil, usecase.Internal("REPO", "application lookup failed", err)
	}
	res, err := s.Repo.Resolve(ctx, app, i.ClientID, i.Code, def)
	if err != nil {
		return nil, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	out := &IngestionConfigResponse{IngestionConfigStatus: configStatus(res)}
	if entry, err := s.Repo.ConfigEntry(ctx, app, i.ClientID, res.Key); err != nil {
		return nil, usecase.Internal("REPO", "config entry lookup failed", err)
	} else if entry != nil && !entry.Secret {
		if !canSeeEntries(ctx) {
			out.ValueHidden = true
		} else {
			// An entry that is not a JSON object is left out; its problem says why.
			var value map[string]any
			if json.Unmarshal([]byte(entry.Value), &value) == nil {
				out.Value = value
			}
		}
	}
	out.SchemaChecked = true
	var schema ingestion.KindSchema
	custom, isCustom := ingestion.ConfigOf(def)
	if isCustom = isCustom && custom.Base == ingestion.KindCustom; isCustom {
		if schema, err = s.Repo.SchemaOf(ctx, custom.Type); err != nil {
			return nil, usecase.Internal("REPO", "live kinds failed", err)
		}
		out.SchemaChecked = schema.Checked
	}
	if canSeeEntries(ctx) {
		if out.Secrets, err = s.secretStates(ctx, app, i, res, schema, isCustom); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// secretStates are the secret settings of the entry as resolved: for a
// built-in kind the secrets its effective source names, for an extension the
// settings its schema types SECRET. Only names and whether a value is set.
func (s *State) secretStates(ctx context.Context, app string, i *ingestion.Ingestion, res ingestion.Resolved, schema ingestion.KindSchema, isCustom bool) ([]IngestionConfigSecret, error) {
	var slots []ingestion.SecretSlot
	switch {
	case isCustom:
		if custom, ok := res.Definition.Source.(ingestion.Custom); ok {
			slots = ingestion.CustomSecretSlots(schema.Fields, custom.Settings)
		} else {
			slots = ingestion.CustomSecretSlots(schema.Fields, nil)
		}
	case res.Runnable():
		slots = ingestion.SecretSlots(res.Definition.Source)
	}
	out := make([]IngestionConfigSecret, 0, len(slots))
	for _, slot := range slots {
		state := IngestionConfigSecret{Field: slot.Path, Name: slot.Name}
		if slot.Name != "" {
			c, err := s.Configs.FindByCoordinate(ctx, app, ingestion.SecretsSection, slot.Name, platformconfig.ScopeClient, new(string(i.ClientID)))
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
			}
			if c != nil && c.ValueType == platformconfig.ValueSecret {
				at := c.UpdatedAt.UTC().Format(time.RFC3339)
				state.Set, state.UpdatedAt = true, &at
			}
		}
		out = append(out, state)
	}
	return out, nil
}

func (s *State) getConfig(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionConfigResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	body, err := s.configResponse(ctx, i)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionConfigResponse]{Body: *body}, nil
}

func (s *State) setConfig(ctx context.Context, in *setConfigInput) (*apicommon.Out[IngestionConfigWritten], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	cmd := operations.SetConfigCommand{ID: in.ID, Value: in.Body.Value, Secrets: in.Body.Secrets, SecretFields: in.Body.SecretFields}
	event, err := usecaseop.Run(ctx, s.UoW, operations.SetIngestionConfig(s.Repo, s.Configs, s.Resolver), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	body, err := s.configResponse(ctx, i)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionConfigWritten]{Body: IngestionConfigWritten{
		IngestionConfigResponse: *body, Ingestions: event.Ingestions, Warnings: event.Warnings,
	}}, nil
}

func (s *State) deleteConfig(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionConfigDeletedResponse], error) {
	if err := auth.CanUpdateIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	event, err := usecaseop.Run(ctx, s.UoW, operations.DeleteIngestionConfig(s.Repo, s.Configs), operations.DeleteConfigCommand{ID: in.ID}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionConfigDeletedResponse]{Body: IngestionConfigDeletedResponse{Key: event.Key, Ingestions: event.Ingestions}}, nil
}
