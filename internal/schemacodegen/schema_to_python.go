package schemacodegen

import (
	"regexp"
	"slices"
	"strings"
)

// Python generates a Python 3.10+ frozen dataclass from a JSON Schema (port
// of schema-to-python.ts generatePythonDataclass). It returns an error
// exactly where the TS would throw.
func Python(schema Schema, eventCode string) (string, error) {
	return guard(func() string { return generatePythonDataclass(schema, eventCode) })
}

func generatePythonDataclass(schema Value, eventCode string) string {
	className := eventCodeToInterfaceName(eventCode)
	required := requiredSet(schema)
	properties := get(schema, "properties")

	if !truthy(properties) {
		return pyBuildClass(className, nil, nil, nil)
	}

	var fields, fromDictLines, toDictLines []string

	// Required fields first, then optional
	var requiredEntries, optionalEntries []entry
	for _, e := range entries(properties) {
		if required[e.key] {
			requiredEntries = append(requiredEntries, e)
		} else {
			optionalEntries = append(optionalEntries, e)
		}
	}

	for _, e := range append(requiredEntries, optionalEntries...) {
		key := e.key
		isRequired := required[key]
		snakeKey := toSnakeCase(key)
		pyType := resolvePythonType(e.val, !isRequired)

		if isRequired {
			fields = append(fields, "    "+snakeKey+": "+pyType)
		} else {
			fields = append(fields, "    "+snakeKey+": "+pyType+" = None")
		}

		fromDictLines = append(fromDictLines, "            "+snakeKey+`=data.get("`+key+`"),`)
		toDictLines = append(toDictLines, `            "`+key+`": self.`+snakeKey+",")
	}

	return pyBuildClass(className, fields, fromDictLines, toDictLines)
}

func orNone(base string, nullable bool) string {
	if nullable {
		return base + " | None"
	}
	return base
}

func resolvePythonType(schema Value, nullable bool) string {
	// const / literal
	if has(schema, "const") {
		return "str"
	}

	// anyOf — union type
	if v := get(schema, "anyOf"); isArray(v) {
		return resolveAnyOfPython(list(v), nullable)
	}

	if v := get(schema, "oneOf"); isArray(v) {
		return resolveAnyOfPython(list(v), nullable)
	}

	typ := get(schema, "type")

	// Handle JSON Schema type arrays like ["string", "null"] → "str | None"
	if isArray(typ) {
		nonNull, hasNull := splitNull(list(typ))
		if len(nonNull) == 1 {
			return resolvePythonType(withType(schema, nonNull[0]), hasNull || nullable)
		}
		types := make([]string, len(nonNull))
		for i, t := range nonNull {
			switch {
			case t.isString("string"):
				types[i] = "str"
			case t.isString("integer"):
				types[i] = "int"
			case t.isString("number"):
				types[i] = "float"
			case t.isString("boolean"):
				types[i] = "bool"
			default:
				types[i] = "Any"
			}
		}
		return orNone(strings.Join(uniq(types), " | "), hasNull || nullable)
	}

	switch {
	case typ.isString("string"):
		return orNone("str", nullable)
	case typ.isString("integer"):
		return orNone("int", nullable)
	case typ.isString("number"):
		return orNone("float", nullable)
	case typ.isString("boolean"):
		return orNone("bool", nullable)
	case typ.isString("null"):
		return "None"
	}

	if typ.isString("array") {
		items := get(schema, "items")
		if !truthy(items) {
			return "list" + orNone("", nullable)
		}
		inner := resolvePythonType(items, false)
		return orNone("list["+inner+"]", nullable)
	}

	if typ.isString("object") {
		return resolveObjectPythonType(schema, nullable)
	}

	return "Any"
}

func resolveObjectPythonType(schema Value, nullable bool) string {
	// Nested object → dict
	if truthy(get(schema, "properties")) {
		return orNone("dict[str, Any]", nullable)
	}

	// Record type via additionalProperties
	if additional := get(schema, "additionalProperties"); truthy(additional) && isObjectish(additional) {
		return orNone("dict[str, "+resolvePythonType(additional, false)+"]", nullable)
	}

	// Record type via patternProperties
	if pattern := get(schema, "patternProperties"); truthy(pattern) {
		if vs := values(pattern); len(vs) > 0 && truthy(vs[0]) {
			return orNone("dict[str, "+resolvePythonType(vs[0], false)+"]", nullable)
		}
	}

	return orNone("dict[str, Any]", nullable)
}

func resolveAnyOfPython(variants []Value, nullable bool) string {
	nonNull, hasNull, allConst := phpSplitVariants(variants)

	// All-const → Literal union
	if allConst {
		parts := make([]string, len(nonNull))
		for i, v := range nonNull {
			parts[i] = Stringify(get(v, "const"), "")
		}
		return orNone("Literal["+strings.Join(parts, ", ")+"]", hasNull || nullable)
	}

	// Single non-null type
	if len(nonNull) == 1 {
		return resolvePythonType(nonNull[0], hasNull || nullable)
	}

	// Multiple non-null types
	types := make([]string, len(nonNull))
	for i, v := range nonNull {
		types[i] = resolvePythonType(v, false)
	}
	return orNone(strings.Join(uniq(types), " | "), hasNull || nullable)
}

func pyBuildClass(className string, fields, fromDictLines, toDictLines []string) string {
	imports := []string{"from __future__ import annotations"}
	needsAny, needsLiteral := false, false
	for _, f := range fields {
		needsAny = needsAny || strings.Contains(f, "Any")
		needsLiteral = needsLiteral || strings.Contains(f, "Literal[")
	}

	var typingImports []string
	if needsAny {
		typingImports = append(typingImports, "Any")
	}
	if needsLiteral {
		typingImports = append(typingImports, "Literal")
	}
	if len(typingImports) > 0 {
		imports = append(imports, "from typing import "+strings.Join(typingImports, ", "))
	}

	imports = append(imports, "from dataclasses import dataclass")

	lines := append(slices.Clone(imports),
		"",
		"",
		"@dataclass(frozen=True)",
		"class "+className+":",
	)

	if len(fields) == 0 {
		lines = append(lines, "    pass")
	} else {
		lines = append(lines, fields...)
	}

	lines = append(lines, "")

	// from_dict
	lines = append(lines,
		"    @classmethod",
		"    def from_dict(cls, data: dict[str, Any]) -> "+className+":",
		"        return cls(",
	)
	lines = append(lines, fromDictLines...)
	lines = append(lines, "        )")

	lines = append(lines, "")

	// to_dict
	lines = append(lines,
		"    def to_dict(self) -> dict[str, Any]:",
		"        return {",
	)
	lines = append(lines, toDictLines...)
	lines = append(lines, "        }")

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

var upperAZ = regexp.MustCompile(`([A-Z])`)

// toSnakeCase is `s.replace(/([A-Z])/g, "_$1").toLowerCase().replace(/^_/, "")`.
func toSnakeCase(s string) string {
	out := jsLower(upperAZ.ReplaceAllString(s, "_$1"))
	return strings.TrimPrefix(out, "_")
}
