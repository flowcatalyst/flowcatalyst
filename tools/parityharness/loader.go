package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// parityCase is one entry from a *.yaml file under -dir.
type parityCase struct {
	// File is the source path; used to make error messages locate-able.
	File string `yaml:"-"`

	Name    string      `yaml:"name"`
	Request requestSpec `yaml:"request"`
	Expect  expectSpec  `yaml:"expect"`
}

type requestSpec struct {
	Method  string            `yaml:"method"`
	Path    string            `yaml:"path"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

type expectSpec struct {
	// Status, if set, must match between Rust and Go and equal this value.
	Status int `yaml:"status"`
	// BodyShape is a placeholder-typed structural description. Leaf
	// strings can be exact values OR placeholder type names
	// (`tsid`, `uuid`, `iso8601-microsecond`, `any-string`, etc.).
	// Optional — if absent, the comparator just diffs Rust vs Go bodies
	// directly.
	BodyShape any `yaml:"body_shape"`
}

// loadCases walks dir recursively, parses every *.yaml as a parityCase,
// and returns them ordered by File path so runs are reproducible.
func loadCases(dir string) ([]parityCase, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var cases []parityCase
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		c, err := loadCase(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		cases = append(cases, c)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return cases, nil
}

func loadCase(path string) (parityCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return parityCase{}, err
	}
	var c parityCase
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return parityCase{}, fmt.Errorf("parse yaml: %w", err)
	}
	c.File = path
	if c.Name == "" {
		// Fall back to the file's relative path for diagnostics if the
		// author forgot to set a name.
		c.Name = filepath.Base(path)
	}
	if c.Request.Method == "" {
		return c, fmt.Errorf("request.method is required")
	}
	if c.Request.Path == "" {
		return c, fmt.Errorf("request.path is required")
	}
	return c, nil
}

// substituteVars replaces every `${VAR}` in s with os.Getenv("VAR").
// Returns the substituted string and the names of any unset vars so
// the runner can skip cases that lack a required value (e.g. a bearer
// token) rather than fail them.
func substituteVars(s string) (substituted string, missing []string) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	out := varRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1] // strip ${ and }
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return match
		}
		return v
	})
	return out, missing
}

// varRefPattern matches `${NAME}` with NAME being [A-Za-z_][A-Za-z0-9_]*.
var varRefPattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
