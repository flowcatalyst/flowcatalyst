package router

import (
	"context"
	"testing"
)

func TestTrafficStrategy_DisabledNoOps(t *testing.T) {
	s, err := NewTrafficStrategy(context.Background(), TrafficConfig{Enabled: false})
	if err != nil {
		t.Fatalf("NewTrafficStrategy: %v", err)
	}
	if err := s.Register(context.Background()); err != nil {
		t.Errorf("Register on disabled strategy: %v", err)
	}
	if err := s.Deregister(context.Background()); err != nil {
		t.Errorf("Deregister on disabled strategy: %v", err)
	}
	st := s.Status()
	if st.Enabled {
		t.Errorf("Enabled=true on disabled strategy")
	}
	if st.Mode != "disabled" {
		t.Errorf("Mode=%q want disabled", st.Mode)
	}
}

func TestTrafficStrategy_DisablesWhenMissingFields(t *testing.T) {
	// Enabled=true but no target group / IP → strategy disables itself
	// with a warning rather than failing construction.
	s, err := NewTrafficStrategy(context.Background(), TrafficConfig{
		Enabled: true, // missing TargetGroupARN + InstanceIP
	})
	if err != nil {
		t.Fatalf("NewTrafficStrategy: %v", err)
	}
	if got := s.Status(); got.Enabled {
		t.Errorf("expected disabled when required fields missing, got %+v", got)
	}
}
