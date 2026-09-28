package engine

import (
	"fmt"
	"slices"

	"github.com/tetratelabs/wazero/api"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// Load failure codes, reported in heartbeats as LOAD:<code>.
const (
	LoadInvalidModule    = "INVALID_MODULE"
	LoadABIUnknown       = "ABI_UNKNOWN"
	LoadExportMissing    = "EXPORT_MISSING"
	LoadExportSignature  = "EXPORT_SIGNATURE"
	LoadImportNotAllowed = "IMPORT_NOT_ALLOWED"
	LoadMemoryOverCap    = "MEMORY_OVER_CAP"
	LoadInstantiate      = "INSTANTIATE"
)

// LoadError is why a module cannot be loaded.
type LoadError struct {
	Code   string
	Detail string
}

func (e *LoadError) Error() string { return "LOAD:" + e.Code + ": " + e.Detail }

var (
	i32 = api.ValueTypeI32
	i64 = api.ValueTypeI64
)

type signature struct{ params, results []api.ValueType }

var requiredExports = map[string]signature{
	abi.ExportMarker:   {nil, nil},
	abi.ExportAlloc:    {[]api.ValueType{i32}, []api.ValueType{i32}},
	abi.ExportHandle:   {[]api.ValueType{i32, i32}, []api.ValueType{i64}},
	abi.ExportDescribe: {nil, []api.ValueType{i64}},
}

var fcImports = map[string]signature{
	abi.ImportCall: {[]api.ValueType{i32, i32, i32}, []api.ValueType{i64}},
	abi.ImportTake: {[]api.ValueType{i32}, nil},
}

// check enforces ABI v1's module contract: required exports with the right
// signatures, one exported memory and no imported one, and imports only from
// `fc` (call/take) and WASI preview 1.
func (m *Module) check() error {
	exports := m.cm.ExportedFunctions()
	if _, ok := exports[abi.ExportMarker]; !ok {
		return &LoadError{Code: LoadABIUnknown, Detail: "the module does not export " + abi.ExportMarker}
	}
	for name, want := range requiredExports {
		def, ok := exports[name]
		if !ok {
			return &LoadError{Code: LoadExportMissing, Detail: "the module does not export " + name}
		}
		if !sameSignature(def, want) {
			return &LoadError{Code: LoadExportSignature, Detail: fmt.Sprintf("%s is %v→%v, want %v→%v", name, def.ParamTypes(), def.ResultTypes(), want.params, want.results)}
		}
	}
	if def, ok := exports[abi.ExportInitialize]; ok {
		if !sameSignature(def, signature{}) {
			return &LoadError{Code: LoadExportSignature, Detail: abi.ExportInitialize + " must take and return nothing"}
		}
		m.hasInit = true
	}
	for _, def := range m.cm.ImportedFunctions() {
		mod, name, _ := def.Import()
		switch mod {
		case abi.WASIModule:
		case abi.ImportModule:
			want, ok := fcImports[name]
			if !ok || !sameSignature(def, want) {
				return &LoadError{Code: LoadImportNotAllowed, Detail: fmt.Sprintf("%s.%s is not an fc import of ABI v1", mod, name)}
			}
		default:
			return &LoadError{Code: LoadImportNotAllowed, Detail: fmt.Sprintf("imports %s.%s; only %s and %s are allowed", mod, name, abi.ImportModule, abi.WASIModule)}
		}
	}
	if len(m.cm.ImportedMemories()) > 0 {
		return &LoadError{Code: LoadImportNotAllowed, Detail: "the module imports a memory; it must define its own"}
	}
	mem, ok := m.cm.ExportedMemories()[abi.ExportMemory]
	if !ok {
		return &LoadError{Code: LoadExportMissing, Detail: "the module does not export its memory as " + abi.ExportMemory}
	}
	const page = 1 << 16
	m.minBytes = uint64(mem.Min()) * page
	m.maxBytes = uint64(maxPages) * page
	if mx, ok := mem.Max(); ok {
		m.maxBytes = uint64(mx) * page
	}
	return nil
}

func sameSignature(def api.FunctionDefinition, want signature) bool {
	return slices.Equal(def.ParamTypes(), want.params) && slices.Equal(def.ResultTypes(), want.results)
}
