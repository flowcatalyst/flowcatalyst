package stream

import "testing"

func TestHealth_DefaultStopped(t *testing.T) {
	h := NewHealth("test")
	if h.IsRunning() {
		t.Errorf("new Health should not be running")
	}
	if h.IsHealthy() {
		t.Errorf("new Health should not be healthy")
	}
	snap := h.Status()
	if snap.Status != StatusStopped {
		t.Errorf("status=%q want STOPPED", snap.Status)
	}
}

func TestHealth_SetRunning(t *testing.T) {
	h := NewHealth("test")
	h.SetRunning(true)
	if !h.IsRunning() || !h.IsHealthy() {
		t.Errorf("running flag not reflected")
	}
	if got := h.Status().Status; got != StatusRunning {
		t.Errorf("status=%q want RUNNING", got)
	}
}

func TestHealth_AddProcessedAccumulates(t *testing.T) {
	h := NewHealth("test")
	h.AddProcessed(10)
	h.AddProcessed(25)
	h.AddProcessed(5)
	if got := h.Status().BatchSequence; got != 40 {
		t.Errorf("BatchSequence=%d want 40", got)
	}
	if h.Status().LastPollTimeMs == 0 {
		t.Errorf("LastPollTimeMs should be set after AddProcessed")
	}
}

func TestHealth_RecordErrorAccumulates(t *testing.T) {
	h := NewHealth("test")
	h.RecordError()
	h.RecordError()
	h.RecordError()
	if got := h.Status().ErrorCount; got != 3 {
		t.Errorf("ErrorCount=%d want 3", got)
	}
}

func TestHealthService_EmptyNotLiveNotReady(t *testing.T) {
	s := NewHealthService()
	if s.IsLive() {
		t.Errorf("empty service should not be live")
	}
	if s.IsReady() {
		t.Errorf("empty service should not be ready")
	}
	agg := s.Aggregate()
	if agg.Healthy || agg.TotalStreams != 0 {
		t.Errorf("empty aggregate=%+v", agg)
	}
}

func TestHealthService_LiveWhenOneRunning(t *testing.T) {
	s := NewHealthService()
	a := NewHealth("a")
	b := NewHealth("b")
	s.Register(a)
	s.Register(b)

	a.SetRunning(true)
	if !s.IsLive() {
		t.Errorf("expected live when one projection running")
	}
	if s.IsReady() {
		t.Errorf("expected NOT ready when one of two is stopped")
	}

	b.SetRunning(true)
	if !s.IsReady() {
		t.Errorf("expected ready when both running")
	}
}

func TestHealthService_AggregateCounts(t *testing.T) {
	s := NewHealthService()
	a, b, c := NewHealth("a"), NewHealth("b"), NewHealth("c")
	s.Register(a)
	s.Register(b)
	s.Register(c)

	a.SetRunning(true)
	a.AddProcessed(7)
	b.RecordError()

	agg := s.Aggregate()
	if agg.TotalStreams != 3 {
		t.Errorf("Total=%d want 3", agg.TotalStreams)
	}
	if agg.HealthyStreams != 1 {
		t.Errorf("Healthy=%d want 1", agg.HealthyStreams)
	}
	if agg.UnhealthyStreams != 2 {
		t.Errorf("Unhealthy=%d want 2", agg.UnhealthyStreams)
	}
	if agg.Healthy {
		t.Errorf("Aggregate.Healthy=true; want false when any stopped")
	}
	if len(agg.Streams) != 3 {
		t.Errorf("Streams len=%d want 3", len(agg.Streams))
	}
}
