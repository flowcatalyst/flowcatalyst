package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

var fnNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// fnTemplates are the starter projects `fcdev fn init` writes, per language.
// {{.Name}} is the function name, {{.App}} the application code,
// {{.SDK}} a local SDK checkout (clients/) or "" for the published SDK.
var fnTemplates = map[string]map[string]string{
	langGo: {
		"go.mod": `module {{.App}}/{{.Name}}

go 1.24

require github.com/flowcatalyst/flowcatalyst-go/clients/fn-go v0.0.0
{{if .SDK}}
replace github.com/flowcatalyst/flowcatalyst-go/clients/fn-go => {{.SDK}}/fn-go
{{end}}`,
		"main.go": `// Command {{.Name}} is a FlowCatalyst function. Build and check it with
// ` + "`fcdev fn build`" + `, then ` + "`fcdev fn deploy {{.App}}.{{.Name}}`" + `.
package main

import (
	"net/http"

	fn "github.com/flowcatalyst/flowcatalyst-go/clients/fn-go"
)

var greeting = fn.Config("GREETING")

func init() {
	fn.Open("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	fn.Platform("GET /hello/{name}", hello)
}

func main() {}

func hello(w http.ResponseWriter, r *http.Request) {
	g, found, err := greeting.Get(r.Context())
	if err != nil || !found {
		g = "Hello"
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(g + ", " + r.PathValue("name")))
}
`,
		".gitignore": "*.wasm\n",
	},
	langRust: {
		"Cargo.toml": `[package]
name = "{{.Name}}"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
{{if .SDK}}flowcatalyst-fn = { path = "{{.SDK}}/fn-rust" }{{else}}flowcatalyst-fn = "0.1"{{end}}

[profile.release]
opt-level = "s"
lto = true
panic = "abort"
strip = true
`,
		"src/lib.rs": `//! {{.App}}.{{.Name}} — a FlowCatalyst function. Build and check it with
//! ` + "`fcdev fn build`" + `, then ` + "`fcdev fn deploy {{.App}}.{{.Name}}`" + `.
use flowcatalyst_fn::{App, Context, Error, Request, Response};

flowcatalyst_fn::export!(|app: &mut App| {
    app.open("GET /healthz", health)
        .platform("GET /hello/{name}", hello)
        .config("GREETING");
});

fn health(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
    Ok(Response::new(200).with_body(b"ok".to_vec()))
}

fn hello(req: &Request, ctx: &Context) -> Result<Response, Error> {
    let greeting = ctx.config_get("GREETING").ok().flatten().unwrap_or_else(|| "Hello".into());
    let name = req.path_param("name").unwrap_or("world");
    Ok(Response::new(200).with_body(format!("{greeting}, {name}").into_bytes()))
}
`,
		".gitignore": "target/\n",
	},
	langTS: {
		"package.json": `{
  "name": "{{.Name}}",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "esbuild src/index.ts --bundle --format=iife --platform=neutral --target=es2020 --outfile=dist/function.js"
  },
  "dependencies": {
    "@flowcatalyst/fn": "{{if .SDK}}file:{{.SDK}}/fn-ts{{else}}^0.1.0{{end}}"
  },
  "devDependencies": {
    "esbuild": "^0.25.0",
    "typescript": "^5.6.0"
  }
}
`,
		"src/index.ts": `// {{.App}}.{{.Name}} — a FlowCatalyst function. Build and check it with
// ` + "`fcdev fn build`" + `, then ` + "`fcdev fn deploy {{.App}}.{{.Name}}`" + `.
import { fn } from "@flowcatalyst/fn";

const greeting = fn.config("GREETING");

fn.open("GET /healthz", () => fn.text(200, "ok"));
fn.platform("GET /hello/{name}", (req) => fn.text(200, ` + "`${greeting.get() ?? \"Hello\"}, ${req.params.name}`" + `));
`,
		"tsconfig.json": `{
  "compilerOptions": { "target": "es2020", "module": "esnext", "moduleResolution": "bundler", "strict": true, "noEmit": true }
}
`,
		".gitignore": "node_modules/\ndist/\n",
	},
}

func newFnInitCmd() *cobra.Command {
	var lang, app, sdk string
	cmd := &cobra.Command{
		Use:   "init <dir>",
		Short: "Scaffold a function project (Go, Rust or TypeScript)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := args[0]
			name := filepath.Base(filepath.Clean(dir))
			if !fnNamePattern.MatchString(name) {
				return fmt.Errorf("the directory name %q is the function name and must match %s", name, fnNamePattern)
			}
			files, ok := fnTemplates[lang]
			if !ok {
				return fmt.Errorf("--lang must be go, rust or ts")
			}
			if app == "" {
				return fmt.Errorf("--app is required: the application code that owns the function (its address is <app>.%s)", name)
			}
			if sdk != "" {
				abs, err := filepath.Abs(sdk)
				if err != nil {
					return err
				}
				sdk = abs
			}
			if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
				return fmt.Errorf("%s exists and is not empty", dir)
			}
			data := struct{ Name, App, SDK string }{name, app, sdk}
			for rel, text := range files {
				t := template.Must(template.New(rel).Parse(text))
				var b strings.Builder
				if err := t.Execute(&b, data); err != nil {
					return err
				}
				p := filepath.Join(dir, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				// #nosec G306 -- project source files, meant to be readable.
				if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
					return err
				}
			}
			fmt.Printf("created %s (%s): address %s.%s\n", dir, lang, app, name)
			switch lang {
			case langGo:
				fmt.Printf("next: (cd %s && go mod tidy) && fcdev fn build %s\n", dir, dir)
			case langTS:
				fmt.Printf("next: (cd %s && npm install) && fcdev fn build %s\n", dir, dir)
			default:
				fmt.Printf("next: fcdev fn build %s\n", dir)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&lang, "lang", langGo, "go, rust or ts")
	cmd.Flags().StringVar(&app, "app", "", "the owning application's code")
	cmd.Flags().StringVar(&sdk, "sdk-path", "", "a local checkout of the SDKs (the repo's clients/ directory) instead of the published ones")
	return cmd
}
