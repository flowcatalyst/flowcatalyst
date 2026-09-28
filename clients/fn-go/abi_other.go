//go:build !wasip1

package fn

// This file wires a settable fake host for every non-wasip1 build so the
// package compiles, vets and runs under a normal `go test`/`go vet` on the
// host OS. abi_wasip1.go replaces it with the real `fc` import glue when
// cross-compiled with GOOS=wasip1.

// FakeHost is a test double for the guest<->host ABI boundary. It lets SDK
// logic (config/secret/http/emit/db/log wrappers, dispatch) run against
// scripted host responses without a real wasm runtime.
type FakeHost struct {
	// Handler is invoked for every host call the SDK issues. If nil, every
	// call succeeds with an empty result (as ops like `log` do on the real
	// ABI). Tests set Handler to script per-op responses or errors.
	Handler func(op int32, meta, body []byte) (respMeta, respBody []byte, hostErr *Error, err error)
	// Calls records every call made to this host, in order, for assertions.
	Calls []FakeCall
}

// FakeCall is one recorded call made against a FakeHost.
type FakeCall struct {
	Op   int32
	Meta []byte
	Body []byte
}

func (h *FakeHost) call(op opCode, meta, body []byte) (respMeta, respBody []byte, hostErr *Error, err error) {
	h.Calls = append(h.Calls, FakeCall{
		Op:   int32(op),
		Meta: append([]byte(nil), meta...),
		Body: append([]byte(nil), body...),
	})
	if h.Handler == nil {
		return nil, nil, nil, nil
	}
	return h.Handler(int32(op), meta, body)
}

// SetHost installs h as the package's active host and returns a restore
// func that puts the previous host back. Tests use it to script capability
// behaviour for the duration of a single test:
//
//	fake := &fn.FakeHost{Handler: ...}
//	defer fn.SetHost(fake)()
func SetHost(h *FakeHost) (restore func()) {
	prev := currentHost
	currentHost = h
	return func() { currentHost = prev }
}

func init() {
	currentHost = &FakeHost{}
}
