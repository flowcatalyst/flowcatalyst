package api

import (
	"sync/atomic"
)

// MockState aggregates atomic counters consumed by /api/test/stats.
// Mirrors crates/fc-router/src/api/mod.rs test endpoint counters. Used
// only by dev / load-test flows.
type MockState struct {
	Fast        atomic.Uint64
	Slow        atomic.Uint64
	Faulty      atomic.Uint64
	Fail        atomic.Uint64
	Success     atomic.Uint64
	Pending     atomic.Uint64
	ClientError atomic.Uint64
	ServerError atomic.Uint64
	// FaultySuccess + FaultyFail track the randomised outcomes of
	// /api/test/faulty so the stats endpoint can show the ratio.
	FaultySuccess atomic.Uint64
	FaultyFail    atomic.Uint64
}

// NewMockState builds a zeroed counter set.
func NewMockState() *MockState { return &MockState{} }

// Reset zeroes every counter.
func (s *MockState) Reset() {
	s.Fast.Store(0)
	s.Slow.Store(0)
	s.Faulty.Store(0)
	s.Fail.Store(0)
	s.Success.Store(0)
	s.Pending.Store(0)
	s.ClientError.Store(0)
	s.ServerError.Store(0)
	s.FaultySuccess.Store(0)
	s.FaultyFail.Store(0)
}
