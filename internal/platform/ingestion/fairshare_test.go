package ingestion

import "testing"

func TestFairShare(t *testing.T) {
	for _, c := range []struct{ polled, runtimes, want int }{
		{0, 1, 0},
		{1, 1, 1},
		{7, 1, 7},
		{7, 2, 4},
		{8, 2, 4},
		{1, 3, 1},
		{3, 3, 1},
		{4, 3, 2},
		{10, 4, 3},
		{5, 0, 5}, // no live runtime counted (cannot happen: the asker is one) is treated as one
	} {
		if got := FairShare(c.polled, c.runtimes); got != c.want {
			t.Errorf("FairShare(%d, %d) = %d, want %d", c.polled, c.runtimes, got, c.want)
		}
	}
}
