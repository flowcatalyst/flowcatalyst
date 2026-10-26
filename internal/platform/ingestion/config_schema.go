package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Setting types of an extension kind's fields, as a runtime reports them
// (ingestor-contract SourceKind.Type).
const (
	SettingText      = "TEXT"
	SettingMultiline = "MULTILINE"
	SettingChoice    = "CHOICE"
	SettingInteger   = "INTEGER"
	SettingBoolean   = "BOOLEAN"
	SettingDuration  = "DURATION"
	SettingURL       = "URL"
	SettingSecret    = "SECRET"
	SettingList      = "LIST"
)

// SettingField is one setting of an extension kind, as the runtime reports it
// in RuntimeKind.Settings.
type SettingField struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	DefaultValue *string  `json:"defaultValue,omitempty"`
	Choices      []string `json:"choices,omitempty"`
	Pattern      *string  `json:"pattern,omitempty"`
	Min          *string  `json:"min,omitempty"`
	Max          *string  `json:"max,omitempty"`
	Rule         *string  `json:"rule,omitempty"`
}

// ParseSettingFields reads what a runtime reported as a kind's settings. ok is
// false when it reported none (or something that is not a field list): the
// platform then has no schema to check against.
func ParseSettingFields(raw json.RawMessage) (fields []SettingField, ok bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) == 0 {
		return nil, false
	}
	return fields, true
}

// CheckSettings checks a value's settings against a kind's fields, as the
// runtime does before it builds the route (ingestor-contract
// SourceKind.problems): a setting the kind does not have, a required one
// missing or blank, a value that is not what its field holds. A blank text is
// not given, since forms send what was left empty as "". Problems are at
// config.<name>.
func CheckSettings(fields []SettingField, settings map[string]any) Problems {
	var v checker
	names := make([]string, 0, len(settings))
	for name := range settings {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !slices.ContainsFunc(fields, func(f SettingField) bool { return f.Name == name }) {
			v.add("config."+name, "INVALID", "is not a setting of the kind")
		}
	}
	for _, f := range fields {
		value, given := settings[f.Name]
		if text, isText := value.(string); !given || value == nil || (isText && strings.TrimSpace(text) == "" && f.Type != SettingMultiline) {
			if f.Required && f.DefaultValue == nil {
				v.add("config."+f.Name, "INVALID", "is required")
			}
			continue
		}
		if problem := f.problem(value); problem != "" {
			v.add("config."+f.Name, "INVALID", problem)
		}
	}
	return v.problems
}

// problem is what is wrong with a value for the field, "" for nothing.
func (f SettingField) problem(value any) string {
	text, isText := value.(string)
	switch f.Type {
	case SettingText:
		switch {
		case !isText:
			return "must be text"
		case !f.matches(text):
			return f.ruleOr("must match " + deref(f.Pattern))
		}
	case SettingMultiline:
		if !isText {
			return "must be text"
		}
	case SettingInteger:
		return f.integer(value)
	case SettingBoolean:
		if _, ok := value.(bool); !ok {
			return "must be true or false"
		}
	case SettingDuration:
		d, err := ParseDuration(text)
		if !isText || err != nil {
			return "must be a duration such as PT30S"
		}
		return f.durationBounds(d)
	case SettingURL:
		u, err := url.Parse(text)
		if !isText || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "must be an http or https address"
		}
	case SettingSecret:
		if !isText || strings.TrimSpace(text) == "" {
			return "must be a secret's name, such as api-password"
		}
	case SettingChoice:
		if !isText || !slices.Contains(f.Choices, text) {
			return "must be one of " + strings.Join(f.Choices, ", ")
		}
	case SettingList:
		return f.list(value)
	}
	return ""
}

func (f SettingField) integer(value any) string {
	const whole = "must be a whole number"
	var n int64
	switch x := value.(type) {
	case float64:
		if x != float64(int64(x)) || x > 1e18 || x < -1e18 {
			return whole
		}
		n = int64(x)
	case string:
		if !regexp.MustCompile(`^-?[0-9]{1,18}$`).MatchString(x) {
			return whole
		}
		n, _ = strconv.ParseInt(x, 10, 64)
	default:
		return whole
	}
	if low, ok := parseBound(f.Min); ok && n < low {
		return f.ruleOr(rangeText(f.Min, f.Max))
	}
	if high, ok := parseBound(f.Max); ok && n > high {
		return f.ruleOr(rangeText(f.Min, f.Max))
	}
	return ""
}

func (f SettingField) durationBounds(d time.Duration) string {
	if f.Min != nil {
		if low, err := ParseDuration(*f.Min); err == nil && d < low {
			return f.ruleOr(rangeText(f.Min, f.Max))
		}
	}
	if f.Max != nil {
		if high, err := ParseDuration(*f.Max); err == nil && d > high {
			return f.ruleOr(rangeText(f.Min, f.Max))
		}
	}
	return ""
}

func (f SettingField) list(value any) string {
	items, ok := value.([]any)
	if !ok {
		if strs, isStrs := value.([]string); isStrs {
			items = make([]any, len(strs))
			for i, s := range strs {
				items[i] = s
			}
		} else {
			return "must be a list of text"
		}
	}
	if low, ok := parseBound(f.Min); ok && int64(len(items)) < low {
		return "must hold " + strings.TrimPrefix(rangeText(f.Min, f.Max), "must be ") + " items"
	}
	if high, ok := parseBound(f.Max); ok && int64(len(items)) > high {
		return "must hold " + strings.TrimPrefix(rangeText(f.Min, f.Max), "must be ") + " items"
	}
	for i, item := range items {
		text, isText := item.(string)
		if !isText {
			return fmt.Sprintf("item %d (%v) %s", i+1, item, f.ruleOr("must be text"))
		}
		if !f.matches(text) {
			return fmt.Sprintf("item %d (%s) %s", i+1, text, f.ruleOr("must match "+deref(f.Pattern)))
		}
	}
	return ""
}

// matches reports whether the whole text matches the field's pattern. A
// pattern this side cannot compile (Java and Go differ at the edges) is not
// checked here: the runtime still checks it.
func (f SettingField) matches(text string) bool {
	if f.Pattern == nil {
		return true
	}
	re, err := regexp.Compile("^(?:" + *f.Pattern + ")$")
	return err != nil || re.MatchString(text)
}

func (f SettingField) ruleOr(plain string) string {
	if f.Rule != nil && *f.Rule != "" {
		return *f.Rule
	}
	return plain
}

func parseBound(s *string) (int64, bool) {
	if s == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(*s, 10, 64)
	return n, err == nil
}

func rangeText(low, high *string) string {
	switch {
	case low != nil && high != nil:
		return "must be " + *low + " to " + *high
	case low != nil:
		return "must be at least " + *low
	default:
		return "must be at most " + deref(high)
	}
}

// KindSchema is what the live runtimes say of an extension kind's settings.
type KindSchema struct {
	// Fields are the kind's settings; empty when Checked is false.
	Fields []SettingField
	// Checked is true when a live runtime reports the kind with a field list.
	// False means the platform cannot check a value's settings: it stores the
	// value with a SCHEMA_UNCHECKED warning, and the runtime checks it before
	// it builds the route.
	Checked bool
}

// SchemaOf is the schema of the extension kind (a custom source's type) the
// live runtimes report. A built-in kind needs none: its definition, strictly
// decoded and validated, is its schema.
func (r *Repository) SchemaOf(ctx context.Context, kind string) (KindSchema, error) {
	reported, err := r.LiveKinds(ctx)
	if err != nil {
		return KindSchema{}, err
	}
	for _, k := range reported {
		if k.Kind != kind {
			continue
		}
		if fields, ok := ParseSettingFields(k.Settings); ok {
			return KindSchema{Fields: fields, Checked: true}, nil
		}
	}
	return KindSchema{}, nil
}
