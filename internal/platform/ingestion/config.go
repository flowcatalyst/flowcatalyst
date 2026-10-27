package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// ConfigStatus says where an ingestion's settings stand in this environment
// (docs/ingestion-design.md section 13).
type ConfigStatus string

const (
	// ConfigInline: the source holds its settings; there is no entry.
	ConfigInline ConfigStatus = "INLINE"
	// ConfigSet: the source names an entry and its value makes a valid
	// definition; the ingestion runs as the effective definition says.
	ConfigSet ConfigStatus = "SET"
	// ConfigMissing: the source names an entry this environment does not have
	// (for this client); the ingestion does not run until it does.
	ConfigMissing ConfigStatus = "MISSING"
	// ConfigInvalid: the entry exists but its value does not make a valid
	// definition; the ingestion does not run until it does.
	ConfigInvalid ConfigStatus = "INVALID"
)

// The warnings a reference adds to a save; they are the text before the colon
// of an entry in a response's warnings.
const (
	WarnConfigMissing    = "CONFIG_MISSING"
	WarnSchemaUnchecked  = "SCHEMA_UNCHECKED"
	configMissingMessage = "the config entry is not set for this client in this environment; the ingestion does not run until it is"
)

// ConfigEntry is a config entry as read: the platform config row of section
// ingest, scope CLIENT.
type ConfigEntry struct {
	// Value is the JSON object the entry holds.
	Value string
	// Secret is true for a row whose type is SECRET, which an entry never is:
	// it is read as invalid, and its value is never looked at.
	Secret    bool
	UpdatedAt time.Time
}

// Resolved is a definition read against its config entry (ApplyConfig).
type Resolved struct {
	// Key is the entry's key; empty when the settings are inline.
	Key    string
	Status ConfigStatus
	// Definition is what runs: the effective definition when the status is SET
	// (inline settings are their own), otherwise the definition as it was
	// given, whose source is still the reference and must not be read for its
	// settings.
	Definition Definition
	// Problems are why the entry is INVALID.
	Problems Problems
	// UpdatedAt is when the entry was last written, when there is one.
	UpdatedAt *time.Time
}

// Runnable reports whether the ingestion can run as Definition says: its
// settings are inline, or its entry is set and valid.
func (r Resolved) Runnable() bool { return r.Status == ConfigInline || r.Status == ConfigSet }

// Redacted is r as a caller who may not read the entry's settings sees it: the
// status, problems and update time stay, and the definition is the reference as
// it was given (reference), so nothing built from it (where the data comes
// from, the delivery URL, the secret names, an effective definition's
// warnings) quotes the entry. Settings inline have no entry: r is unchanged.
func (r Resolved) Redacted(reference Definition) Resolved {
	if r.Status == ConfigInline {
		return r
	}
	r.Definition = reference
	return r
}

// Warnings are what the definition says that saves but will not run as meant:
// an effective definition's own (an sftp source that checks no host key), and
// CONFIG_MISSING for an entry that is not set.
func (r Resolved) Warnings() []string {
	var out []string
	if r.Status == ConfigMissing {
		out = append(out, WarnConfigMissing+": "+configMissingMessage+" (entry "+r.Key+")")
	}
	if r.Runnable() {
		out = append(out, r.Definition.Warnings()...)
	}
	return out
}

// ApplyConfig reads def, the definition of the ingestion with this code,
// against the value of the entry it names (nil: there is none). It is the pure
// core of Resolve:
//
//   - settings inline: the definition itself, INLINE;
//   - no entry: MISSING;
//   - an entry whose value, given the way an inline source holds its settings
//     (the same fields, strictly decoded) and with the delivery URL either in
//     the definition or in the value's targetUrl (exactly one of them), makes a
//     definition that passes Validate: that effective definition, SET;
//   - otherwise INVALID, with what is wrong.
func ApplyConfig(def Definition, code string, entry *ConfigEntry) Resolved {
	c, ok := def.Source.(Configured)
	if !ok {
		return Resolved{Status: ConfigInline, Definition: def}
	}
	r := Resolved{Key: c.Config.KeyFor(code), Definition: def}
	if entry == nil {
		r.Status = ConfigMissing
		return r
	}
	updated := entry.UpdatedAt
	r.UpdatedAt = &updated
	effective, problems := effectiveDefinition(def, c, entry)
	if len(problems) > 0 {
		r.Status, r.Problems = ConfigInvalid, problems
		return r
	}
	r.Status, r.Definition = ConfigSet, effective
	return r
}

// entrySettings splits an entry's value into the delivery URL it may carry and
// the kind's settings, reporting what is wrong with the shape of the value.
func entrySettings(entry *ConfigEntry) (targetURL string, settings map[string]json.RawMessage, problems Problems) {
	var v checker
	if entry.Secret {
		v.add("config", "INVALID", "the entry is a secret value; an ingestion's settings are a PLAIN entry holding a JSON object")
		return "", nil, v.problems
	}
	if err := json.Unmarshal([]byte(entry.Value), &settings); err != nil || settings == nil {
		v.add("config", "INVALID", "must be a JSON object: the kind's settings, and optionally targetUrl")
		return "", nil, v.problems
	}
	if raw, ok := settings["targetUrl"]; ok {
		if err := json.Unmarshal(raw, &targetURL); err != nil {
			v.add("config.targetUrl", "INVALID", "must be text: the delivery URL")
		}
		delete(settings, "targetUrl")
	}
	for _, name := range []string{"kind", "config"} {
		if _, ok := settings[name]; ok {
			v.add("config."+name, "INVALID", "is not a setting: the definition names the kind and the entry")
		}
	}
	return targetURL, settings, v.problems
}

// settingsProblem says why a value does not decode into the kind's settings,
// without quoting it: the decoder's own error names the offending value (a
// number's text, say), and a problem is shown to every viewer of the
// ingestion, who may not read the entry. A wrong type is told as the field and
// the type it wants, an unknown field by its name; anything else generically.
func settingsProblem(err error) string {
	if te, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && te.Field != "" {
		return "settings." + te.Field + " must be of type " + te.Type.String()
	}
	if _, name, ok := strings.Cut(err.Error(), "json: unknown field "); ok {
		return "settings has an unknown field " + name
	}
	return "the settings do not fit the kind: a field has the wrong type or shape"
}

// effectiveDefinition is def with the entry's settings in its source, and its
// delivery URL when the definition gives none.
func effectiveDefinition(def Definition, c Configured, entry *ConfigEntry) (Definition, Problems) {
	targetURL, settings, problems := entrySettings(entry)
	if len(problems) > 0 {
		return Definition{}, problems
	}
	var src Source
	if c.Base == KindCustom {
		custom := Custom{Type: c.Type}
		if len(settings) > 0 {
			rest, err := json.Marshal(settings)
			if err == nil {
				err = json.Unmarshal(rest, &custom.Settings)
			}
			if err != nil {
				return Definition{}, Problems{{Field: "config", Code: "INVALID", Message: err.Error()}}
			}
		}
		src = custom
	} else {
		body := make(map[string]json.RawMessage, len(settings)+1)
		maps.Copy(body, settings)
		body["kind"], _ = json.Marshal(c.Base)
		raw, err := json.Marshal(body)
		if err == nil {
			src, err = UnmarshalSource(raw)
		}
		if err != nil {
			return Definition{}, Problems{{Field: "config", Code: "INVALID", Message: settingsProblem(err)}}
		}
	}
	eff := def
	eff.Source = src
	var v checker
	switch {
	case def.Target.TargetURL != "" && targetURL != "":
		v.add("target.targetUrl", "INVALID", "is given in the definition and in the config entry: give the delivery URL in one of them")
	case def.Target.TargetURL == "" && targetURL == "":
		v.add("target.targetUrl", "INVALID", "is required: give the delivery URL in the definition or as targetUrl in the config entry")
	case def.Target.TargetURL == "":
		eff.Target.TargetURL = targetURL
	}
	return eff, append(v.problems, eff.Validate()...)
}

// Resolve reads the entry def's source names for the ingestion with this code
// and client, and applies it. Settings inline: no read is made.
func (r *Repository) Resolve(ctx context.Context, applicationCode string, clientID ids.ClientID, code string, def Definition) (Resolved, error) {
	c, ok := def.Source.(Configured)
	if !ok {
		return Resolved{Status: ConfigInline, Definition: def}, nil
	}
	entry, err := r.ConfigEntry(ctx, applicationCode, clientID, c.Config.KeyFor(code))
	if err != nil {
		return Resolved{}, err
	}
	return ApplyConfig(def, code, entry), nil
}

// ResolveOf is Resolve for an ingestion that is saved, whose application is
// known by id.
func (r *Repository) ResolveOf(ctx context.Context, i *Ingestion, def Definition) (Resolved, error) {
	if _, ok := def.Source.(Configured); !ok {
		return Resolved{Status: ConfigInline, Definition: def}, nil
	}
	app, err := r.ApplicationCode(ctx, i.ApplicationID)
	if err != nil {
		return Resolved{}, err
	}
	return r.Resolve(ctx, app, i.ClientID, i.Code, def)
}

// ApplicationCode is the code of an application.
func (r *Repository) ApplicationCode(ctx context.Context, id ids.ApplicationID) (string, error) {
	code, err := r.q.IngestionApplicationCode(ctx, id)
	if err != nil {
		return "", fmt.Errorf("ingestion repo: application code: %w", err)
	}
	return code, nil
}

// ConfigEntry reads the entry of the key for the application at the client,
// nil when there is none.
func (r *Repository) ConfigEntry(ctx context.Context, applicationCode string, clientID ids.ClientID, key string) (*ConfigEntry, error) {
	return configEntry(ctx, r.q, applicationCode, clientID, key)
}

func configEntry(ctx context.Context, q *dbq.Queries, applicationCode string, clientID ids.ClientID, key string) (*ConfigEntry, error) {
	row, err := q.IngestionConfigEntryFind(ctx, dbq.IngestionConfigEntryFindParams{
		ApplicationCode: applicationCode, ClientID: &clientID, Property: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: config entry: %w", err)
	}
	return &ConfigEntry{Value: row.Value, Secret: row.ValueType != "PLAIN", UpdatedAt: row.UpdatedAt}, nil
}

// NamingConfig lists the ingestions of the application and client whose live or
// latest saved version names the entry of the key, in id order.
func (r *Repository) NamingConfig(ctx context.Context, applicationID ids.ApplicationID, clientID ids.ClientID, key string) ([]*Ingestion, error) {
	idList, err := r.q.IngestionsNamingConfig(ctx, dbq.IngestionsNamingConfigParams{ApplicationID: applicationID, ClientID: clientID, Key: key})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: ingestions naming config: %w", err)
	}
	out := make([]*Ingestion, 0, len(idList))
	for _, id := range idList {
		i, err := r.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if i != nil {
			out = append(out, i)
		}
	}
	return out, nil
}

// ConfigNamed is the definition of the ingestion that names a config entry,
// for the entry endpoints: the live version's when it names one (what runs),
// otherwise the latest saved version's (a draft that will). ok is false when
// neither does.
func (r *Repository) ConfigNamed(ctx context.Context, i *Ingestion) (def Definition, key string, ok bool, err error) {
	if k := ConfigKeyOf(i.Definition, i.Code); k != "" {
		return i.Definition, k, true, nil
	}
	if i.LatestVersion > i.Version {
		v, err := r.FindVersion(ctx, i.ID, i.LatestVersion)
		if err != nil {
			return Definition{}, "", false, err
		}
		if v != nil {
			if k := ConfigKeyOf(v.Definition, i.Code); k != "" {
				return v.Definition, k, true, nil
			}
		}
	}
	return Definition{}, "", false, nil
}

// resolveListed resolves the settings of the ingestions of a poll with one
// read of the entries, inside the poll's transaction: an ingestion that names
// an entry is given its effective definition, and is left out while the entry
// is missing or invalid (it takes no lease and does not count in the fair
// share). A runtime never sees a reference.
func resolveListed(ctx context.Context, q *dbq.Queries, items []ListItem) ([]ListItem, error) {
	if !anyConfigured(items) {
		return items, nil
	}
	rows, err := q.IngestionConfigEntriesList(ctx)
	if err != nil {
		return nil, fmt.Errorf("config entries: %w", err)
	}
	type coordinate struct{ app, client, key string }
	entries := make(map[coordinate]*ConfigEntry, len(rows))
	for _, row := range rows {
		if row.ClientID == nil {
			continue
		}
		entries[coordinate{row.ApplicationCode, string(*row.ClientID), row.Property}] = &ConfigEntry{
			Value: row.Value, Secret: row.ValueType != "PLAIN", UpdatedAt: row.UpdatedAt,
		}
	}
	out := make([]ListItem, 0, len(items))
	for _, item := range items {
		c, ok := item.Definition.Source.(Configured)
		if !ok {
			out = append(out, item)
			continue
		}
		key := c.Config.KeyFor(item.Code)
		res := ApplyConfig(item.Definition, item.Code, entries[coordinate{item.ApplicationCode, string(item.ClientID), key}])
		if !res.Runnable() {
			slog.Debug("ingestion not listed: its config entry is not usable",
				"ingestion_id", item.ID, "key", key, "status", res.Status)
			continue
		}
		item.Definition = res.Definition
		out = append(out, item)
	}
	return out, nil
}

func anyConfigured(items []ListItem) bool {
	for _, item := range items {
		if _, ok := item.Definition.Source.(Configured); ok {
			return true
		}
	}
	return false
}

// DescribeProblems is the problems of an entry in one line, for a message.
func DescribeProblems(p Problems) string { return strings.TrimSpace(p.String()) }
