package schemacodegen

// The goldens under testdata/golden are produced by running the original
// TypeScript sources with node; regenerate them after changing the inputs in
// testdata/schemas or testdata/export (needs node >= 22.18 and
// frontend/node_modules).
//
//go:generate node testdata/gen/gen.mjs
