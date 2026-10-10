package schemacodegen

import (
	"slices"
	"strings"
)

// Java generates a Java 17+ record from a JSON Schema (port of
// schema-to-java.ts generateJavaRecord). It returns an error exactly where
// the TS would throw.
func Java(schema Schema, eventCode string) (string, error) {
	return guard(func() string { return generateJavaRecord(schema, eventCode) })
}

func generateJavaRecord(schema Value, eventCode string) string {
	packageName, className := eventCodeToJavaNames(eventCode)
	required := requiredSet(schema)
	properties := get(schema, "properties")

	if !truthy(properties) {
		return javaBuildRecord(packageName, className, nil, map[string]bool{}, nil, nil)
	}

	var components, fromMapLines, toMapLines []string
	imports := map[string]bool{}

	for _, e := range entries(properties) {
		key := e.key
		nullable := !required[key]
		typeInfo := resolveJavaType(e.val, nullable)

		for _, imp := range typeInfo.imports {
			imports[imp] = true
		}

		components = append(components, "    "+typeInfo.typ+" "+key)
		fromMapLines = append(fromMapLines, "            ("+typeInfo.cast+`) map.get("`+key+`")`)
		toMapLines = append(toMapLines, `        map.put("`+key+`", `+key+");")
	}

	return javaBuildRecord(packageName, className, components, imports, fromMapLines, toMapLines)
}

type javaTypeInfo struct {
	typ     string
	cast    string
	imports []string
}

func javaPlain(t string) javaTypeInfo { return javaTypeInfo{typ: t, cast: t} }

func javaPrim(prim, boxed string, nullable bool) javaTypeInfo {
	if nullable {
		return javaTypeInfo{typ: boxed, cast: boxed}
	}
	return javaTypeInfo{typ: prim, cast: boxed}
}

func resolveJavaType(schema Value, nullable bool) javaTypeInfo {
	// anyOf — handle nullable and literal unions
	if v := get(schema, "anyOf"); isArray(v) {
		return resolveAnyOfJava(list(v), nullable)
	}

	if v := get(schema, "oneOf"); isArray(v) {
		return resolveAnyOfJava(list(v), nullable)
	}

	// const / literal
	if has(schema, "const") {
		return javaPlain("String")
	}

	typ := get(schema, "type")

	// Handle JSON Schema type arrays like ["string", "null"]
	if isArray(typ) {
		nonNull, hasNull := splitNull(list(typ))
		if len(nonNull) == 1 {
			return resolveJavaType(withType(schema, nonNull[0]), hasNull || nullable)
		}
		// Multiple non-null types — fall back to Object
		return javaPlain("Object")
	}

	switch {
	case typ.isString("string"):
		return javaPlain("String")
	case typ.isString("integer"):
		return javaPrim("int", "Integer", nullable)
	case typ.isString("number"):
		return javaPrim("double", "Double", nullable)
	case typ.isString("boolean"):
		return javaPrim("boolean", "Boolean", nullable)
	case typ.isString("null"):
		return javaPlain("Void")
	}

	if typ.isString("array") {
		return resolveArrayJavaType(schema)
	}

	if typ.isString("object") {
		return resolveObjectJavaType(schema)
	}

	return javaPlain("Object")
}

func resolveArrayJavaType(schema Value) javaTypeInfo {
	items := get(schema, "items")
	imports := []string{"java.util.List"}

	if !truthy(items) {
		return javaTypeInfo{typ: "List<Object>", cast: "List<Object>", imports: imports}
	}

	inner := resolveJavaType(items, false)
	boxed := boxType(inner.typ)
	return javaTypeInfo{
		typ:     "List<" + boxed + ">",
		cast:    "List<" + boxed + ">",
		imports: append(imports, inner.imports...),
	}
}

func javaMapOf(inner javaTypeInfo, imports []string) javaTypeInfo {
	boxed := boxType(inner.typ)
	t := "Map<String, " + boxed + ">"
	return javaTypeInfo{typ: t, cast: t, imports: append(imports, inner.imports...)}
}

func resolveObjectJavaType(schema Value) javaTypeInfo {
	properties := get(schema, "properties")
	imports := []string{"java.util.Map"}

	if truthy(properties) {
		// Nested object → Map<String, Object>
		return javaTypeInfo{typ: "Map<String, Object>", cast: "Map<String, Object>", imports: imports}
	}

	// Record via additionalProperties
	if additional := get(schema, "additionalProperties"); truthy(additional) && isObjectish(additional) {
		return javaMapOf(resolveJavaType(additional, false), imports)
	}

	// Record via patternProperties
	if pattern := get(schema, "patternProperties"); truthy(pattern) {
		if vs := values(pattern); len(vs) > 0 && truthy(vs[0]) {
			return javaMapOf(resolveJavaType(vs[0], false), imports)
		}
	}

	return javaTypeInfo{typ: "Map<String, Object>", cast: "Map<String, Object>", imports: imports}
}

func resolveAnyOfJava(variants []Value, nullable bool) javaTypeInfo {
	nonNull, hasNull, allConst := phpSplitVariants(variants)

	// All-const → String
	if allConst {
		return javaPlain("String")
	}

	// Single non-null type → that type (boxed if nullable)
	if len(nonNull) == 1 {
		inner := resolveJavaType(nonNull[0], hasNull || nullable)
		if hasNull || nullable {
			inner.typ = boxType(inner.typ)
		}
		return inner
	}

	return javaPlain("Object")
}

func boxType(t string) string {
	switch t {
	case "int":
		return "Integer"
	case "double":
		return "Double"
	case "boolean":
		return "Boolean"
	}
	return t
}

func javaBuildRecord(packageName, className string, components []string, imports map[string]bool, fromMapLines, toMapLines []string) string {
	lines := []string{"package " + packageName + ";"}

	// Collect imports — always need Map for fromMap/toMap
	all := map[string]bool{"java.util.Map": true, "java.util.LinkedHashMap": true}
	for imp := range imports {
		all[imp] = true
	}
	sorted := make([]string, 0, len(all))
	for imp := range all {
		sorted = append(sorted, imp)
	}
	slices.Sort(sorted) // JS default sort is by UTF-16 code unit; identical for ASCII names
	lines = append(lines, "")
	for _, imp := range sorted {
		lines = append(lines, "import "+imp+";")
	}

	lines = append(lines, "", "public record "+className+"(")
	lines = appendCommaSeparated(lines, components)
	lines = append(lines, ") {")

	// fromMap factory
	lines = append(lines, "", `    @SuppressWarnings("unchecked")`,
		"    public static "+className+" fromMap(Map<String, Object> map) {",
		"        return new "+className+"(")
	lines = appendCommaSeparated(lines, fromMapLines)
	lines = append(lines, "        );", "    }")

	// toMap method
	lines = append(lines, "", "    public Map<String, Object> toMap() {",
		"        var map = new LinkedHashMap<String, Object>();")
	lines = append(lines, toMapLines...)
	lines = append(lines, "        return map;", "    }")

	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
}

// appendCommaSeparated appends each line with a trailing comma on all but the last.
func appendCommaSeparated(lines, items []string) []string {
	for i, it := range items {
		if i < len(items)-1 {
			it += ","
		}
		lines = append(lines, it)
	}
	return lines
}

func eventCodeToJavaNames(eventCode string) (packageName, className string) {
	segments := strings.Split(eventCode, ":")
	seg := func(i int, fallback string) string {
		if i < len(segments) {
			return segments[i]
		}
		return fallback
	}
	app, subdomain, aggregate := seg(0, "app"), seg(1, "domain"), seg(2, "aggregate")
	packageName = "com." + jsLower(app) + ".events." + jsLower(subdomain) + "." + jsLower(aggregate)
	className = pascalCase(seg(2, "Unknown")) + pascalCase(seg(3, "Event")) + "Data"
	return packageName, className
}
