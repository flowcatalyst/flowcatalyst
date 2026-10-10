package schemacodegen

import "strings"

// PHP generates a PHP 8.1+ readonly DTO class from a JSON Schema (port of
// schema-to-php.ts generatePhpDto). It returns an error exactly where the TS
// would throw.
func PHP(schema Schema, eventCode string) (string, error) {
	return guard(func() string { return generatePhpDto(schema, eventCode) })
}

func generatePhpDto(schema Value, eventCode string) string {
	namespace, className := eventCodeToPhpNames(eventCode)
	required := requiredSet(schema)
	properties := get(schema, "properties")

	if !truthy(properties) {
		return phpBuildClass(namespace, className, nil, nil, nil)
	}

	var constructorLines, fromArrayLines, toArrayLines []string

	for _, e := range entries(properties) {
		key := e.key
		nullable := !required[key]
		typeInfo := resolvePhpType(e.val, nullable)

		// The TS collects PHPDoc blocks here but buildClass ignores them;
		// resolvePhpType is still evaluated above (it can throw).

		prefix := ""
		if nullable && typeInfo.typ != "mixed" {
			prefix = "?"
		}
		constructorLines = append(constructorLines,
			"        public "+prefix+typeInfo.typ+" $"+key+",")

		// TS builds per-type fromArray/toArray closures, but every one of
		// them is the same expression, so they are inlined.
		fromArrayLines = append(fromArrayLines, "            "+key+": $data['"+key+"'],")
		toArrayLines = append(toArrayLines, "            '"+key+"' => $this->"+key+",")
	}

	return phpBuildClass(namespace, className, constructorLines, fromArrayLines, toArrayLines)
}

// phpTypeInfo mirrors PhpTypeInfo minus the (constant) fromArray/toArray
// closures. hasDoc distinguishes phpDoc null from "".
type phpTypeInfo struct {
	typ    string
	phpDoc string
	hasDoc bool
}

func phpSimple(t string) phpTypeInfo { return phpTypeInfo{typ: t} }

func phpDocInfo(t, doc string) phpTypeInfo { return phpTypeInfo{typ: t, phpDoc: doc, hasDoc: true} }

func resolvePhpType(schema Value, nullable bool) phpTypeInfo {
	// anyOf — handle nullable and literal unions
	if v := get(schema, "anyOf"); isArray(v) {
		return resolveAnyOfPhp(list(v), nullable)
	}

	if v := get(schema, "oneOf"); isArray(v) {
		return resolveAnyOfPhp(list(v), nullable)
	}

	// const / literal
	if has(schema, "const") {
		return phpDocInfo("string", "'"+jsToString(get(schema, "const"))+"'")
	}

	typ := get(schema, "type")

	// Handle JSON Schema type arrays like ["string", "null"]
	if isArray(typ) {
		nonNull, hasNull := splitNull(list(typ))
		if len(nonNull) == 1 {
			return resolvePhpType(withType(schema, nonNull[0]), hasNull || nullable)
		}
		return phpSimple("mixed")
	}

	switch {
	case typ.isString("string"):
		return phpSimple("string")
	case typ.isString("integer"):
		return phpSimple("int")
	case typ.isString("number"):
		return phpSimple("float")
	case typ.isString("boolean"):
		return phpSimple("bool")
	case typ.isString("null"):
		return phpSimple("mixed")
	}

	if typ.isString("array") {
		doc, ok := resolveArrayPhpDoc(schema)
		return phpTypeInfo{typ: "array", phpDoc: doc, hasDoc: ok}
	}

	if typ.isString("object") {
		return resolveObjectPhpType(schema, nullable)
	}

	return phpSimple("mixed")
}

// splitNull is `[types.filter(t => t !== "null"), types.includes("null")]`.
func splitNull(types []Value) (nonNull []Value, hasNull bool) {
	for _, t := range types {
		if t.isString("null") {
			hasNull = true
		} else {
			nonNull = append(nonNull, t)
		}
	}
	return nonNull, hasNull
}

func resolveObjectPhpType(schema Value, nullable bool) phpTypeInfo {
	properties := get(schema, "properties")

	if truthy(properties) {
		// Nested object → array with shape doc
		required := requiredSet(schema)
		var shapeEntries []string
		for _, e := range entries(properties) {
			innerType := resolvePhpDocType(e.val, !required[e.key])
			shapeEntries = append(shapeEntries, e.key+": "+innerType)
		}
		shape := "array{" + strings.Join(shapeEntries, ", ") + "}"
		if nullable {
			return phpDocInfo("array", shape+"|null")
		}
		return phpDocInfo("array", shape)
	}

	// Record type
	if nullable {
		return phpDocInfo("array", "array<string, mixed>|null")
	}
	return phpDocInfo("array", "array<string, mixed>")
}

// phpSplitVariants flattens anyOf variants and splits off the null ones.
func phpSplitVariants(variants []Value) (nonNull []Value, hasNull, allConst bool) {
	flat := flattenAnyOf(variants)
	for _, v := range flat {
		if get(v, "type").isString("null") {
			hasNull = true
			break
		}
	}
	for _, v := range flat {
		if !get(v, "type").isString("null") {
			nonNull = append(nonNull, v)
		}
	}
	allConst = true
	for _, v := range nonNull {
		if !has(v, "const") {
			allConst = false
			break
		}
	}
	return nonNull, hasNull, allConst
}

func phpLiterals(nonNull []Value) string {
	parts := make([]string, len(nonNull))
	for i, v := range nonNull {
		parts[i] = "'" + jsToString(get(v, "const")) + "'"
	}
	return strings.Join(parts, "|")
}

func resolveAnyOfPhp(variants []Value, nullable bool) phpTypeInfo {
	nonNull, hasNull, allConst := phpSplitVariants(variants)

	// All-const → string with PHPDoc literal union
	if allConst {
		literals := phpLiterals(nonNull)
		if hasNull {
			return phpDocInfo("string", literals+"|null")
		}
		return phpDocInfo("string", literals)
	}

	// Single non-null type → that type, nullable
	if len(nonNull) == 1 {
		return resolvePhpType(nonNull[0], hasNull || nullable)
	}

	return phpSimple("mixed")
}

func resolvePhpDocType(schema Value, nullable bool) string {
	if v := get(schema, "anyOf"); isArray(v) {
		nonNull, hasNull, allConst := phpSplitVariants(list(v))

		if allConst {
			literals := phpLiterals(nonNull)
			if hasNull {
				return literals + "|null"
			}
			return literals
		}

		if len(nonNull) == 1 {
			return resolvePhpDocType(nonNull[0], hasNull || nullable)
		}

		return "mixed"
	}

	if has(schema, "const") {
		return "'" + jsToString(get(schema, "const")) + "'"
	}

	typ := get(schema, "type")
	suffix := ""
	if nullable {
		suffix = "|null"
	}

	// Handle JSON Schema type arrays like ["string", "null"]
	if isArray(typ) {
		nonNull, hasNull := splitNull(list(typ))
		if len(nonNull) == 1 {
			return resolvePhpDocType(withType(schema, nonNull[0]), hasNull || nullable)
		}
		return "mixed"
	}

	switch {
	case typ.isString("string"):
		return "string" + suffix
	case typ.isString("integer"):
		return "int" + suffix
	case typ.isString("number"):
		return "float" + suffix
	case typ.isString("boolean"):
		return "bool" + suffix
	case typ.isString("null"):
		return "null"
	case typ.isString("array"), typ.isString("object"):
		return "array" + suffix
	}
	return "mixed"
}

func resolveArrayPhpDoc(schema Value) (string, bool) {
	items := get(schema, "items")
	if !truthy(items) {
		return "", false
	}
	return resolvePhpDocType(items, false) + "[]", true
}

func phpBuildClass(namespace, className string, constructorLines, fromArrayLines, toArrayLines []string) string {
	lines := []string{
		"<?php",
		"",
		"declare(strict_types=1);",
		"",
		"namespace " + namespace + ";",
		"",
		"final readonly class " + className,
		"{",
	}

	// Constructor
	if len(constructorLines) > 0 {
		lines = append(lines, "    public function __construct(")
		lines = append(lines, constructorLines...)
		lines = append(lines, "    ) {}")
	} else {
		lines = append(lines, "    public function __construct() {}")
	}

	lines = append(lines, "")

	// fromArray
	lines = append(lines,
		"    /** @param array<string, mixed> $data */",
		"    public static function fromArray(array $data): self",
		"    {",
		"        return new self(",
	)
	lines = append(lines, fromArrayLines...)
	lines = append(lines, "        );", "    }")

	lines = append(lines, "")

	// toArray
	lines = append(lines,
		"    /** @return array<string, mixed> */",
		"    public function toArray(): array",
		"    {",
		"        return [",
	)
	lines = append(lines, toArrayLines...)
	lines = append(lines, "        ];", "    }")

	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
}

func eventCodeToPhpNames(eventCode string) (namespace, className string) {
	segments := strings.Split(eventCode, ":")
	// e.g. platform:iam:user:created → App=Platform, Subdomain=Iam, Aggregate=User, Event=Created
	seg := func(i int, fallback string) string {
		if i < len(segments) {
			return pascalCase(segments[i])
		}
		return fallback
	}
	app, subdomain, aggregate, event := seg(0, "App"), seg(1, "Domain"), seg(2, "Aggregate"), seg(3, "Event")
	namespace = `Flowcatalyst\Events\` + app + `\` + subdomain + `\` + aggregate
	className = seg(2, "Unknown") + event + "Data"
	return namespace, className
}
