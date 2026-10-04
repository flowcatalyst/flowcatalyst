// Package runtimetune adjusts Go runtime settings once at start-up from the
// container's cgroup limits and the roles this process runs.
//
// Two problems are addressed:
//
//   - Go's container-aware GOMAXPROCS default never goes below 2, so a process
//     limited to one CPU of quota runs two scheduler threads on it and wastes
//     throughput on context switches. With a quota of at most 1.0 CPU and no
//     explicit GOMAXPROCS, it is set to 1.
//   - The router and the dispatch-job scheduler allocate heavily and trade
//     memory for throughput well, so when either runs and GOGC is not set the
//     GC target is raised (default 200). Because that lets the heap grow, a
//     soft memory limit of 90% of the cgroup memory limit is set as well
//     (unless GOMEMLIMIT is set) so the collector tightens before the
//     container is OOM-killed.
//
// Everything decision-shaped is a pure function over file contents and env
// lookups; Apply is the thin layer that reads the files and calls runtime.
package runtimetune

import (
	"io/fs"
	"log/slog"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// Env variable names.
const (
	EnvGOMAXPROCS = "GOMAXPROCS"
	EnvGOGC       = "GOGC"
	EnvGOMEMLIMIT = "GOMEMLIMIT"
	// EnvGCPercent overrides the GC target this package would pick.
	EnvGCPercent = "FC_GC_PERCENT"
)

const (
	// DefaultGCPercent is the GC target used when a heavy role is enabled.
	DefaultGCPercent = 200
	// memLimitFraction of the cgroup memory limit becomes the soft limit.
	memLimitFraction = 0.9
	// cgroupRoot is where cgroup files are looked up.
	cgroupRoot = "/sys/fs/cgroup"
)

// Lookup is os.LookupEnv-shaped.
type Lookup func(string) (string, bool)

func isSet(lookup Lookup, name string) bool {
	v, ok := lookup(name)
	return ok && strings.TrimSpace(v) != ""
}

// ---- cgroup parsing -------------------------------------------------------

// ParseCPUMaxV2 parses cgroup v2 cpu.max ("<quota|max> <period>") into a CPU
// count. ok is false for "max" (no limit) and for malformed content.
func ParseCPUMaxV2(content string) (cpus float64, ok bool) {
	f := strings.Fields(content)
	if len(f) < 1 || len(f) > 2 || f[0] == "max" {
		return 0, false
	}
	period := "100000" // kernel default when the period is omitted
	if len(f) == 2 {
		period = f[1]
	}
	return ratio(f[0], period)
}

// ParseCPUV1 parses cgroup v1 cpu.cfs_quota_us / cpu.cfs_period_us. A quota
// of -1 means no limit.
func ParseCPUV1(quota, period string) (cpus float64, ok bool) {
	return ratio(strings.TrimSpace(quota), strings.TrimSpace(period))
}

func ratio(quota, period string) (float64, bool) {
	q, err := strconv.ParseInt(quota, 10, 64)
	if err != nil || q <= 0 {
		return 0, false
	}
	p, err := strconv.ParseInt(period, 10, 64)
	if err != nil || p <= 0 {
		return 0, false
	}
	return float64(q) / float64(p), true
}

// ParseMemory parses a cgroup memory limit file (v2 memory.max, v1
// memory.limit_in_bytes). "max" and the v1 "unlimited" sentinel (a value near
// the int64 ceiling) mean no limit.
func ParseMemory(content string) (bytes int64, ok bool) {
	s := strings.TrimSpace(content)
	if s == "" || s == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n >= 1<<60 {
		return 0, false
	}
	return n, true
}

func readFile(fsys fs.FS, name string) (string, bool) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// CPULimit finds the cgroup CPU limit under fsys (rooted at /sys/fs/cgroup),
// trying v2 first, then the v1 layouts.
func CPULimit(fsys fs.FS) (cpus float64, ok bool) {
	if c, found := readFile(fsys, "cpu.max"); found {
		return ParseCPUMaxV2(c)
	}
	for _, dir := range []string{"cpu", "cpu,cpuacct"} {
		q, qok := readFile(fsys, dir+"/cpu.cfs_quota_us")
		p, pok := readFile(fsys, dir+"/cpu.cfs_period_us")
		if qok && pok {
			return ParseCPUV1(q, p)
		}
	}
	return 0, false
}

// MemoryLimit finds the cgroup memory limit under fsys, v2 then v1.
func MemoryLimit(fsys fs.FS) (bytes int64, ok bool) {
	for _, name := range []string{"memory.max", "memory/memory.limit_in_bytes"} {
		if c, found := readFile(fsys, name); found {
			return ParseMemory(c)
		}
	}
	return 0, false
}

// ---- decisions ------------------------------------------------------------

// DecideGOMAXPROCS returns (1, true) when GOMAXPROCS is not set in the
// environment and the CPU limit is known and at most 1.0; otherwise Go's
// default is left alone.
func DecideGOMAXPROCS(lookup Lookup, cpus float64, limited bool) (n int, apply bool) {
	if isSet(lookup, EnvGOMAXPROCS) || !limited || cpus > 1.0 {
		return 0, false
	}
	return 1, true
}

// DecideGC returns the GC percent to set. GOGC in the environment always wins
// (apply=false). FC_GC_PERCENT: a positive integer is used as given (and
// applied whether or not a heavy role runs); "off" disables the GC target
// (-1; only the memory limit then triggers collection); anything else
// (including 0 and negatives) is a bad value: warn is non-empty and the
// default applies when a heavy role is enabled. With FC_GC_PERCENT unset the
// result is DefaultGCPercent when heavyRole, else untouched.
func DecideGC(lookup Lookup, heavyRole bool) (percent int, apply bool, warn string) {
	if isSet(lookup, EnvGOGC) {
		return 0, false, ""
	}
	if isSet(lookup, EnvGCPercent) {
		v, _ := lookup(EnvGCPercent)
		v = strings.TrimSpace(v)
		if strings.EqualFold(v, "off") {
			return -1, true, ""
		}
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n, true, ""
		}
		warn = EnvGCPercent + "=" + strconv.Quote(v) + " is not a positive integer or \"off\"; using the default"
	}
	if heavyRole {
		return DefaultGCPercent, true, warn
	}
	return 0, false, warn
}

// DecideMemoryLimit returns 90% of the cgroup memory limit when GOMEMLIMIT is
// not set and a limit exists.
func DecideMemoryLimit(lookup Lookup, cgroupBytes int64, limited bool) (limit int64, apply bool) {
	if isSet(lookup, EnvGOMEMLIMIT) || !limited || cgroupBytes <= 0 {
		return 0, false
	}
	return int64(math.Floor(float64(cgroupBytes) * memLimitFraction)), true
}

// ---- apply ----------------------------------------------------------------

// Result records what Apply decided and did.
type Result struct {
	CPULimit      float64 // 0 when none/unknown
	GOMAXPROCS    int     // value in effect after Apply
	GOMAXPROCSSet bool
	GCPercent     int
	GCPercentSet  bool
	MemLimit      int64
	MemLimitSet   bool
}

// Apply reads the cgroup limits under fsys, makes the decisions and applies
// them to the runtime. heavyRole is true when the router or the dispatch-job
// scheduler runs in this process. The soft memory limit is only set when the
// GC target was raised or overridden here, since that is what lets the heap
// grow.
func Apply(fsys fs.FS, lookup Lookup, heavyRole bool) Result {
	var r Result

	cpus, limited := CPULimit(fsys)
	if limited {
		r.CPULimit = cpus
	}
	if n, ok := DecideGOMAXPROCS(lookup, cpus, limited); ok {
		runtime.GOMAXPROCS(n)
		r.GOMAXPROCSSet = true
	}
	r.GOMAXPROCS = runtime.GOMAXPROCS(0)

	pct, ok, warn := DecideGC(lookup, heavyRole)
	if warn != "" {
		slog.Warn("runtimetune: " + warn)
	}
	if ok {
		debug.SetGCPercent(pct)
		r.GCPercent, r.GCPercentSet = pct, true

		mem, mok := MemoryLimit(fsys)
		if lim, apply := DecideMemoryLimit(lookup, mem, mok); apply {
			debug.SetMemoryLimit(lim)
			r.MemLimit, r.MemLimitSet = lim, true
		}
	}

	slog.Info("runtime tuning",
		"cgroup_cpu_limit", r.CPULimit,
		"gomaxprocs", r.GOMAXPROCS,
		"gomaxprocs_overridden", r.GOMAXPROCSSet,
		"gc_percent", r.GCPercent,
		"gc_percent_overridden", r.GCPercentSet,
		"memory_limit_bytes", r.MemLimit,
		"memory_limit_overridden", r.MemLimitSet,
		"heavy_role", heavyRole,
	)
	return r
}

// ApplyFromEnv is Apply against the real /sys/fs/cgroup and process env.
func ApplyFromEnv(heavyRole bool) Result {
	return Apply(os.DirFS(cgroupRoot), os.LookupEnv, heavyRole)
}
