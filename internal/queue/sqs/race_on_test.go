//go:build race

package sqs

// raceEnabled: allocation counts are not meaningful under -race (sync.Pool
// drops items, instrumentation allocates).
const raceEnabled = true
