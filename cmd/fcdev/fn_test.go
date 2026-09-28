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
				err := template.Must(template.New(rel).Parse(text)).Execute(&b, struct{ Name, App, SDK string }{"hello", "demo", sdk})
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
