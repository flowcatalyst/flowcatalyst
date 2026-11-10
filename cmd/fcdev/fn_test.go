package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func TestDetectLang(t *testing.T) {
	for file, want := range map[string]string{"Cargo.toml": langRust, "package.json": langTS, "go.mod": langGo, "main.go": langGo} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got, err := detectLang(dir); err != nil || got != want {
			t.Errorf("%s: %q, %v", file, got, err)
		}
	}
	if _, err := detectLang(t.TempDir()); err == nil {
		t.Error("an empty directory has a language")
	}
}

func TestRuntimeFor(t *testing.T) {
	if runtimeFor("dist/function.js") != "js" || runtimeFor("function.wasm") != "wasm" {
		t.Fatal("runtime by extension")
	}
}

func TestFnTemplatesRender(t *testing.T) {
	for lang, files := range fnTemplates {
		for rel, text := range files {
			for _, sdk := range []string{"", "/src/clients"} {
				var b strings.Builder
				err := template.Must(template.New(rel).Parse(text)).Execute(&b, newFnInitData("hello", "demo", sdk))
				if err != nil {
					t.Errorf("%s/%s: %v", lang, rel, err)
				}
				if sdk != "" && (rel == "go.mod" || rel == "Cargo.toml" || rel == "package.json") && !strings.Contains(b.String(), sdk) {
					t.Errorf("%s/%s ignores --sdk-path", lang, rel)
				}
			}
		}
	}
}

func TestFnInitRejects(t *testing.T) {
	cmd := newFnInitCmd()
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "Bad_Name"), "--app", "demo"})
	if err := cmd.Execute(); err == nil {
		t.Error("an invalid function name was accepted")
	}
	cmd = newFnInitCmd()
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "ok"), "--lang", "cobol", "--app", "demo"})
	if err := cmd.Execute(); err == nil {
		t.Error("an unknown language was accepted")
	}
	cmd = newFnInitCmd()
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "ok")})
	if err := cmd.Execute(); err == nil {
		t.Error("a missing --app was accepted")
	}
}

func TestFnSDKVersionsMatchVersionFiles(t *testing.T) {
	for file, want := range map[string]string{"fn-go": fnGoSDKVersion, "fn-ts": fnTSSDKVersion} {
		b, err := os.ReadFile(filepath.Join("..", "..", "clients", file, "VERSION"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(b)); got != want {
			t.Errorf("clients/%s/VERSION is %s but fn init uses %s: update cmd/fcdev/fn_sdk_versions.go", file, got, want)
		}
	}
}

func initFiles(t *testing.T, lang string, extra ...string) map[string]string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hello")
	cmd := newFnInitCmd()
	cmd.SetArgs(append([]string{dir, "--lang", lang, "--app", "demo"}, extra...))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range []string{"go.mod", "Cargo.toml", "package.json"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			out[f] = string(b)
		}
	}
	return out
}

func TestFnInitPinsPublishedSDKVersions(t *testing.T) {
	gomod := initFiles(t, "go")["go.mod"]
	if !strings.Contains(gomod, "require github.com/flowcatalyst/flowcatalyst/clients/fn-go v"+fnGoSDKVersion+"\n") || strings.Contains(gomod, "v0.0.0") || strings.Contains(gomod, "replace") {
		t.Errorf("go.mod:\n%s", gomod)
	}
	pkg := initFiles(t, "ts")["package.json"]
	if !strings.Contains(pkg, `"@flowcatalyst/fn": "github:flowcatalyst/fn-ts-sdk#v`+fnTSSDKVersion+`"`) {
		t.Errorf("package.json:\n%s", pkg)
	}
	cargo := initFiles(t, "rust")["Cargo.toml"]
	want := `flowcatalyst-fn = { git = "https://github.com/flowcatalyst/flowcatalyst", tag = "fcdev/v` + version() + `" }`
	if !strings.Contains(cargo, want) {
		t.Errorf("Cargo.toml:\n%s", cargo)
	}
}

func TestFnInitSDKPathStillWins(t *testing.T) {
	sdk := t.TempDir()
	if g := initFiles(t, "go", "--sdk-path", sdk)["go.mod"]; !strings.Contains(g, "replace github.com/flowcatalyst/flowcatalyst/clients/fn-go => "+sdk+"/fn-go") {
		t.Errorf("go.mod:\n%s", g)
	}
	if p := initFiles(t, "ts", "--sdk-path", sdk)["package.json"]; !strings.Contains(p, "file:"+sdk+"/fn-ts") || strings.Contains(p, "github:") {
		t.Errorf("package.json:\n%s", p)
	}
	if c := initFiles(t, "rust", "--sdk-path", sdk)["Cargo.toml"]; !strings.Contains(c, `path = "`+sdk+`/fn-rust"`) || strings.Contains(c, "git =") {
		t.Errorf("Cargo.toml:\n%s", c)
	}
}
