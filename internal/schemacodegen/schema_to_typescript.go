package schemacodegen

import (
	"regexp"
	"strings"
)

// TypeScript generates a TypeScript `export interface` from a JSON Schema
// (port of schema-to-typescript.ts generateTypeScriptInterface). It returns
// an error exactly where the TS would throw.
func TypeScript(schema Schema, eventCode string) (string, error) {
	return guard(func() string { return generateTypeScriptInterface(schema, eventCode) })
}

func generateTypeScriptInterface(schema Value, eventCode string) string {
	name := eventCodeToInterfaceName(eventCode)
	required := requiredSet(schema)
	properties := get(schema, "properties")

	if !truthy(properties) {
		return "export interface " + name + " {\n  [key: string]: unknown;\n}\n"
	}

	lines := []string{"export interface " + name + " {"}

	for _, e := range entries(properties) {
		optional := !required[e.key]
		typ := tsResolveType(e.val, "  ")
		lines = append(lines, "  "+e.key+optMark(optional)+": "+typ+";")
	}

	lines = append(lines, "}\n")
	return strings.Join(lines, "\n")
}

func optMark(optional bool) string {
	if optional {
		return "?"
	}
	return ""
}

func tsResolveType(schema Value, indent string) string {
	// const / literal
	if has(schema, "const") {
		return Stringify(get(schema, "const"), "")
	}

	// anyOf — union type
	if v := get(schema, "anyOf"); isArray(v) {
		return tsResolveAnyOf(list(v), indent)
	}

	// oneOf — same treatment
	if v := get(schema, "oneOf"); isArray(v) {
		return tsResolveAnyOf(list(v), indent)
	}

	typ := get(schema, "type")

	// Handle JSON Schema type arrays like ["string", "null"] → "string | null"
	if isArray(typ) {
		var types []string
		for _, t := range list(typ) {
			switch {
			case t.isString("string"):
				types = append(types, "string")
			case t.isString("integer"), t.isString("number"):
				types = append(types, "number")
			case t.isString("boolean"):
				types = append(types, "boolean")
			case t.isString("null"):
				types = append(types, "null")
			case t.isString("object"):
				types = append(types, tsResolveObjectType(schema, indent))
			case t.isString("array"):
				types = append(types, "unknown[]")
			default:
				types = append(types, "unknown")
			}
		}
		return strings.Join(uniq(types), " | ")
	}

	switch {
	case typ.isString("string"):
		return "string"
	case typ.isString("integer"), typ.isString("number"):
		return "number"
	case typ.isString("boolean"):
		return "boolean"
	case typ.isString("null"):
		return "null"
	}

	if typ.isString("array") {
		items := get(schema, "items")
		if !truthy(items) {
			return "unknown[]"
		}
		inner := tsResolveType(items, indent)
		// Wrap union types in parens for array
		if strings.Contains(inner, "|") {
			return "(" + inner + ")[]"
		}
		return inner + "[]"
	}

	if typ.isString("object") {
		return tsResolveObjectType(schema, indent)
	}

	return "unknown"
}

func tsResolveObjectType(schema Value, indent string) string {
	properties := get(schema, "properties")

	if truthy(properties) {
		required := requiredSet(schema)
		innerIndent := indent + "  "
		lines := []string{"{"}

		for _, e := range entries(properties) {
			optional := !required[e.key]
			typ := tsResolveType(e.val, innerIndent)
			lines = append(lines, innerIndent+e.key+optMark(optional)+": "+typ+";")
		}

		lines = append(lines, indent+"}")
		return strings.Join(lines, "\n")
	}

	// Record<string, T> via additionalProperties
	additional := get(schema, "additionalProperties")
	if truthy(additional) && isObjectish(additional) {
		return "Record<string, " + tsResolveType(additional, indent) + ">"
	}

	// Record<string, T> via patternProperties
	if pattern := get(schema, "patternProperties"); truthy(pattern) {
		if vs := values(pattern); len(vs) > 0 && truthy(vs[0]) {
			return "Record<string, " + tsResolveType(vs[0], indent) + ">"
		}
	}

	return "Record<string, unknown>"
}

// flattenAnyOf flattens one level of nested anyOf (shared by the generators).
func flattenAnyOf(variants []Value) []Value {
	var flat []Value
	for _, v := range variants {
		if nested := get(v, "anyOf"); isArray(nested) {
			flat = append(flat, list(nested)...)
		} else {
			flat = append(flat, v)
		}
	}
	return flat
}

func tsResolveAnyOf(variants []Value, indent string) string {
	flat := flattenAnyOf(variants)

	// All-const → string literal union
	allConst := true
	for _, v := range flat {
		if !has(v, "const") {
			allConst = false
			break
		}
	}
	if allConst {
		parts := make([]string, len(flat))
		for i, v := range flat {
			parts[i] = Stringify(get(v, "const"), "")
		}
		return strings.Join(parts, " | ")
	}

	types := make([]string, len(flat))
	for i, v := range flat {
		types[i] = tsResolveType(v, indent)
	}
	// Deduplicate
	return strings.Join(uniq(types), " | ")
}

var nameSplit = regexp.MustCompile(`[-_]`)

// eventCodeToInterfaceName is the last-two-segments PascalCase + "Data" name
// shared by the TypeScript and Python generators.
func eventCodeToInterfaceName(eventCode string) string {
	segments := strings.Split(eventCode, ":")
	if len(segments) > 2 {
		segments = segments[len(segments)-2:]
	}
	var sb strings.Builder
	for _, s := range segments {
		sb.WriteString(pascalCase(s))
	}
	return sb.String() + "Data"
}
