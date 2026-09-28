package budget

import (
	"os"
	"strconv"
	"strings"
)

// unlimitedV1 is the value cgroup v1 reports for "no limit" (a huge number
// just under the maximum page-aligned int64).
const unlimitedV1 = int64(1) << 60

// DetectLimit is the memory the process may use: the cgroup v2 limit, else
// the cgroup v1 limit, else physical RAM. ok is false when none can be read.
func DetectLimit() (limit int64, ok bool) {
	if v, ok := readCgroupV2("memory.max"); ok {
		return v, true
	}
	if v, ok := readInt("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok && v > 0 && v < unlimitedV1 {
		return v, true
	}
	return physicalMemory()
}

// CurrentUsage is the memory the process (or its cgroup) currently uses,
// for the compiled-code watchdog. ok is false where it cannot be read
// cheaply (non-Linux), and the watchdog then stays idle.
func CurrentUsage() (int64, bool) {
	if v, ok := readCgroupV2("memory.current"); ok {
		return v, true
	}
	if v, ok := readInt("/sys/fs/cgroup/memory/memory.usage_in_bytes"); ok {
		return v, true
	}
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * int64(os.Getpagesize()), true
}

// readCgroupV2 reads a single-value file of the process's own cgroup v2
// directory ("max" means unlimited → not ok).
func readCgroupV2(name string) (int64, bool) {
	dir := "/sys/fs/cgroup"
	if b, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		// v2 line: "0::/path"
		for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
			if rest, found := strings.CutPrefix(line, "0::"); found {
				if rest != "/" && rest != "" {
					// #nosec G703 -- the path comes from /proc/self/cgroup (kernel-written) and is read-only.
					if _, err := os.Stat(dir + rest + "/" + name); err == nil {
						dir += rest
					}
				}
				break
			}
		}
	}
	return readInt(dir + "/" + name)
}

func readInt(path string) (int64, bool) {
	b, err := os.ReadFile(path) // #nosec G703 -- fixed /sys and /proc paths, read-only.
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "max" || s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}
