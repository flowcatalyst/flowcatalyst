package main

import "time"

// The trace commands print instants in the local time zone, and their tests
// compare that output as text. The zone is therefore fixed to UTC once, when
// the test binary is initialised and no other goroutine exists yet. Assigning
// time.Local from a test (and restoring it in a cleanup) races with every
// goroutine that reads the clock, the testing package's own timers included:
// the race detector reported it as a failure of whichever test ran last.
func init() { time.Local = time.UTC }
