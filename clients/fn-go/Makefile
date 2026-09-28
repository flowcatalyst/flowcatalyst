# clients/fn-go — Go guest SDK for FlowCatalyst functions.
#
# The SDK package itself (fn) is plain Go with zero third-party
# dependencies; only the example needs to be cross-compiled to wasm.

.PHONY: test vet build-hello clean

test:
	go test ./...

vet:
	go vet ./...

# build-hello cross-compiles examples/hello to a wasip1 reactor module. The
# guest ABI (plan §5.1) requires fc_alloc/fc_handle/fc_describe/fc_abi_v1
# exports plus a `memory` export; -buildmode=c-shared with GOOS=wasip1
# produces exactly that (Go's wasip1 toolchain also emits the optional
# _initialize export automatically).
build-hello:
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
		-o examples/hello/hello.wasm ./examples/hello

clean:
	rm -f examples/hello/hello.wasm examples/hello/hello.h
