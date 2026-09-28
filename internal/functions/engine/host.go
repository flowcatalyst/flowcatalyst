package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"

	"github.com/tetratelabs/wazero/api"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// Host serves a guest's capability calls for one invocation. meta and body
// are copies the Host may keep. A failure is an *abi.Error, never a panic
// (a panic is recovered and answered as UNAVAILABLE, and logged).
type Host interface {
	Call(ctx context.Context, op abi.Op, meta, body []byte) (respMeta, respBody []byte, err *abi.Error)
}

// HostFunc adapts a function to Host.
type HostFunc func(ctx context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error)

// Call implements Host.
func (f HostFunc) Call(ctx context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error) {
	return f(ctx, op, meta, body)
}

// callState is the per-call context the fc host functions reach through ctx:
// the Host serving this invocation, and the result waiting for fc.take.
type callState struct {
	host    Host
	pending []byte
}

type callStateKey struct{}

func withState(ctx context.Context, host Host) context.Context {
	return context.WithValue(ctx, callStateKey{}, &callState{host: host})
}

// Result codes in the high half of fc.call's return.
const (
	callOK    = 0
	callError = 1
)

func (e *Engine) instantiateHostModule(ctx context.Context) error {
	_, err := e.rt.NewHostModuleBuilder(abi.ImportModule).
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(e.hostCall), []api.ValueType{i32, i32, i32}, []api.ValueType{i64}).
		WithParameterNames("op", "ptr", "len").
		Export(abi.ImportCall).
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(e.hostTake), []api.ValueType{i32}, nil).
		WithParameterNames("dst").
		Export(abi.ImportTake).
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("engine: fc host module: %w", err)
	}
	return nil
}

// hostCall implements fc.call(op, ptr, len) → code<<32 | resultLen.
func (e *Engine) hostCall(ctx context.Context, mod api.Module, stack []uint64) {
	op, ptr, n := abi.Op(api.DecodeU32(stack[0])), api.DecodeU32(stack[1]), api.DecodeU32(stack[2])
	st, _ := ctx.Value(callStateKey{}).(*callState)
	if st == nil {
		// Not inside a call this engine started; nothing to answer through.
		panic("fc.call outside an invocation")
	}
	st.pending = nil
	in, ok := mod.Memory().Read(ptr, n)
	if !ok {
		stack[0] = e.fail(st, abi.Errorf(abi.CodeBadRequest, fmt.Sprintf("frame [%d, +%d) is outside memory", ptr, n)))
		return
	}
	meta, body, err := abi.DecodeFrame(in)
	if err != nil {
		stack[0] = e.fail(st, abi.Errorf(abi.CodeBadRequest, err.Error()))
		return
	}
	if st.host == nil {
		stack[0] = e.fail(st, abi.Errorf(abi.CodeCapabilityUnavailable, op.String()+" is not available here"))
		return
	}
	// Copies: the guest's memory may move or be rewritten while the host works.
	meta, body = append([]byte(nil), meta...), append([]byte(nil), body...)
	rMeta, rBody, cerr := e.serve(ctx, st.host, op, meta, body)
	if cerr != nil {
		stack[0] = e.fail(st, cerr)
		return
	}
	if len(rMeta) == 0 {
		rMeta = []byte("{}")
	}
	st.pending = abi.EncodeFrame(rMeta, rBody)
	stack[0] = abi.Pack(callOK, uint32(len(st.pending)))
}

func (e *Engine) serve(ctx context.Context, h Host, op abi.Op, meta, body []byte) (rMeta, rBody []byte, err *abi.Error) {
	defer func() {
		if p := recover(); p != nil {
			e.log.Error("function host call panicked", "op", op.String(), "panic", p, "stack", string(debug.Stack()))
			rMeta, rBody, err = nil, nil, abi.Errorf(abi.CodeUnavailable, "the host failed serving "+op.String())
		}
	}()
	return h.Call(ctx, op, meta, body)
}

func (e *Engine) fail(st *callState, cerr *abi.Error) uint64 {
	meta, _ := json.Marshal(cerr)
	st.pending = abi.EncodeFrame(meta, nil)
	return abi.Pack(callError, uint32(len(st.pending)))
}

// hostTake implements fc.take(dst): copy the pending result into guest memory.
func (e *Engine) hostTake(ctx context.Context, mod api.Module, stack []uint64) {
	dst := api.DecodeU32(stack[0])
	st, _ := ctx.Value(callStateKey{}).(*callState)
	if st == nil || st.pending == nil {
		panic("fc.take with no pending result")
	}
	if !mod.Memory().Write(dst, st.pending) {
		panic(fmt.Sprintf("fc.take destination [%d, +%d) is outside memory", dst, len(st.pending)))
	}
	st.pending = nil
}
