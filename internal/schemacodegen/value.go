// Package schemacodegen is a pure-Go port of the SPA's JSON Schema code
// generators (frontend/packages/schema-codegen), its syntax highlighter
// (frontend/src/utils/schema-highlight.ts) and its schema zip export
// (frontend/src/utils/schema-export.ts), so the server-rendered UI can show
// the event-type schema viewer without JavaScript.
//
// Output is byte-identical to the TypeScript originals for the same input.
// To get there the package carries a small JavaScript-flavoured JSON value
// model (this file): object key order is preserved the way JSON.parse
// preserves it, numbers are printed the way JS prints them, and the helpers
// the generators lean on (truthiness, "in", Object.entries, template-literal
// string conversion) behave like their JS counterparts, including the places
// where JS throws a TypeError. Wherever the TS would throw, the port panics
// with a jsError internally and the exported functions turn that into an
// error return (the viewer shows its "Unable to generate ..." fallback text).
package schemacodegen

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind is the JSON/JS type of a Value.
type Kind int

// Value kinds. KindUndefined is the JS "undefined": it is what looking up a
// missing property yields and never appears in parsed JSON.
const (
	KindUndefined Kind = iota
	KindNull
	KindBool
	KindNumber
	KindString
	KindArray
	KindObject
)

// Value is an immutable-by-convention JSON value. Unlike map[string]any it
// preserves object key order exactly as JSON.parse does (including the JS
// rule that integer-like keys sort first, ascending).
type Value struct {
	kind Kind
	b    bool
	n    float64
	s    string
	arr  []Value
	obj  *Object
}

// Object is an ordered string-keyed map with JS property-order semantics.
type Object struct {
	keys []string
	vals []Value
	idx  map[string]int
}

// Kind reports the value's kind.
func (v Value) Kind() Kind { return v.kind }

// Keys returns an object's keys in JS property order (nil for non-objects).
func (v Value) Keys() []string {
	if v.kind != KindObject {
		return nil
	}
	return append([]string(nil), v.obj.keys...)
}

// Lookup returns the property of an object and whether it exists.
func (v Value) Lookup(key string) (Value, bool) {
	if v.kind != KindObject {
		return Value{}, false
	}
	i, ok := v.obj.idx[key]
	if !ok {
		return Value{}, false
	}
	return v.obj.vals[i], true
}

// String returns the string payload of a KindString value ("" otherwise).
func (v Value) String() string {
	if v.kind == KindString {
		return v.s
	}
	return ""
}

func newObject() *Object { return &Object{idx: map[string]int{}} }

// set assigns a property: a repeated key keeps its first position and takes
// the last value, exactly like assigning to a JS object.
func (o *Object) set(k string, v Value) {
	if i, ok := o.idx[k]; ok {
		o.vals[i] = v
		return
	}
	o.idx[k] = len(o.keys)
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

// normalize reorders keys into JS own-property order: canonical array-index
// keys ("0", "7", "42" ...) first in ascending numeric order, then every
// other key in insertion order.
func (o *Object) normalize() {
	type kv struct {
		k string
		v Value
		n uint64
	}
	var ints, rest []kv
	for i, k := range o.keys {
		if n, ok := arrayIndex(k); ok {
			ints = append(ints, kv{k, o.vals[i], n})
		} else {
			rest = append(rest, kv{k, o.vals[i], 0})
		}
	}
	if len(ints) == 0 {
		return
	}
	sort.SliceStable(ints, func(a, b int) bool { return ints[a].n < ints[b].n })
	all := slices.Concat(ints, rest)
	for i, e := range all {
		o.keys[i], o.vals[i], o.idx[e.k] = e.k, e.v, i
	}
}

// arrayIndex reports whether s is a canonical JS array index (0 .. 2^32-2).
func arrayIndex(s string) (uint64, bool) {
	if s == "" || len(s) > 10 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n >= 1<<32-1 {
		return 0, false
	}
	return n, true
}

var undefinedValue = Value{kind: KindUndefined}

func strValue(s string) Value          { return Value{kind: KindString, s: s} }
func numValue(n float64) Value         { return Value{kind: KindNumber, n: n} }
func boolValue(b bool) Value           { return Value{kind: KindBool, b: b} }
func objValue(o *Object) Value         { o.normalize(); return Value{kind: KindObject, obj: o} }
func arrValue(a []Value) Value         { return Value{kind: KindArray, arr: a} }
func nullValue() Value                 { return Value{kind: KindNull} }
func (v Value) isString(s string) bool { return v.kind == KindString && v.s == s }

// ---------------------------------------------------------------------------
// JS semantics helpers
// ---------------------------------------------------------------------------

// jsError stands in for a thrown JS exception (TypeError). Only the
// generators panic with it; exported entry points recover it into an error.
type jsError struct{ msg string }

func (e jsError) Error() string { return e.msg }

func throw(format string, args ...any) { panic(jsError{fmt.Sprintf(format, args...)}) }

// guard runs f and converts a jsError panic into an error return.
func guard[T any](f func() T) (out T, err error) {
	defer func() {
		if r := recover(); r != nil {
			var je jsError
			if e, ok := r.(jsError); ok {
				je = e
			} else {
				panic(r)
			}
			err = je
		}
	}()
	return f(), nil
}

// get is v[key]: undefined for a missing property or on a primitive/array,
// a TypeError on null/undefined.
func get(v Value, key string) Value {
	switch v.kind {
	case KindNull, KindUndefined:
		throw("Cannot read properties of %s (reading '%s')", kindName(v.kind), key)
	case KindObject:
		if x, ok := v.Lookup(key); ok {
			return x
		}
	case KindBool, KindNumber, KindString, KindArray:
	}
	return undefinedValue
}

// has is `key in v`: a TypeError unless v is an object or array.
func has(v Value, key string) bool {
	switch v.kind {
	case KindObject:
		_, ok := v.Lookup(key)
		return ok
	case KindArray:
		return false // callers only test non-index names ("const")
	case KindUndefined, KindNull, KindBool, KindNumber, KindString:
	}
	throw("Cannot use 'in' operator to search for '%s' in %s", key, kindName(v.kind))
	return false
}

func kindName(k Kind) string {
	switch k {
	case KindUndefined:
		return "undefined"
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindArray:
		return "array"
	case KindObject:
		return "object"
	}
	return "unknown"
}

// truthy is JS truthiness.
func truthy(v Value) bool {
	switch v.kind {
	case KindUndefined, KindNull:
		return false
	case KindBool:
		return v.b
	case KindNumber:
		return v.n != 0 && !math.IsNaN(v.n)
	case KindString:
		return v.s != ""
	case KindArray, KindObject:
		return true
	}
	return false
}

// isArray is Array.isArray.
func isArray(v Value) bool { return v.kind == KindArray }

// isObjectish is `typeof v === "object"` for a truthy v (arrays count).
func isObjectish(v Value) bool { return v.kind == KindObject || v.kind == KindArray }

type entry struct {
	key string
	val Value
}

// entries is Object.entries: own enumerable properties of objects, indexed
// elements of arrays, and the characters of a string; nothing for the rest.
func entries(v Value) []entry {
	switch v.kind {
	case KindObject:
		out := make([]entry, len(v.obj.keys))
		for i, k := range v.obj.keys {
			out[i] = entry{k, v.obj.vals[i]}
		}
		return out
	case KindArray:
		out := make([]entry, len(v.arr))
		for i, e := range v.arr {
			out[i] = entry{strconv.Itoa(i), e}
		}
		return out
	case KindString:
		var out []entry
		i := 0
		for _, r := range v.s {
			out = append(out, entry{strconv.Itoa(i), strValue(string(r))})
			i++
		}
		return out
	case KindUndefined, KindNull, KindBool, KindNumber:
	}
	return nil
}

// values is Object.values.
func values(v Value) []Value {
	es := entries(v)
	out := make([]Value, len(es))
	for i, e := range es {
		out[i] = e.val
	}
	return out
}

// list returns the elements of an array Value (callers have checked isArray).
func list(v Value) []Value { return v.arr }

// requiredSet is `new Set(Array.isArray(schema.required) ? schema.required : [])`
// restricted to the string members (the only ones a property name can equal).
func requiredSet(schema Value) map[string]bool {
	set := map[string]bool{}
	if r := get(schema, "required"); isArray(r) {
		for _, e := range list(r) {
			if e.kind == KindString {
				set[e.s] = true
			}
		}
	}
	return set
}

// withType is `{...schema, type: t}` for an object schema that already has a
// type key (so key order is unchanged).
func withType(schema, t Value) Value {
	o := newObject()
	for i, k := range schema.obj.keys {
		o.set(k, schema.obj.vals[i])
	}
	o.set("type", t)
	return objValue(o)
}

// uniq is [...new Set(xs)].
func uniq(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// jsToString is the template-literal conversion `${v}`.
func jsToString(v Value) string {
	switch v.kind {
	case KindUndefined:
		return "undefined"
	case KindNull:
		return "null"
	case KindBool:
		return strconv.FormatBool(v.b)
	case KindNumber:
		return numberString(v.n)
	case KindString:
		return v.s
	case KindArray:
		parts := make([]string, len(v.arr))
		for i, e := range v.arr {
			if e.kind != KindNull && e.kind != KindUndefined {
				parts[i] = jsToString(e)
			}
		}
		return strings.Join(parts, ",")
	case KindObject:
		return "[object Object]"
	}
	return ""
}

// numberString is ECMAScript Number::toString(10).
func numberString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case f == 0:
		return "0" // also -0
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	// Shortest round-trip digits: "d.ddde±XX".
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expStr, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp10, _ := strconv.Atoi(expStr)
	k := len(digits)
	n := exp10 + 1
	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits
	}
	ex := n - 1
	es := "+"
	if ex < 0 {
		es, ex = "-", -ex
	}
	if k == 1 {
		return sign + digits + "e" + es + strconv.Itoa(ex)
	}
	return sign + digits[:1] + "." + digits[1:] + "e" + es + strconv.Itoa(ex)
}

// ---------------------------------------------------------------------------
// Parse (JSON.parse)
// ---------------------------------------------------------------------------

const maxDepth = 5000

// Parse is JSON.parse: strict RFC 8259 text (only space, \t, \n, \r as
// whitespace), object key order preserved with JS ordering rules, duplicate
// keys last-wins at the first key's position, numbers as float64, lone
// \uD800-style escapes preserved (internally as WTF-8).
func Parse(src string) (Value, error) {
	p := &parser{s: strings.ToValidUTF8(src, "�")}
	p.ws()
	v, err := p.value(0)
	if err != nil {
		return Value{}, err
	}
	p.ws()
	if p.i < len(p.s) {
		return Value{}, p.errf("unexpected token after JSON value")
	}
	return v, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) errf(msg string) error {
	return fmt.Errorf("invalid JSON at offset %d: %s", p.i, msg)
}

func (p *parser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (Value, error) {
	if depth > maxDepth {
		return Value{}, p.errf("nesting too deep")
	}
	if p.i >= len(p.s) {
		return Value{}, p.errf("unexpected end of input")
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.str()
		return strValue(s), err
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	for _, lit := range []struct {
		text string
		v    Value
	}{{"true", boolValue(true)}, {"false", boolValue(false)}, {"null", nullValue()}} {
		if strings.HasPrefix(p.s[p.i:], lit.text) {
			p.i += len(lit.text)
			return lit.v, nil
		}
	}
	return Value{}, p.errf("unexpected token")
}

func (p *parser) object(depth int) (Value, error) {
	p.i++ // {
	o := newObject()
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return objValue(o), nil
	}
	for {
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return Value{}, p.errf("expected property name")
		}
		k, err := p.str()
		if err != nil {
			return Value{}, err
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return Value{}, p.errf("expected ':'")
		}
		p.i++
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		o.set(k, v)
		p.ws()
		if p.i >= len(p.s) {
			return Value{}, p.errf("unexpected end of input")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return objValue(o), nil
		default:
			return Value{}, p.errf("expected ',' or '}'")
		}
	}
}

func (p *parser) array(depth int) (Value, error) {
	p.i++ // [
	arr := []Value{}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return arrValue(arr), nil
	}
	for {
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		arr = append(arr, v)
		p.ws()
		if p.i >= len(p.s) {
			return Value{}, p.errf("unexpected end of input")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return arrValue(arr), nil
		default:
			return Value{}, p.errf("expected ',' or ']'")
		}
	}
}

func (p *parser) number() (Value, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	digits := func() int {
		n := 0
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	switch {
	case p.i < len(p.s) && p.s[p.i] == '0':
		p.i++
	case digits() == 0:
		return Value{}, p.errf("invalid number")
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if digits() == 0 {
			return Value{}, p.errf("invalid number")
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if digits() == 0 {
			return Value{}, p.errf("invalid number")
		}
	}
	// Out-of-range literals parse to ±Inf like JS (ParseFloat returns the
	// infinity alongside the range error).
	f, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return Value{}, p.errf("invalid number")
	}
	return numValue(f), nil
}

func (p *parser) str() (string, error) {
	p.i++ // opening quote
	var sb strings.Builder
	for {
		if p.i >= len(p.s) {
			return "", p.errf("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return sb.String(), nil
		case c < 0x20:
			return "", p.errf("control character in string")
		case c != '\\':
			sb.WriteByte(c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.s) {
			return "", p.errf("unterminated string")
		}
		esc := p.s[p.i]
		p.i++
		switch esc {
		case '"', '\\', '/':
			sb.WriteByte(esc)
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'u':
			u, ok := p.hex4()
			if !ok {
				return "", p.errf("bad \\u escape")
			}
			if u >= 0xD800 && u <= 0xDBFF && strings.HasPrefix(p.s[p.i:], "\\u") {
				save := p.i
				p.i += 2
				if lo, ok := p.hex4(); ok && lo >= 0xDC00 && lo <= 0xDFFF {
					sb.WriteRune(0x10000 + (u-0xD800)<<10 + (lo - 0xDC00))
					continue
				}
				p.i = save
			}
			writeUnit(&sb, u)
		default:
			return "", p.errf("bad escape")
		}
	}
}

func (p *parser) hex4() (rune, bool) {
	if p.i+4 > len(p.s) {
		return 0, false
	}
	n, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 32)
	if err != nil || strings.ContainsAny(p.s[p.i:p.i+4], "+-_") {
		return 0, false
	}
	p.i += 4
	return rune(n), true
}

// writeUnit writes a UTF-16 code unit; lone surrogates (not valid UTF-8) are
// kept as 3-byte WTF-8 so Stringify can re-emit them as \udXXX.
func writeUnit(sb *strings.Builder, u rune) {
	if u >= 0xD800 && u <= 0xDFFF {
		sb.WriteByte(0xE0 | byte(u>>12))
		sb.WriteByte(0x80 | byte(u>>6)&0x3F)
		sb.WriteByte(0x80 | byte(u)&0x3F)
		return
	}
	sb.WriteRune(u)
}

// scrub replaces WTF-8 lone surrogates (and any other invalid UTF-8) with
// U+FFFD, one replacement per lone surrogate, which is what TextEncoder (and
// therefore a browser rendering or a file write of the JS string) produces.
func scrub(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2]&0xC0 == 0x80 {
			sb.WriteRune(utf8.RuneError)
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		sb.WriteRune(r)
		i += size
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Stringify (JSON.stringify)
// ---------------------------------------------------------------------------

// Stringify is JSON.stringify(v, null, indent): indent "" gives compact
// output, "  " gives the two-space form the viewer uses. Key order, string
// escaping (< > & are NOT escaped; control characters are \n \t ... or
// \u00xx, lone surrogates \udXXX) and number formatting follow JS exactly.
func Stringify(v Value, indent string) string {
	var sb strings.Builder
	writeJSON(&sb, v, indent, "")
	return sb.String()
}

func writeJSON(sb *strings.Builder, v Value, indent, cur string) {
	switch v.kind {
	case KindUndefined, KindNull:
		sb.WriteString("null")
	case KindBool:
		sb.WriteString(strconv.FormatBool(v.b))
	case KindNumber:
		if math.IsInf(v.n, 0) || math.IsNaN(v.n) {
			sb.WriteString("null")
		} else {
			sb.WriteString(numberString(v.n))
		}
	case KindString:
		quote(sb, v.s)
	case KindArray:
		if len(v.arr) == 0 {
			sb.WriteString("[]")
			return
		}
		inner := cur + indent
		sb.WriteByte('[')
		for i, e := range v.arr {
			if i > 0 {
				sb.WriteByte(',')
			}
			newline(sb, indent, inner)
			writeJSON(sb, e, indent, inner)
		}
		newline(sb, indent, cur)
		sb.WriteByte(']')
	case KindObject:
		if len(v.obj.keys) == 0 {
			sb.WriteString("{}")
			return
		}
		inner := cur + indent
		sb.WriteByte('{')
		for i, k := range v.obj.keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			newline(sb, indent, inner)
			quote(sb, k)
			sb.WriteByte(':')
			if indent != "" {
				sb.WriteByte(' ')
			}
			writeJSON(sb, v.obj.vals[i], indent, inner)
		}
		newline(sb, indent, cur)
		sb.WriteByte('}')
	}
}

func newline(sb *strings.Builder, indent, cur string) {
	if indent != "" {
		sb.WriteByte('\n')
		sb.WriteString(cur)
	}
}

const hexDigits = "0123456789abcdef"

func quote(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			sb.WriteString(`\"`)
		case c == '\\':
			sb.WriteString(`\\`)
		case c == '\b':
			sb.WriteString(`\b`)
		case c == '\f':
			sb.WriteString(`\f`)
		case c == '\n':
			sb.WriteString(`\n`)
		case c == '\r':
			sb.WriteString(`\r`)
		case c == '\t':
			sb.WriteString(`\t`)
		case c < 0x20:
			sb.WriteString(`\u00`)
			sb.WriteByte(hexDigits[c>>4])
			sb.WriteByte(hexDigits[c&0xF])
		case c < utf8.RuneSelf:
			sb.WriteByte(c)
		case c == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2]&0xC0 == 0x80:
			// WTF-8 lone surrogate -> \udXXX
			u := rune(c&0x0F)<<12 | rune(s[i+1]&0x3F)<<6 | rune(s[i+2]&0x3F)
			sb.WriteString(`\u`)
			for shift := 12; shift >= 0; shift -= 4 {
				sb.WriteByte(hexDigits[(u>>shift)&0xF])
			}
			i += 3
			continue
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			sb.WriteRune(r)
			i += size
			continue
		}
		i++
	}
	sb.WriteByte('"')
}

// ---------------------------------------------------------------------------
// Case mapping (String.prototype.toLowerCase / toUpperCase)
// ---------------------------------------------------------------------------

// isWTF8Surrogate reports whether s[i:] starts with a 3-byte encoded lone
// surrogate (see writeUnit).
func isWTF8Surrogate(s string, i int) bool {
	return s[i] == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2]&0xC0 == 0x80
}

// mapValid applies f to the valid-UTF-8 stretches of s and leaves WTF-8 lone
// surrogates untouched (JS case mapping leaves lone surrogates alone, whereas
// Go's strings.ToLower would turn each byte into U+FFFD).
//
// Go note: Go uses simple Unicode case mapping where JS uses full mapping, so
// special cases such as "ß".toUpperCase() ("SS"), "İ".toLowerCase() and the
// Greek final sigma differ. Property names and event codes are ASCII in
// practice.
func mapValid(s string, f func(string) string) string {
	if utf8.ValidString(s) {
		return f(s)
	}
	var sb strings.Builder
	start := 0
	for i := 0; i < len(s); {
		if isWTF8Surrogate(s, i) {
			sb.WriteString(f(s[start:i]))
			sb.WriteString(s[i : i+3])
			i += 3
			start = i
			continue
		}
		i++
	}
	sb.WriteString(f(s[start:]))
	return sb.String()
}

func jsLower(s string) string { return mapValid(s, strings.ToLower) }

// splitFirst is `[s.charAt(0), s.slice(1)]` for case-mapping purposes: the
// first UTF-16 code unit. A character outside the BMP is two code units in
// JS, neither of which changes under case mapping, so it is returned as the
// "unmappable" first part (mappable=false).
func splitFirst(s string) (first, rest string, mappable bool) {
	if s == "" {
		return "", "", false
	}
	if isWTF8Surrogate(s, 0) {
		return s[:3], s[3:], false
	}
	r, n := utf8.DecodeRuneInString(s)
	return s[:n], s[n:], r < 0x10000
}

// pascalCase is `s.split(/[-_]/).map(w => w.charAt(0).toUpperCase() +
// w.slice(1).toLowerCase()).join("")`.
func pascalCase(s string) string {
	var sb strings.Builder
	for _, w := range nameSplit.Split(s, -1) {
		first, rest, mappable := splitFirst(w)
		if mappable {
			first = strings.ToUpper(first)
		}
		sb.WriteString(first)
		sb.WriteString(jsLower(rest))
	}
	return sb.String()
}

// camelCase is pascalCase with the first character lower-cased.
func camelCase(s string) string {
	first, rest, mappable := splitFirst(pascalCase(s))
	if mappable {
		first = strings.ToLower(first)
	}
	return first + rest
}
