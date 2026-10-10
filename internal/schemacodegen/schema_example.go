package schemacodegen

// Example walks a JSON Schema and produces a sample payload (port of
// schema-example.ts generateExample). It returns an error exactly where the
// TS would throw (e.g. a primitive or null schema).
func Example(schema Schema) (Value, error) {
	return guard(func() Value { return generateExample(schema) })
}

func generateExample(schema Value) Value {
	// Literal / const
	if has(schema, "const") {
		return get(schema, "const")
	}

	// anyOf (TypeBox unions) — pick first non-null, fallback to the first variant
	if v := get(schema, "anyOf"); isArray(v) {
		return generateExample(firstNonNull(list(v)))
	}

	// oneOf — same approach
	if v := get(schema, "oneOf"); isArray(v) {
		return generateExample(firstNonNull(list(v)))
	}

	typ := get(schema, "type")

	if typ.isString("object") {
		properties := get(schema, "properties")
		if truthy(properties) {
			result := newObject()
			for _, e := range entries(properties) {
				result.set(e.key, generateExample(e.val))
			}
			return objValue(result)
		}

		// Record<string, T> — patternProperties or additionalProperties
		additional := get(schema, "additionalProperties")
		if truthy(additional) && isObjectish(additional) {
			return singleKey(generateExample(additional))
		}

		pattern := get(schema, "patternProperties")
		if truthy(pattern) {
			if vs := values(pattern); len(vs) > 0 && truthy(vs[0]) {
				return singleKey(generateExample(vs[0]))
			}
		}

		return objValue(newObject())
	}

	if typ.isString("array") {
		if items := get(schema, "items"); truthy(items) {
			return arrValue([]Value{generateExample(items)})
		}
		return arrValue([]Value{})
	}

	switch {
	case typ.isString("string"):
		return strValue("string")
	case typ.isString("integer"), typ.isString("number"):
		return numValue(0)
	case typ.isString("boolean"):
		return boolValue(true)
	case typ.isString("null"):
		return nullValue()
	}

	// Unknown / empty schema
	return objValue(newObject())
}

// firstNonNull is `variants.find(v => v.type !== "null") ?? variants[0]`;
// like the TS, an empty list yields undefined, which generateExample rejects.
func firstNonNull(variants []Value) Value {
	for _, v := range variants {
		if !get(v, "type").isString("null") {
			return v
		}
	}
	if len(variants) == 0 {
		return undefinedValue
	}
	return variants[0]
}

func singleKey(v Value) Value {
	o := newObject()
	o.set("key", v)
	return objValue(o)
}
