package schemacodegen

// Syntax highlighting (port of frontend/src/utils/schema-highlight.ts).
//
// Lightweight single-pass regex highlighting for JSON, PHP, TypeScript,
// Python and Java. Each language uses one combined regex so matched tokens
// never interfere with each other.
//
// SAFETY: every Highlight* function returns HTML that is safe to insert as
// element content (e.g. via templ.Raw inside a <pre><code>). All input text
// is HTML-escaped before tokenising and the only markup in the result is the
// package's own <span class="hl-..."> wrappers, whose contents are escaped
// text. Quotes are not escaped outside the languages' own string tokens
// (the output is never meant for an attribute value). Do not place the
// result inside an attribute.
//
// Differences from the TS: highlightJson in the TS does not escape its input
// at all (an XSS in the SPA); here JSON input is escaped for & < > first.
// That is invisible for input without those characters. Go's RE2 has no
// lookahead, so the post-escape string alternative
// `&quot;(?:[^&]|&(?!quot;))*?&quot;` is written as the equivalent lazy
// `&quot;(?s:.*?)&quot;` (a lazy match can never run past the first &quot;
// anyway). The Python `^` anchor (multiline) matches after \n only, whereas
// JS also matches after \r, U+2028 and U+2029.

import (
	"regexp"
	"strings"
)

// Fragments shared by the patterns. JS `.` excludes \n \r U+2028 U+2029 and
// JS `\s` is the Unicode white-space set; RE2's `.` and `\s` differ, so both
// are spelled out.
const (
	reDot   = `[^\n\r\x{2028}\x{2029}]`
	reSpace = `[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`
	reEsc   = `&quot;(?s:.*?)&quot;` // string token after escapeHTML
)

var (
	htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	// jsonEscaper leaves quotes alone: the JSON tokenizer needs the raw ".
	jsonEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

func wrap(cls, text string) string {
	return `<span class="` + cls + `">` + text + `</span>`
}

// replacer receives the 1-based index of the capture group that matched, the
// submatch indexes and the subject, and returns the replacement text.
type replacer func(group int, m []int, s string) string

func highlight(re *regexp.Regexp, s string, rep replacer) string {
	var sb strings.Builder
	prev := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		sb.WriteString(s[prev:m[0]])
		prev = m[1]
		for g := 1; 2*g < len(m); g++ {
			if m[2*g] >= 0 {
				sb.WriteString(rep(g, m, s))
				break
			}
		}
	}
	sb.WriteString(s[prev:])
	return sb.String()
}

func group(s string, m []int, g int) string { return s[m[2*g]:m[2*g+1]] }

// classify wraps identifier text with the class of the set it belongs to.
func classify(text string, sets ...identSet) string {
	for _, set := range sets {
		if set.names[text] {
			return wrap(set.class, text)
		}
	}
	return text
}

type identSet struct {
	class string
	names map[string]bool
}

func newSet(class string, names ...string) identSet {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return identSet{class, m}
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

var jsonRE = regexp.MustCompile(
	`("(?:\\` + reDot + `|[^"\\])*")` + reSpace + `*(:)` +
		`|("(?:\\` + reDot + `|[^"\\])*")` +
		`|(true|false|null)` +
		`|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)`)

// HighlightJSON highlights JSON text. The result is safe HTML (see the file
// comment).
func HighlightJSON(json string) string {
	return highlight(jsonRE, jsonEscaper.Replace(json), func(g int, m []int, s string) string {
		switch g {
		case 1:
			return wrap("hl-key", group(s, m, 1)) + group(s, m, 2)
		case 3:
			return wrap("hl-str", group(s, m, 3))
		case 4:
			return wrap("hl-kw", group(s, m, 4))
		default:
			return wrap("hl-num", group(s, m, 5))
		}
	})
}

// ---------------------------------------------------------------------------
// TypeScript
// ---------------------------------------------------------------------------

var (
	tsKeywords = newSet("hl-kw", "export", "interface", "type", "const", "let", "var", "function", "return",
		"import", "from", "readonly", "extends", "implements", "class", "new",
		"typeof", "keyof", "in", "of", "as")
	tsTypes = newSet("hl-type", "string", "number", "boolean", "null", "undefined", "unknown", "void",
		"never", "any", "Record", "Array", "Partial", "Required", "Pick", "Omit")

	tsRE = regexp.MustCompile(strings.Join([]string{
		`(//` + reDot + `*)`,              // comment
		`("(?:\\` + reDot + `|[^"\\])*")`, // string (pre-escape: won't appear after escapeHtml)
		`(` + reEsc + `)`,                 // string (post-escape)
		`('(?:\\'|[^'])*')`,               // single-quoted string
		`(\b\d+(?:\.\d+)?\b)`,             // number
		`(\b[A-Za-z_]\w*\b)`,              // identifier
	}, "|"))
)

// HighlightTypeScript highlights generated TypeScript. The result is safe
// HTML (see the file comment).
func HighlightTypeScript(code string) string {
	return highlight(tsRE, htmlEscaper.Replace(code), func(g int, m []int, s string) string {
		text := group(s, m, g)
		switch g {
		case 1:
			return wrap("hl-comment", text)
		case 2, 3, 4:
			return wrap("hl-str", text)
		case 5:
			return wrap("hl-num", text)
		default:
			return classify(text, tsKeywords, tsTypes)
		}
	})
}

// ---------------------------------------------------------------------------
// PHP
// ---------------------------------------------------------------------------

var (
	phpKeywords = newSet("hl-kw", "declare", "namespace", "use", "final", "readonly", "class", "function",
		"public", "private", "protected", "static", "return", "new", "self",
		"array", "int", "float", "bool", "string", "mixed", "null", "true",
		"false", "strict_types")

	phpRE = regexp.MustCompile(strings.Join([]string{
		`(/\*\*[\s\S]*?\*/)`, // doc comment (multiline)
		`(//` + reDot + `*)`, // single-line comment
		`('(?:\\'|[^'])*')`,  // single-quoted string
		`(\$\w+)`,            // variable
		`(&lt;\?php)`,        // opening tag (escaped)
		`(\b\d+\b)`,          // number
		`(\b[A-Za-z_]\w*\b)`, // identifier
	}, "|"))
)

// HighlightPHP highlights generated PHP. The result is safe HTML (see the
// file comment).
func HighlightPHP(code string) string {
	return highlight(phpRE, htmlEscaper.Replace(code), func(g int, m []int, s string) string {
		text := group(s, m, g)
		switch g {
		case 1, 2:
			return wrap("hl-comment", text)
		case 3:
			return wrap("hl-str", text)
		case 4:
			return wrap("hl-var", text)
		case 5:
			return wrap("hl-kw", text)
		case 6:
			return wrap("hl-num", text)
		default:
			return classify(text, phpKeywords)
		}
	})
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

var (
	pyKeywords = newSet("hl-kw", "from", "import", "class", "def", "return", "if", "elif", "else", "for",
		"in", "is", "not", "and", "or", "with", "as", "pass", "None", "True",
		"False", "self", "cls")
	pyTypes = newSet("hl-type", "str", "int", "float", "bool", "list", "dict", "tuple", "set",
		"Any", "Literal", "Optional", "Union")

	pyRE = regexp.MustCompile(`(?m)` + strings.Join([]string{
		`(#` + reDot + `*)`,               // comment
		`^(@\w+(?:\(` + reDot + `*?\))?)`, // decorator (start of line)
		`('(?:\\'|[^'])*')`,               // single-quoted string
		`("(?:\\"|[^"])*")`,               // double-quoted string (pre-escape)
		`(` + reEsc + `)`,                 // double-quoted string (post-escape)
		`(\b\d+(?:\.\d+)?\b)`,             // number
		`(\b[A-Za-z_]\w*\b)`,              // identifier
	}, "|"))
)

// HighlightPython highlights generated Python. The result is safe HTML (see
// the file comment).
func HighlightPython(code string) string {
	return highlight(pyRE, htmlEscaper.Replace(code), func(g int, m []int, s string) string {
		text := group(s, m, g)
		switch g {
		case 1, 2:
			return wrap("hl-comment", text)
		case 3, 4, 5:
			return wrap("hl-str", text)
		case 6:
			return wrap("hl-num", text)
		default:
			return classify(text, pyKeywords, pyTypes)
		}
	})
}

// ---------------------------------------------------------------------------
// Java
// ---------------------------------------------------------------------------

var (
	javaKeywords = newSet("hl-kw", "package", "import", "public", "private", "protected", "static", "final",
		"record", "class", "interface", "enum", "extends", "implements", "new",
		"return", "var", "void", "if", "else", "for", "while", "switch", "case",
		"default", "throws", "throw", "try", "catch", "finally")
	javaTypes = newSet("hl-type", "String", "Integer", "Double", "Boolean", "Object", "Map", "List", "Set",
		"LinkedHashMap", "HashMap", "ArrayList",
		"int", "double", "boolean", "long", "float", "char", "byte", "short")

	javaRE = regexp.MustCompile(strings.Join([]string{
		`(//` + reDot + `*)`,           // single-line comment
		`(@\w+)`,                       // annotation
		`("(?:\\"|[^"])*")`,            // string (pre-escape)
		`(` + reEsc + `)`,              // string (post-escape)
		`(\b\d+(?:\.\d+)?[fFdDlL]?\b)`, // number
		`(\b[A-Za-z_]\w*\b)`,           // identifier
	}, "|"))
)

// HighlightJava highlights generated Java. The result is safe HTML (see the
// file comment).
func HighlightJava(code string) string {
	return highlight(javaRE, htmlEscaper.Replace(code), func(g int, m []int, s string) string {
		text := group(s, m, g)
		switch g {
		case 1, 2:
			return wrap("hl-comment", text)
		case 3, 4:
			return wrap("hl-str", text)
		case 5:
			return wrap("hl-num", text)
		default:
			return classify(text, javaKeywords, javaTypes)
		}
	})
}
