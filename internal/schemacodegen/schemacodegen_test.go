package schemacodegen

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const defaultEventCode = "platform:iam:user:created"

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func schemaNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("testdata/schemas/*.json")
	if err != nil || len(files) < 15 {
		t.Fatalf("expected at least 15 schema inputs, got %d (%v)", len(files), err)
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(filepath.Base(f), ".json")
	}
	return names
}

// TestGoldens runs every generator and highlighter over every input and
// compares byte for byte with output of the original TypeScript.
func TestGoldens(t *testing.T) {
	for _, name := range schemaNames(t) {
		src := readFile(t, "testdata/schemas/"+name+".json")
		code := defaultEventCode
		if b, err := os.ReadFile("testdata/schemas/" + name + ".code"); err == nil {
			code = string(b)
		}

		schema, example := FormatSchema(src), ExampleJSON(src)
		ts, php := TypeScriptText(src, code), PHPText(src, code)
		py, java := PythonText(src, code), JavaText(src, code)

		cases := []struct{ tab, got string }{
			{"schema", schema},
			{"example", example},
			{"typescript", ts},
			{"php", php},
			{"python", py},
			{"java", java},
			{"hl-schema", HighlightJSON(schema)},
			{"hl-example", HighlightJSON(example)},
			{"hl-typescript", HighlightTypeScript(ts)},
			{"hl-php", HighlightPHP(php)},
			{"hl-python", HighlightPython(py)},
			{"hl-java", HighlightJava(java)},
		}
		for _, c := range cases {
			t.Run(name+"/"+c.tab, func(t *testing.T) {
				want := readFile(t, "testdata/golden/"+name+"."+c.tab+".txt")
				if c.got != want {
					t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", c.got, want)
				}
			})
		}
	}
}

type exportGolden struct {
	Exported int               `json:"exported"`
	Errors   []string          `json:"errors"`
	Order    []string          `json:"order"`
	Files    map[string]string `json:"files"`
}

func TestExportZip(t *testing.T) {
	inputs, _ := filepath.Glob("testdata/export/*.json")
	if len(inputs) == 0 {
		t.Fatal("no export inputs")
	}
	for _, in := range inputs {
		name := filepath.Base(in)
		t.Run(name, func(t *testing.T) {
			var raw []struct {
				Application, Subdomain, Aggregate, Event string
				CurrentSchema                            string `json:"currentSchema"`
			}
			if err := json.Unmarshal([]byte(readFile(t, in)), &raw); err != nil {
				t.Fatal(err)
			}
			ets := make([]ExportEventType, len(raw))
			for i, r := range raw {
				ets[i] = ExportEventType{r.Application, r.Subdomain, r.Aggregate, r.Event, r.CurrentSchema}
			}
			var want exportGolden
			if err := json.Unmarshal([]byte(readFile(t, "testdata/golden/export/"+name)), &want); err != nil {
				t.Fatal(err)
			}

			z, exported, errs, err := ExportZip(ets)
			if err != nil {
				t.Fatal(err)
			}
			if exported != want.Exported {
				t.Errorf("exported = %d, want %d", exported, want.Exported)
			}
			if strings.Join(errs, "\n") != strings.Join(want.Errors, "\n") {
				t.Errorf("errors = %q, want %q", errs, want.Errors)
			}
			got := map[string]string{}
			var order []string
			if z != nil {
				zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range zr.File {
					if !f.Modified.Equal(zipEpoch) {
						t.Errorf("%s: modified %v, want %v", f.Name, f.Modified, zipEpoch)
					}
					rc, _ := f.Open()
					b, _ := io.ReadAll(rc)
					_ = rc.Close()
					got[f.Name] = string(b)
					order = append(order, f.Name)
				}
			} else if want.Exported != 0 {
				t.Fatal("no zip produced")
			}
			if strings.Join(order, "\n") != strings.Join(want.Order, "\n") {
				t.Errorf("entry order\n got %q\nwant %q", order, want.Order)
			}
			var names []string
			for k := range want.Files {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if got[k] != want.Files[k] {
					t.Errorf("%s: content mismatch\n--- got ---\n%s\n--- want ---\n%s", k, got[k], want.Files[k])
				}
			}
			if len(got) != len(want.Files) {
				t.Errorf("%d entries, want %d", len(got), len(want.Files))
			}
		})
	}

	// Determinism: identical input gives identical bytes.
	ets := []ExportEventType{{"a", "b", "c", "d", `{"type":"object"}`}}
	z1, _, _, _ := ExportZip(ets)
	z2, _, _, _ := ExportZip(ets)
	if !bytes.Equal(z1, z2) || z1 == nil {
		t.Error("ExportZip is not deterministic")
	}
}

// TestHighlightHostile checks that no input text reaches the output as live
// markup and that unescaping the output round-trips the input.
func TestHighlightHostile(t *testing.T) {
	hostile := []string{
		`</span><script>alert(1)</script>`,
		`&lt;script&gt;alert(1)&lt;/script&gt; &amp; &quot;`,
		`"><img src=x onerror=alert(1)>`,
		`'; </span><b onmouseover=x>`,
		"// </span><script>\n# <script>\n/** <script> */ 'a<script>' \"b<script>\"",
	}
	funcs := map[string]func(string) string{
		"json": HighlightJSON, "ts": HighlightTypeScript, "php": HighlightPHP,
		"python": HighlightPython, "java": HighlightJava,
	}
	for fname, f := range funcs {
		for _, in := range hostile {
			for _, wrapped := range []string{in, `{"k": "` + in + `"}`, "x = '" + in + "'\n// " + in} {
				out := f(wrapped)
				stripped := stripOurSpans(t, out)
				if strings.ContainsAny(stripped, "<>") {
					t.Errorf("%s: markup leaked for %q: %s", fname, wrapped, out)
				}
				if strings.Contains(strings.ToLower(out), "<script") {
					t.Errorf("%s: <script in output for %q", fname, wrapped)
				}
				// Round trip: remove our spans, undo the escaping.
				back := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&").Replace(stripped)
				if back != wrapped {
					t.Errorf("%s: round trip\n got %q\nwant %q", fname, back, wrapped)
				}
			}
		}
	}
}

// stripOurSpans removes the package's own <span class="hl-...">/</span> tags.
func stripOurSpans(t *testing.T, s string) string {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], `<span class="hl-`):
			end := strings.Index(s[i:], `">`)
			if end < 0 {
				t.Fatalf("unterminated span in %q", s)
			}
			i += end + 2
		case strings.HasPrefix(s[i:], "</span>"):
			i += len("</span>")
		default:
			sb.WriteByte(s[i])
			i++
		}
	}
	return sb.String()
}

func TestStringifyMatchesJS(t *testing.T) {
	cases := map[string]string{
		`1.0`:                       `1`,
		`1e2`:                       `100`,
		`1.5e-7`:                    `1.5e-7`,
		`-0`:                        `0`,
		`1e21`:                      `1e+21`,
		`1e-7`:                      `1e-7`,
		`0.000001`:                  `0.000001`,
		`123456789012345678901`:     `123456789012345680000`,
		`1e999`:                     `null`,
		`"<>&"`:                     `"<>&"`,
		`"\u0001\ud800"`:            `"\u0001\ud800"`,
		`{"b":1,"1":2,"a":3,"b":4}`: "{\n  \"1\": 2,\n  \"b\": 4,\n  \"a\": 3\n}",
		`[]`:                        `[]`,
		`{}`:                        `{}`,
	}
	for in, want := range cases {
		v, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%s): %v", in, err)
			continue
		}
		if got := Stringify(v, "  "); got != want {
			t.Errorf("Stringify(Parse(%s)) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{``, `{`, `[1,]`, `{"a":1,}`, `01`, `1.`, `.5`, `'a'`, `"\x"`, "\"a\nb\"", `nul`, `[1] 2`, "\xef\xbb\xbf1"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", bad)
		}
	}
}
