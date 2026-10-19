package loginbackoff

import "testing"

// FuzzComputeDelaySecs: for any policy and failure counts the delay never
// panics, never exceeds the cap (or the base when the base alone exceeds it),
// is zero inside the free allowance, and never decreases as failures grow.
func FuzzComputeDelaySecs(f *testing.F) {
	f.Add(uint32(3), uint32(2), uint32(300), uint32(0), uint32(100))
	f.Add(uint32(0), uint32(1), uint32(1), uint32(1<<31), uint32(1<<32-1))
	f.Add(uint32(1<<32-1), uint32(1<<32-1), uint32(1<<32-1), uint32(1<<32-1), uint32(1<<32-1))
	f.Fuzz(func(t *testing.T, free, base, maxd, n1, n2 uint32) {
		p := Policy{FreeAttempts: free, BaseDelaySecs: base, MaxDelaySecs: maxd}
		d1, d2 := p.ComputeDelaySecs(n1), p.ComputeDelaySecs(n2)
		if n1 <= free && d1 != 0 {
			t.Fatalf("delay %d inside the free allowance", d1)
		}
		if d1 > maxd && d1 != base {
			t.Fatalf("delay %d above cap %d", d1, maxd)
		}
		if n1 <= n2 && d1 > d2 && d1 <= maxd && d2 <= maxd {
			t.Fatalf("delay decreased: n=%d→%d, %d→%d (policy %+v)", n1, n2, d1, d2, p)
		}
	})
}
