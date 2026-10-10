package schemacodegen

// Schema is a parsed JSON Schema document (port of types.ts: Record<string, unknown>).
// It is a Value so that property order is preserved; any JSON value is accepted
// because the TS generators accept whatever JSON.parse returned.
type Schema = Value

// SupportedLanguage names a code generation target (port of types.ts).
type SupportedLanguage string

// The supported generation targets.
const (
	LangTypeScript SupportedLanguage = "typescript"
	LangPHP        SupportedLanguage = "php"
	LangPython     SupportedLanguage = "python"
	LangJava       SupportedLanguage = "java"
)

// SupportedLanguages lists every target in the order types.ts declares them.
var SupportedLanguages = []SupportedLanguage{LangTypeScript, LangPHP, LangPython, LangJava}

// GenerateCode generates code for a language from a schema (port of
// index.ts generateCode). An unknown language yields an error.
func GenerateCode(schema Schema, eventCode string, lang SupportedLanguage) (string, error) {
	switch lang {
	case LangTypeScript:
		return TypeScript(schema, eventCode)
	case LangPHP:
		return PHP(schema, eventCode)
	case LangPython:
		return Python(schema, eventCode)
	case LangJava:
		return Java(schema, eventCode)
	}
	return "", jsError{"unsupported language " + string(lang)}
}
