package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
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
	Value map[string]any `json:"value,omitempty" doc:"The entry: the kind's settings as an inline source holds them (an extension kind's settings are the object itself), and optionally targetUrl. Absent when there is no entry or it is not an object"`
	// SchemaChecked says whether a value is checked against the kind's schema
	// when written: always for a built-in kind (its definition is the schema),
	// for an extension kind only while a live runtime reports its settings.
	SchemaChecked bool `json:"schemaChecked" doc:"A value is checked against the kind's schema when written: always for a built-in kind, for an extension's only while a live runtime reports its settings"`
}

// SetIngestionConfigRequest is the wire body of PUT /api/ingestions/{id}/config.
type SetIngestionConfigRequest struct {
	Value map[string]any `json:"value" doc:"The entry: the kind's settings as an inline source holds them, and optionally targetUrl. A secret setting holds the secret's name"`
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
// answers it.
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
		// An entry that is not a JSON object is left out; its problem says why.
		var value map[string]any
		if json.Unmarshal([]byte(entry.Value), &value) == nil {
			out.Value = value
		}
	}
	out.SchemaChecked = true
	if custom, ok := ingestion.ConfigOf(def); ok && custom.Base == ingestion.KindCustom {
		schema, err := s.Repo.SchemaOf(ctx, custom.Type)
		if err != nil {
			return nil, usecase.Internal("REPO", "live kinds failed", err)
		}
		out.SchemaChecked = schema.Checked
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
	cmd := operations.SetConfigCommand{ID: in.ID, Value: in.Body.Value}
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
