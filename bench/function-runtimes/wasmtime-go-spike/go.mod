module github.com/flowcatalyst/flowcatalyst-go/bench/function-runtimes/wasmtime-go-spike

go 1.27

require (
	github.com/bytecodealliance/wasmtime-go/v50 v50.0.0
	github.com/flowcatalyst/flowcatalyst-go v0.0.0-00010101000000-000000000000
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace (
	github.com/flowcatalyst/flowcatalyst-go => ../../..
	github.com/flowcatalyst/go-sdk => ../../../pkg/fcsdk
)
