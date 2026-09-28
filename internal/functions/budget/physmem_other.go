//go:build !darwin

package budget

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
)

func physicalMemory() (int64, bool) { return memTotal() }

// memTotal parses /proc/meminfo's MemTotal (kB).
func memTotal() (int64, bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if rest, found := strings.CutPrefix(sc.Text(), "MemTotal:"); found {
			f := strings.Fields(rest)
			if len(f) == 0 {
				return 0, false
			}
			kb, err := strconv.ParseInt(f[0], 10, 64)
			if err != nil {
				return 0, false
			}
			return kb * 1024, true
		}
	}
	return 0, false
}
