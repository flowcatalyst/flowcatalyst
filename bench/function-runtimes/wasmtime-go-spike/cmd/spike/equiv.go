//go:build wasmtime

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
)

// errPathScript exercises the host-call error frames: a declared key, an
// undeclared one, an unknown op, a log with attributes, and meta that is not
// JSON. Its answer is every result the guest saw, so two engines must agree on
// each frame the host wrote.
var errPathScript = jsScript(`
  const r = [];
  const push = (f) => {
    try {
      const x = f();
      r.push({ code: x.code, meta: x.meta, body: x.body ? __fc_utf8_decode(x.body) : null });
    } catch (e) {
      r.push({ threw: String(e) });
    }
  };
  push(() => __fc_call(2, '{"key":"BENCH"}', undefined));
  push(() => __fc_call(2, '{"key":"NOPE"}', undefined));
  push(() => __fc_call(99, '{}', undefined));
  push(() => __fc_call(1, '{"level":"warn","msg":"m","attrs":{"a":1}}', undefined));
  push(() => __fc_call(2, 'not json', undefined));
  return { meta: '{"status":200,"headers":{"x-n":["5"]}}', body: JSON.stringify(r) };`)

type equivCase struct {
	name   string
	g      guest
	script []byte // JS only: overrides the guest's file
	frame  []byte
	expect []byte // nil: no reference file; compare engines only
	host   bool   // false: no capabilities (nil Host)
	calls  int64  // host calls expected per invocation (-1: do not check)
}

func equiv(ctx context.Context) {
	in := inputs()
	var cs []equivCase
	for _, gl := range strings.Split(*guests, ",") {
		g := guestByLabel(gl)
		for w := 1; w <= 5; w++ {
			calls := int64(0)
			if w == 5 {
				calls = 20
			}
			cs = append(cs, equivCase{name: fmt.Sprintf("%s W%d", gl, w), g: g, frame: in[w].frame, expect: in[w].expect, host: true, calls: calls})
		}
	}
	js := guest{"js-errpath", "js", ""}
	cs = append(cs,
		equivCase{name: "js error frames, with host", g: js, script: errPathScript, frame: metaFrame(`{"w":0}`), host: true, calls: 5},
		equivCase{name: "js error frames, nil host", g: js, script: errPathScript, frame: metaFrame(`{"w":0}`), host: false, calls: 0},
		equivCase{name: "gospike 7 host calls", g: guestByLabel("gospike"), frame: metaFrame(`{"w":12,"n":7}`), expect: []byte("252"), host: true, calls: 7},
	)
	engs := engines()
	rts := map[string]rt{}
	for _, e := range engs {
		rts[e] = makeRT(ctx, e)
	}
	fmt.Printf("%-30s %-10s %-18s %-8s %-6s %s\n", "case", "engine", "frame sha256[:8]", "len", "ref", "host calls/invocation")
	allSame := true
	for _, c := range cs {
		var first []byte
		for _, e := range engs {
			r := rts[e]
			var f fn
			var err error
			if c.script != nil {
				f, err = r.Load(ctx, "js", c.script)
			} else {
				f, err = r.Load(ctx, c.g.kind, mustRead(c.g.path))
			}
			if err != nil {
				panic(err)
			}
			i := newInst(ctx, f)
			h := newHost()
			var frames [][]byte
			for range 3 { // three calls on one instance: the answer must not drift
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				var host engine.Host
				if c.host {
					host = h
				}
				out, err := i.Handle(cctx, c.frame, host)
				cancel()
				if err != nil {
					panic(fmt.Sprintf("%s %s: %v", c.name, e, err))
				}
				frames = append(frames, out)
			}
			i.Close()
			stable := bytes.Equal(frames[0], frames[1]) && bytes.Equal(frames[1], frames[2])
			ref := "-"
			if c.expect != nil {
				ref = "OK"
				if !bytes.Equal(bodyOf(frames[0]), c.expect) {
					ref = "DIFF"
					allSame = false
				}
			}
			perCall := h.calls.Load() / 3
			hc := fmt.Sprintf("%d", perCall)
			if c.calls >= 0 && perCall != c.calls {
				hc += fmt.Sprintf(" (want %d)", c.calls)
				allSame = false
			}
			if !stable {
				hc += " UNSTABLE"
				allSame = false
			}
			fmt.Printf("%-30s %-10s %-18s %-8d %-6s %s\n", c.name, e, sha(frames[0]), len(frames[0]), ref, hc)
			if first == nil {
				first = frames[0]
			} else if !bytes.Equal(first, frames[0]) {
				fmt.Printf("  DIFFERENT FRAME from %s:\n    %s: %.300q\n    %s: %.300q\n", engs[0], engs[0], first, e, frames[0])
				allSame = false
			}
			if strings.HasPrefix(c.name, "js error frames") && e == engs[0] {
				fmt.Printf("  body: %s\n", bodyOf(frames[0]))
			}
		}
	}
	fmt.Printf("RESULT metric=equiv all_identical=%v cases=%d engines=%s\n", allSame, len(cs), strings.Join(engs, ","))
	for _, r := range rts {
		r.Close()
	}
}
