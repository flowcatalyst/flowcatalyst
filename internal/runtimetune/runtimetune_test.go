package runtimetune

import (
	"runtime"
	"runtime/debug"
	"testing"
	"testing/fstest"
)

func env(kv ...string) Lookup {
	m := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestParseCPUMaxV2(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"100000 100000\n", 1, true},
		{"50000 100000", 0.5, true},
		{"150000 100000", 1.5, true},
		{"200000 100000", 2, true},
		{"max 100000", 0, false},
		{"max", 0, false},
		{"", 0, false},
		{"abc 100000", 0, false},
		{"100000 0", 0, false},
		{"100000 100000 7", 0, false},
		{"100000", 1, true},
	}
	for _, c := range cases {
		got, ok := ParseCPUMaxV2(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: got (%v,%v) want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseCPUV1(t *testing.T) {
	if got, ok := ParseCPUV1("50000\n", "100000\n"); !ok || got != 0.5 {
		t.Errorf("fractional: %v %v", got, ok)
	}
	if _, ok := ParseCPUV1("-1", "100000"); ok {
		t.Error("-1 must mean unlimited")
	}
	if _, ok := ParseCPUV1("x", "100000"); ok {
		t.Error("malformed quota")
	}
	if _, ok := ParseCPUV1("100000", ""); ok {
		t.Error("malformed period")
	}
}

func TestParseMemory(t *testing.T) {
	if n, ok := ParseMemory("536870912\n"); !ok || n != 536870912 {
		t.Errorf("limited: %v %v", n, ok)
	}
	for _, s := range []string{"max", "max\n", "", "junk", "-5", "0", "9223372036854771712"} {
		if _, ok := ParseMemory(s); ok {
			t.Errorf("%q should be no limit", s)
		}
	}
}

func TestCPULimitFS(t *testing.T) {
	v2 := fstest.MapFS{"cpu.max": {Data: []byte("100000 100000\n")}}
	if c, ok := CPULimit(v2); !ok || c != 1 {
		t.Errorf("v2: %v %v", c, ok)
	}
	v2max := fstest.MapFS{"cpu.max": {Data: []byte("max 100000\n")}}
	if _, ok := CPULimit(v2max); ok {
		t.Error("v2 max")
	}
	for _, dir := range []string{"cpu", "cpu,cpuacct"} {
		v1 := fstest.MapFS{
			dir + "/cpu.cfs_quota_us":  {Data: []byte("50000\n")},
			dir + "/cpu.cfs_period_us": {Data: []byte("100000\n")},
		}
		if c, ok := CPULimit(v1); !ok || c != 0.5 {
			t.Errorf("v1 %s: %v %v", dir, c, ok)
		}
	}
	v1un := fstest.MapFS{
		"cpu/cpu.cfs_quota_us":  {Data: []byte("-1\n")},
		"cpu/cpu.cfs_period_us": {Data: []byte("100000\n")},
	}
	if _, ok := CPULimit(v1un); ok {
		t.Error("v1 unlimited")
	}
	if _, ok := CPULimit(fstest.MapFS{}); ok {
		t.Error("missing files")
	}
	bad := fstest.MapFS{"cpu.max": {Data: []byte("garbage")}}
	if _, ok := CPULimit(bad); ok {
		t.Error("malformed")
	}
}

func TestMemoryLimitFS(t *testing.T) {
	v2 := fstest.MapFS{"memory.max": {Data: []byte("1073741824\n")}}
	if n, ok := MemoryLimit(v2); !ok || n != 1073741824 {
		t.Errorf("v2: %v %v", n, ok)
	}
	v1 := fstest.MapFS{"memory/memory.limit_in_bytes": {Data: []byte("1073741824\n")}}
	if n, ok := MemoryLimit(v1); !ok || n != 1073741824 {
		t.Errorf("v1: %v %v", n, ok)
	}
	v1un := fstest.MapFS{"memory/memory.limit_in_bytes": {Data: []byte("9223372036854771712\n")}}
	if _, ok := MemoryLimit(v1un); ok {
		t.Error("v1 unlimited")
	}
	if _, ok := MemoryLimit(fstest.MapFS{"memory.max": {Data: []byte("max\n")}}); ok {
		t.Error("v2 max")
	}
	if _, ok := MemoryLimit(fstest.MapFS{}); ok {
		t.Error("missing")
	}
}

func TestDecideGOMAXPROCS(t *testing.T) {
	cases := []struct {
		name    string
		lookup  Lookup
		cpus    float64
		limited bool
		apply   bool
	}{
		{"env set", env("GOMAXPROCS", "4"), 1, true, false},
		{"limit 1", env(), 1, true, true},
		{"limit 0.5", env(), 0.5, true, true},
		{"limit 1.5", env(), 1.5, true, false},
		{"limit 2", env(), 2, true, false},
		{"no limit", env(), 0, false, false},
		{"empty env counts as unset", env("GOMAXPROCS", ""), 1, true, true},
	}
	for _, c := range cases {
		n, apply := DecideGOMAXPROCS(c.lookup, c.cpus, c.limited)
		if apply != c.apply || (apply && n != 1) {
			t.Errorf("%s: got (%d,%v)", c.name, n, apply)
		}
	}
}

func TestDecideGC(t *testing.T) {
	cases := []struct {
		name  string
		l     Lookup
		heavy bool
		pct   int
		apply bool
		warn  bool
	}{
		{"GOGC set", env("GOGC", "100"), true, 0, false, false},
		{"GOGC set beats override", env("GOGC", "100", "FC_GC_PERCENT", "300"), true, 0, false, false},
		{"no role", env(), false, 0, false, false},
		{"heavy role", env(), true, 200, true, false},
		{"override", env("FC_GC_PERCENT", "300"), true, 300, true, false},
		{"override without role", env("FC_GC_PERCENT", "150"), false, 150, true, false},
		{"off", env("FC_GC_PERCENT", "off"), true, -1, true, false},
		{"bad value with role", env("FC_GC_PERCENT", "lots"), true, 200, true, true},
		{"zero is bad", env("FC_GC_PERCENT", "0"), true, 200, true, true},
		{"bad value without role", env("FC_GC_PERCENT", "-3"), false, 0, false, true},
	}
	for _, c := range cases {
		pct, apply, warn := DecideGC(c.l, c.heavy)
		if pct != c.pct || apply != c.apply || (warn != "") != c.warn {
			t.Errorf("%s: got (%d,%v,%q)", c.name, pct, apply, warn)
		}
	}
}

func TestDecideMemoryLimit(t *testing.T) {
	if _, ok := DecideMemoryLimit(env("GOMEMLIMIT", "1GiB"), 1000, true); ok {
		t.Error("GOMEMLIMIT set")
	}
	if _, ok := DecideMemoryLimit(env(), 0, false); ok {
		t.Error("no limit")
	}
	if n, ok := DecideMemoryLimit(env(), 1000, true); !ok || n != 900 {
		t.Errorf("90%%: %d %v", n, ok)
	}
}

// restore puts the global runtime settings back after a test that applies.
func restore(t *testing.T) {
	t.Helper()
	procs := runtime.GOMAXPROCS(0)
	gc := debug.SetGCPercent(100)
	debug.SetGCPercent(gc)
	mem := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(procs)
		debug.SetGCPercent(gc)
		debug.SetMemoryLimit(mem)
	})
}

func TestApply(t *testing.T) {
	restore(t)
	fsys := fstest.MapFS{
		"cpu.max":    {Data: []byte("100000 100000\n")},
		"memory.max": {Data: []byte("1000000000\n")},
	}
	r := Apply(fsys, env(), true)
	if !r.GOMAXPROCSSet || runtime.GOMAXPROCS(0) != 1 {
		t.Errorf("GOMAXPROCS not applied: %+v", r)
	}
	if !r.GCPercentSet || r.GCPercent != 200 {
		t.Errorf("GC not applied: %+v", r)
	}
	if !r.MemLimitSet || r.MemLimit != 900000000 || debug.SetMemoryLimit(-1) != 900000000 {
		t.Errorf("memory limit not applied: %+v", r)
	}
}

func TestApplyNoRoleLeavesGCAndMemory(t *testing.T) {
	restore(t)
	before := debug.SetMemoryLimit(-1)
	fsys := fstest.MapFS{"memory.max": {Data: []byte("1000000000\n")}}
	r := Apply(fsys, env(), false)
	if r.GCPercentSet || r.MemLimitSet || r.GOMAXPROCSSet {
		t.Errorf("nothing should be applied: %+v", r)
	}
	if debug.SetMemoryLimit(-1) != before {
		t.Error("memory limit changed")
	}
}
