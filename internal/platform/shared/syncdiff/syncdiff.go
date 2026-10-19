// Package syncdiff holds the value comparisons the definition syncs use to
// tell a row they would rewrite with different content from one they would
// rewrite with the same content (F30): an unchanged row is neither saved nor
// counted nor audited.
package syncdiff

import "slices"

// SameText compares two optional texts by value (nil equals nil only).
func SameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// SameSet compares two string lists as sets: order and duplicates are
// ignored, as a stored permission list comes back sorted and de-duplicated.
func SameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(slices.Compact(x), slices.Compact(y))
}
