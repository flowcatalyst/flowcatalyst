package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// newFnCmd is `fcdev fn`: build, check, publish and run functions
// (docs/function-runner-plan.md §10).
func newFnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fn",
		Short: "Build, publish and run FlowCatalyst functions",
	}
	cmd.AddCommand(newFnInitCmd(), newFnBuildCmd(), newFnDescribeCmd(), newFnRunCmd())
	addFnPlatformCmds(cmd)
	return cmd
}

// Languages a project can be in, detected from its files.
const (
	langGo   = "go"
	langRust = "rust"
	langTS   = "ts"
)

// detectLang reports a project directory's language.
func detectLang(dir string) (string, error) {
	switch {
	case fileExists(filepath.Join(dir, "Cargo.toml")):
		return langRust, nil
	case fileExists(filepath.Join(dir, "package.json")):
		return langTS, nil
	case fileExists(filepath.Join(dir, "go.mod")) || hasGoFiles(dir):
		return langGo, nil
	}
	return "", fmt.Errorf("%s: no go.mod, Cargo.toml or package.json — not a function project", dir)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func hasGoFiles(dir string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return len(m) > 0
}

// runtimeFor maps an artifact path to its runtime: a .js file is a script
// for the shared JS engine, anything else a module.
func runtimeFor(path string) string {
	if strings.HasSuffix(path, ".js") {
		return runtimes.JS
	}
	return runtimes.Wasm
}

// buildProject builds the project in dir with its own toolchain and returns
// the artifact's path.
func buildProject(ctx context.Context, dir string) (string, error) {
	lang, err := detectLang(dir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	var out string
	switch lang {
	case langGo:
		out = filepath.Join(abs, "function.wasm")
		// #nosec G204 -- a fixed go build; out is a path this command chose.
		cmd = exec.CommandContext(ctx, "go", "build", "-buildmode=c-shared", "-ldflags=-s -w", "-o", out, ".")
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	case langRust:
		cmd = exec.CommandContext(ctx, "cargo", "build", "--release", "--target", "wasm32-unknown-unknown")
	case langTS:
		out = filepath.Join(abs, "dist", "function.js")
		cmd = exec.CommandContext(ctx, "npm", "run", "build")
	}
	cmd.Dir = abs
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s build failed: %w", lang, err)
	}
	if lang == langRust {
		out, err = rustArtifact(abs)
		if err != nil {
			return "", err
		}
	}
	if !fileExists(out) {
		return "", fmt.Errorf("the build did not produce %s", out)
	}
	return out, nil
}

// rustArtifact finds the cdylib cargo built for wasm32-unknown-unknown.
func rustArtifact(dir string) (string, error) {
	m, _ := filepath.Glob(filepath.Join(dir, "target", "wasm32-unknown-unknown", "release", "*.wasm"))
	if len(m) != 1 {
		return "", fmt.Errorf("expected one .wasm under target/wasm32-unknown-unknown/release, found %d", len(m))
	}
	return m[0], nil
}

// describeArtifact reads and validates an artifact's describe document the
// way publish will (plan §5.4): an instance with no capabilities.
func describeArtifact(ctx context.Context, path, runtime string) (*abi.Describe, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	b, err := budget.New(512<<20, 0)
	if err != nil {
		return nil, nil, err
	}
	e, err := engine.New(ctx, engine.Config{Budget: b})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = e.Close(context.Background()) }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	doc, err := runtimes.NewLoader(e).Describe(ctx, runtime, data, 256<<20)
	if err != nil {
		return nil, nil, err
	}
	d, err := abi.ParseDescribe(doc)
	if err != nil {
		return nil, doc, err
	}
	return d, doc, nil
}

func printDescribe(path string, d *abi.Describe) {
	fmt.Printf("%s\n", path)
	for _, e := range d.Endpoints {
		fmt.Printf("  endpoint     %-28s auth=%s\n", e.Pattern(), e.Auth)
	}
	for _, s := range d.Subscriptions {
		fmt.Printf("  subscription %-28s → %s\n", s.EventType, s.Path)
	}
	for _, s := range d.Schedules {
		fmt.Printf("  schedule     %-28s → %s\n", s.Cron, s.Path)
	}
	list := func(label string, v []string) {
		if len(v) > 0 {
			fmt.Printf("  %-12s %s\n", label, strings.Join(v, ", "))
		}
	}
	list("config", d.Config)
	list("secrets", d.Secrets)
	list("db", d.DB)
	list("httpAllow", d.HTTPAllow)
	list("emits", d.Emits)
}

func newFnBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build [dir]",
		Short: "Build a function project and check its describe document",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			out, err := buildProject(cmd.Context(), dir)
			if err != nil {
				return err
			}
			d, _, err := describeArtifact(cmd.Context(), out, runtimeFor(out))
			if err != nil {
				return fmt.Errorf("%s built, but its describe document is not publishable: %w", out, err)
			}
			info, _ := os.Stat(out)
			printDescribe(fmt.Sprintf("%s (%d KB)", out, info.Size()/1024), d)
			return nil
		},
	}
}

func newFnDescribeCmd() *cobra.Command {
	var runtime string
	var raw bool
	cmd := &cobra.Command{
		Use:   "describe <artifact>",
		Short: "Print and validate an artifact's describe document, as publish reads it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime == "" {
				runtime = runtimeFor(args[0])
			}
			d, doc, err := describeArtifact(cmd.Context(), args[0], runtime)
			if raw && doc != nil {
				var v any
				if json.Unmarshal(doc, &v) == nil {
					pretty, _ := json.MarshalIndent(v, "", "  ")
					fmt.Println(string(pretty))
				}
			}
			if err != nil {
				if de, ok := errors.AsType[*abi.DescribeError](err); ok {
					for _, p := range de.Problems {
						fmt.Fprintln(os.Stderr, "  - "+p)
					}
				}
				return err
			}
			if !raw {
				printDescribe(args[0], d)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&runtime, "runtime", "", "wasm or js (default: from the file extension)")
	cmd.Flags().BoolVar(&raw, "json", false, "print the raw describe document")
	return cmd
}

// addFnPlatformCmds adds the commands that talk to a platform (publish,
// promote, deploy, invoke, status, watch) — see fn_platform.go.
var addFnPlatformCmds = func(*cobra.Command) {}
