package schemacodegen

// Viewer text (port of the computed properties in
// frontend/src/pages/event-types/SchemaViewerDialog.vue): what each tab of
// the schema viewer displays for a stored schema string, including the
// fallback strings. The *Text functions take the raw stored schema string.
//
// The returned text is scrubbed of lone surrogates (replaced by U+FFFD, as
// the browser would render them) so it is always valid UTF-8.

// parsedViewerSchema is the viewer's `parsedSchema` computed: the parsed
// schema, or ok=false when the string is empty, unparseable or parses to a
// falsy value (null, false, 0, "") — all of which the viewer treats as "no
// schema".
func parsedViewerSchema(src string) (Value, bool) {
	if src == "" {
		return Value{}, false
	}
	v, err := Parse(src)
	if err != nil || !truthy(v) {
		return Value{}, false
	}
	return v, true
}

// FormatSchema is the Schema tab: the schema pretty-printed with two-space
// indent (JSON.stringify(parsed, null, 2)), or the raw string when it does
// not parse (or parses to a falsy value, as in the SPA).
func FormatSchema(src string) string {
	v, ok := parsedViewerSchema(src)
	if !ok {
		return scrub(src)
	}
	return scrub(Stringify(v, "  "))
}

// PrettyJSON is the SPA's formatJson: JSON.stringify(JSON.parse(src), null,
// 2), key order kept and < > & left as they are; ok is false when src is not
// JSON.
func PrettyJSON(src string) (string, bool) {
	v, err := Parse(src)
	if err != nil {
		return "", false
	}
	return scrub(Stringify(v, "  ")), true
}

// ExampleJSON is the Example tab: a generated sample payload as indented
// JSON, or "// Unable to generate example".
func ExampleJSON(src string) string {
	const fallback = "// Unable to generate example"
	v, ok := parsedViewerSchema(src)
	if !ok {
		return fallback
	}
	ex, err := Example(v)
	if err != nil {
		return fallback
	}
	return scrub(Stringify(ex, "  "))
}

func viewerCode(src, eventCode, noSchema, failed string, gen func(Schema, string) (string, error)) string {
	v, ok := parsedViewerSchema(src)
	if !ok {
		return noSchema
	}
	out, err := gen(v, eventCode)
	if err != nil {
		return failed
	}
	return scrub(out)
}

// TypeScriptText is the TypeScript tab.
func TypeScriptText(src, eventCode string) string {
	return viewerCode(src, eventCode, "// No schema available", "// Unable to generate TypeScript interface", TypeScript)
}

// PHPText is the PHP tab.
func PHPText(src, eventCode string) string {
	return viewerCode(src, eventCode, "// No schema available", "// Unable to generate PHP DTO", PHP)
}

// PythonText is the Python tab.
func PythonText(src, eventCode string) string {
	return viewerCode(src, eventCode, "# No schema available", "# Unable to generate Python dataclass", Python)
}

// JavaText is the Java tab.
func JavaText(src, eventCode string) string {
	return viewerCode(src, eventCode, "// No schema available", "// Unable to generate Java record", Java)
}
