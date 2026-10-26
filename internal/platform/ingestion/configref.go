package ingestion

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ConfigSection is the platform config section that holds the per-environment
// settings of ingestions (docs/ingestion-design.md section 13). Its entries are
// written only through the ingestion that names them; platformconfig refuses
// every other write in it (platformconfig.ReservedSection is the same name).
const ConfigSection = "ingest"

// ConfigRef is how a source names its config entry: `config: true` is the
// ingestion's own code, `config: <key>` an explicit key (a key follows the
// rules of a code, so several ingestions of one application and client may
// share an entry).
type ConfigRef struct {
	// Own is `config: true`: the key is the ingestion's code, known only where
	// the code is (a definition alone keeps the reference as it is).
	Own bool
	// Key is the explicit key; empty when Own.
	Key string
}

// KeyFor is the entry's key for the ingestion with this code.
func (c ConfigRef) KeyFor(code string) string {
	if c.Own {
		return code
	}
	return c.Key
}

func (c ConfigRef) MarshalJSON() ([]byte, error) {
	if c.Own {
		return []byte("true"), nil
	}
	return json.Marshal(c.Key)
}

func (c *ConfigRef) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		if !x {
			return errors.New("config is true (the ingestion's own code) or the key of the entry")
		}
		*c = ConfigRef{Own: true}
	case string:
		*c = ConfigRef{Key: x}
	default:
		return errors.New("config is true (the ingestion's own code) or the key of the entry")
	}
	return nil
}

// Configured is a source whose settings are not in the definition but in a
// platform config entry of the environment: {kind, config} for a built-in
// kind, {kind: custom, type, config} for an extension's. It is one more member
// of the Source union, reporting the kind it stands for (Kind, SingleOwner and
// RunKind answer as the inline source would), so everything that routes by
// kind (the poll, the fair share, the list filter, the kind permission, the
// carried kinds) keeps working; the settings only appear once the entry is
// read (Resolve, ApplyConfig), and nothing that reads settings (secret names,
// outbound addresses, the runtime identity) is ever given a Configured source:
// each is given the effective definition.
type Configured struct {
	// Base is the kind the source stands for: a built-in kind, or custom.
	Base string
	// Type is the extension's kind when Base is custom, otherwise empty.
	Type   string
	Config ConfigRef
	// Others names the settings the definition gave beside config. A source
	// holds its settings inline or names an entry, never both: Validate
	// refuses the definition, so this is only ever filled by parsing one that
	// is about to be refused (it is not written back).
	Others []string
}

func (c Configured) Kind() string { return c.Base }

// SingleOwner is the answer of the kind the source stands for.
func (c Configured) SingleOwner() bool {
	if src, ok := baseSources[c.Base]; ok {
		return src.SingleOwner()
	}
	return Custom{}.SingleOwner()
}

func (Configured) isSource() {}

var baseSources = map[string]Source{
	KindHTTP: HTTP{}, KindSFTP: Sftp{}, KindS3: S3{}, KindSQS: Sqs{}, KindRabbitMQ: RabbitMQ{},
	KindDatabasePoll: DatabasePoll{}, KindAPIPoll: APIPoll{}, KindCustom: Custom{},
}

func (c Configured) MarshalJSON() ([]byte, error) {
	body := struct {
		Type   string    `json:"type,omitempty"`
		Config ConfigRef `json:"config"`
	}{Type: c.Type, Config: c.Config}
	return marshalTagged(c.Base, body)
}

// decodeConfigured reads a source object that carries "config": its kind, its
// type (an extension's) and the reference; any other field is noted in Others
// for Validate to refuse.
func decodeConfigured(fields map[string]json.RawMessage) (Source, error) {
	var kind string
	if err := json.Unmarshal(fields["kind"], &kind); err != nil || kind == "" {
		return nil, errors.New("source needs a kind")
	}
	if _, ok := baseSources[kind]; !ok {
		return nil, fmt.Errorf("source kind %q is not known", kind)
	}
	c := Configured{Base: kind}
	if err := json.Unmarshal(fields["config"], &c.Config); err != nil {
		return nil, fmt.Errorf("source %s: %w", kind, err)
	}
	for name, raw := range fields {
		switch {
		case name == "kind" || name == "config":
		case name == "type" && kind == KindCustom:
			if err := json.Unmarshal(raw, &c.Type); err != nil {
				return nil, fmt.Errorf("source custom: type must be text")
			}
		default:
			c.Others = append(c.Others, name)
		}
	}
	slices.Sort(c.Others)
	return c, nil
}

// ConfigOf reports whether the definition's source names a config entry, and
// which.
func ConfigOf(d Definition) (Configured, bool) {
	c, ok := d.Source.(Configured)
	return c, ok
}

// ConfigKeyOf is the key of the entry the definition names for the ingestion
// with this code, "" when its settings are inline.
func ConfigKeyOf(d Definition, code string) string {
	if c, ok := d.Source.(Configured); ok {
		return c.Config.KeyFor(code)
	}
	return ""
}

// configured checks the shape of a reference: the key follows the rules of a
// code, an extension's type those of a custom source's, and nothing else is
// given beside config.
func (v *checker) configured(c Configured) {
	if len(c.Others) > 0 {
		v.add("source.config", "INVALID", "a source holds its settings inline or names a config entry, never both: remove "+
			strings.Join(c.Others, ", ")+", or remove config")
	}
	if !c.Config.Own {
		v.matches("source.config", c.Config.Key, codePattern,
			"must be true (the ingestion's own code) or a key: lowercase letters, digits and '-', not at either end, at most 64")
	}
	if c.Base == KindCustom {
		if !customType.MatchString(c.Type) {
			v.add("source.type", "INVALID", "must be lowercase words joined by '-', such as camel-route")
		} else {
			v.require("source.type", !IsBuiltInKind(c.Type), "is a built-in kind: write it with that kind")
		}
	}
}
