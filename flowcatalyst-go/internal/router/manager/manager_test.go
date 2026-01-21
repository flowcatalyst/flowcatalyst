package manager

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/router/pool"
)

// MockMediator implements pool.Mediator for testing
type MockMediator struct {
	processFunc func(msg *pool.MessagePointer) *pool.MediationOutcome
	callCount   atomic.Int32
}

func (m *MockMediator) Process(msg *pool.MessagePointer) *pool.MediationOutcome {
	m.callCount.Add(1)
	if m.processFunc != nil {
		return m.processFunc(msg)
	}
	return &pool.MediationOutcome{Result: pool.MediationResultSuccess}
}

func TestNewQueueManager(t *testing.T) {
	manager := NewQueueManager(nil)

	if manager == nil {
		t.Fatal("NewQueueManager returned nil")
	}

	if manager.pools == nil {
		t.Error("pools map is nil")
	}

	if manager.mediator == nil {
		t.Error("mediator is nil")
	}

	if manager.messageCallback == nil {
		t.Error("messageCallback is nil")
	}
}

func TestQueueManagerStartStop(t *testing.T) {
	manager := NewQueueManager(nil)

	manager.Start()

	manager.runningMu.Lock()
	if !manager.running {
		t.Error("Manager should be running after Start()")
	}
	manager.runningMu.Unlock()

	manager.Stop()

	manager.runningMu.Lock()
	if manager.running {
		t.Error("Manager should not be running after Stop()")
	}
	manager.runningMu.Unlock()
}

func TestGetOrCreatePool(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	cfg := &PoolConfig{
		Code:          "test-pool",
		Concurrency:   5,
		QueueCapacity: 100,
	}

	// First call should create the pool
	pool1 := manager.GetOrCreatePool(cfg)
	if pool1 == nil {
		t.Fatal("GetOrCreatePool returned nil")
	}

	// Second call should return the same pool
	pool2 := manager.GetOrCreatePool(cfg)
	if pool1 != pool2 {
		t.Error("GetOrCreatePool returned different pool for same code")
	}

	// Verify pool exists in map
	if manager.GetPool("test-pool") != pool1 {
		t.Error("GetPool returned different pool than GetOrCreatePool")
	}
}

func TestGetPoolNonExistent(t *testing.T) {
	manager := NewQueueManager(nil)

	pool := manager.GetPool("non-existent")
	if pool != nil {
		t.Error("GetPool should return nil for non-existent pool")
	}
}

func TestUpdatePoolNonExistent(t *testing.T) {
	manager := NewQueueManager(nil)

	updated := manager.UpdatePool(&PoolConfig{
		Code:        "non-existent",
		Concurrency: 10,
	})

	if updated {
		t.Error("UpdatePool should return false for non-existent pool")
	}
}

func TestRemovePool(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	// Create a pool
	cfg := &PoolConfig{
		Code:          "remove-test",
		Concurrency:   5,
		QueueCapacity: 100,
	}
	manager.GetOrCreatePool(cfg)

	// Verify it exists
	if manager.GetPool("remove-test") == nil {
		t.Fatal("Pool should exist before removal")
	}

	// Remove it
	manager.RemovePool("remove-test")

	// Verify it's gone
	if manager.GetPool("remove-test") != nil {
		t.Error("Pool should not exist after removal")
	}
}

func TestRouteMessageWhenNotRunning(t *testing.T) {
	manager := NewQueueManager(nil)
	// Don't call Start()

	msg := &DispatchMessage{
		JobID:          "test-job",
		DispatchPoolID: "test-pool",
		MessageGroup:   "group-1",
		TargetURL:      "http://example.com",
		Payload:        "{}",
	}

	if manager.RouteMessage(msg) {
		t.Error("RouteMessage should return false when manager is not running")
	}
}

func TestRouteMessageDeduplication(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	msg := &DispatchMessage{
		JobID:          "duplicate-test",
		DispatchPoolID: "test-pool",
		MessageGroup:   "group-1",
		TargetURL:      "http://example.com",
		Payload:        "{}",
	}

	// First submission
	result1 := manager.RouteMessage(msg)

	// Second submission with same ID should be deduplicated
	result2 := manager.RouteMessage(msg)

	if !result1 || !result2 {
		t.Error("Both RouteMessage calls should succeed (second deduplicated)")
	}

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)
}

func TestAckRemovesFromPipeline(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	msg := &pool.MessagePointer{
		ID: "ack-test",
	}

	// Add to pipeline
	manager.inPipelineMap.Store(msg.ID, msg)

	// Verify it's there
	if _, exists := manager.inPipelineMap.Load(msg.ID); !exists {
		t.Fatal("Message should be in pipeline map")
	}

	// Ack should remove it
	manager.Ack(msg)

	// Verify it's gone
	if _, exists := manager.inPipelineMap.Load(msg.ID); exists {
		t.Error("Message should be removed from pipeline map after ack")
	}
}

func TestNackRemovesFromPipeline(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	msg := &pool.MessagePointer{
		ID: "nack-test",
	}

	// Add to pipeline
	manager.inPipelineMap.Store(msg.ID, msg)

	// Nack should remove it
	manager.Nack(msg)

	// Verify it's gone
	if _, exists := manager.inPipelineMap.Load(msg.ID); exists {
		t.Error("Message should be removed from pipeline map after nack")
	}
}

func TestMessageCallbackAck(t *testing.T) {
	manager := NewQueueManager(nil)
	callback := &MessageCallbackImpl{manager: manager}

	var ackCalled atomic.Bool
	msg := &pool.MessagePointer{
		ID: "callback-ack-test",
		AckFunc: func() error {
			ackCalled.Store(true)
			return nil
		},
	}

	manager.inPipelineMap.Store(msg.ID, msg)

	callback.Ack(msg)

	if !ackCalled.Load() {
		t.Error("AckFunc should have been called")
	}
}

func TestMessageCallbackNack(t *testing.T) {
	manager := NewQueueManager(nil)
	callback := &MessageCallbackImpl{manager: manager}

	var nakCalled atomic.Bool
	msg := &pool.MessagePointer{
		ID: "callback-nack-test",
		NakFunc: func() error {
			nakCalled.Store(true)
			return nil
		},
	}

	manager.inPipelineMap.Store(msg.ID, msg)

	callback.Nack(msg)

	if !nakCalled.Load() {
		t.Error("NakFunc should have been called")
	}
}

func TestMessageCallbackSetVisibilityDelay(t *testing.T) {
	manager := NewQueueManager(nil)
	callback := &MessageCallbackImpl{manager: manager}

	var delaySeconds atomic.Int32
	msg := &pool.MessagePointer{
		ID: "visibility-test",
		NakDelayFunc: func(d time.Duration) error {
			delaySeconds.Store(int32(d.Seconds()))
			return nil
		},
	}

	callback.SetVisibilityDelay(msg, 30)

	if delaySeconds.Load() != 30 {
		t.Errorf("Expected 30 second delay, got %d", delaySeconds.Load())
	}
}

func TestMultiplePoolsConcurrent(t *testing.T) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	var wg sync.WaitGroup
	poolCount := 5

	for i := 0; i < poolCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg := &PoolConfig{
				Code:          string(rune('A' + idx)),
				Concurrency:   5,
				QueueCapacity: 100,
			}
			manager.GetOrCreatePool(cfg)
		}(i)
	}

	wg.Wait()

	// Verify all pools were created
	manager.poolsMu.RLock()
	defer manager.poolsMu.RUnlock()

	if len(manager.pools) != poolCount {
		t.Errorf("Expected %d pools, got %d", poolCount, len(manager.pools))
	}
}

func TestGenerateBatchID(t *testing.T) {
	ids := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		id := GenerateBatchID()
		if ids[id] {
			t.Errorf("Duplicate batch ID generated: %s", id)
		}
		ids[id] = true

		// TSID should be 13 characters
		if len(id) != 13 {
			t.Errorf("Expected 13 character batch ID, got %d: %s", len(id), id)
		}
	}
}

func TestRouterStartStop(t *testing.T) {
	router := NewRouter(nil, nil)

	router.Start()

	if router.manager == nil {
		t.Error("Router manager is nil")
	}

	router.Stop()
}

func TestRouterManager(t *testing.T) {
	router := NewRouter(nil, nil)

	manager := router.Manager()
	if manager == nil {
		t.Error("Router.Manager() returned nil")
	}
}

func BenchmarkRouteMessage(b *testing.B) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &DispatchMessage{
			JobID:          string(rune(i)),
			DispatchPoolID: "bench-pool",
			MessageGroup:   "group-1",
			TargetURL:      "http://example.com",
			Payload:        "{}",
		}
		manager.RouteMessage(msg)
	}
}

func BenchmarkGetOrCreatePool(b *testing.B) {
	manager := NewQueueManager(nil)
	manager.Start()
	defer manager.Stop()

	cfg := &PoolConfig{
		Code:          "bench-pool",
		Concurrency:   10,
		QueueCapacity: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetOrCreatePool(cfg)
	}
}
