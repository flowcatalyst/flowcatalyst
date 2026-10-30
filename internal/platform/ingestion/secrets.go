package ingestion

import (
	"context"
	"slices"
)

// SecretNamesOf is the secrets a source needs set: SecretNames, except that
// for an extension kind a live runtime reports, only the settings its schema
// types SECRET name one (MiX's clientSecret and password, not its URLs or its
// username). Use it to say which secrets an ingestion needs and whether they
// are set. The authorship and fetch checks keep SecretNames, which can only
// count too many, and the cautious count stands while no runtime reports the
// kind.
func (r *Repository) SecretNamesOf(ctx context.Context, src Source) ([]string, error) {
	c, ok := src.(Custom)
	if !ok {
		return SecretNames(src), nil
	}
	schema, err := r.SchemaOf(ctx, c.Type)
	if err != nil {
		return nil, err
	}
	if !schema.Checked {
		return SecretNames(src), nil
	}
	var names []string
	for _, slot := range CustomSecretSlots(schema.Fields, c.Settings) {
		if slot.Name != "" {
			names = append(names, slot.Name)
		}
	}
	return names, nil
}

// SecretNames lists the secrets a source names, by walking the typed model:
// the credentials of each built-in kind, the auth of an http or api-poll
// source, and for a runtime-supplied kind the text settings (the platform
// cannot tell which of an extension's settings are secret names, and a name
// that is no secret reveals nothing). The runtime reads only these.
func SecretNames(src Source) []string {
	var names []string
	if c, ok := src.(Custom); ok {
		for _, value := range c.Settings {
			switch x := value.(type) {
			case string:
				names = append(names, x)
			case []any:
				for _, item := range x {
					if text, ok := item.(string); ok {
						names = append(names, text)
					}
				}
			case []string:
				names = append(names, x...)
			}
		}
	} else {
		for _, slot := range SecretSlots(src) {
			names = append(names, slot.Name)
		}
	}
	return slices.DeleteFunc(names, func(n string) bool { return n == "" })
}

// SecretsSection is the platform config section that holds the values of the
// secrets an ingestion binds by name: the application's, at the ingestion's
// client, as SECRET values, set on the platform configuration page.
const SecretsSection = "secrets"
