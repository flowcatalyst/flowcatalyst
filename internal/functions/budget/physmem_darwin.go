//go:build darwin

package budget

import "golang.org/x/sys/unix"

func physicalMemory() (int64, bool) {
	v, err := unix.SysctlUint64("hw.memsize")
	if err != nil || v == 0 {
		return 0, false
	}
	return int64(v), true
}
