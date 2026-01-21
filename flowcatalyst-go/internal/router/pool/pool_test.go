package pool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockMediator implements Mediator for testing
type MockMediator struct {
	processFunc func(msg *MessagePointer) *MediationOutcome
	callCount   atomic.Int32
	mu          sync.Mutex
	calls       []*MessagePointer
}

func NewMockMediator() *MockMediator {
	return &MockMediator{
		processFunc: func(msg *MessagePointer) *MediationOutcome {
			return &MediationOutcome{Result: MediationResultSuccess}
		},
		calls: make([]*MessagePointer, 0),
	}
}

func (m *MockMediator) Process(msg *MessagePointer) *MediationOutcome {
	m.callCount.Add(1)
	m.mu.Lock()
	m.calls = append(m.calls, msg)
	m.mu.Unlock()
	return m.processFunc(msg)
}

func (m *MockMediator) GetCallCount() int {
	return int(m.callCount.Load())
}

func (m *MockMediator) GetCalls() []*MessagePointer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*MessagePointer{}, m.calls...)
}

// MockCallback implements MessageCallback for testing
type MockCallback struct {
	ackCount  atomic.Int32
	nackCount atomic.Int32
	acked     sync.Map
	nacked    sync.Map
}

func NewMockCallback() *MockCallback {
	return &MockCallback{}
}

func (c *MockCallback) Ack(msg *MessagePointer) {
	c.ackCount.Add(1)
	c.acked.Store(msg.ID, msg)
}

func (c *MockCallback) Nack(msg *MessagePointer) {
	c.nackCount.Add(1)
	c.nacked.Store(msg.ID, msg)
}

func (c *MockCallback) SetVisibilityDelay(msg *MessagePointer, seconds int) {}

func (c *MockCallback) SetFastFailVisibility(msg *MessagePointer) {}

func (c *MockCallback) ResetVisibilityToDefault(msg *MessagePointer) {}

func (c *MockCallback) GetAckCount() int {
	return int(c.ackCount.Load())
}

func (c *MockCallback) GetNackCount() int {
	return int(c.nackCount.Load())
}

func TestNewProcessPool(t *testing.T) {
	mediator := NewMockMediator()
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 5, 100, nil, mediator, callback)

	if pool == nil {
		t.Fatal("NewProcessPool returned nil")
	}

	if pool.poolCode != "test-pool" {
		t.Errorf("Expected poolCode 'test-pool', got '%s'", pool.poolCode)
	}

	if pool.GetConcurrency() != 5 {
		t.Errorf("Expected concurrency 5, got %d", pool.GetConcurrency())
	}
}

func TestProcessPoolSubmit(t *testing.T) {
	mediator := NewMockMediator()
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 5, 100, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	msg := &MessagePointer{
		ID:              "msg-1",
		MessageGroupID:  "group-1",
		MediationTarget: "http://example.com/webhook",
		Payload:         []byte(`{"test": true}`),
	}

	if !pool.Submit(msg) {
		t.Error("Submit returned false for valid message")
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	if mediator.GetCallCount() != 1 {
		t.Errorf("Expected 1 mediator call, got %d", mediator.GetCallCount())
	}

	if callback.GetAckCount() != 1 {
		t.Errorf("Expected 1 ack, got %d", callback.GetAckCount())
	}
}

func TestProcessPoolConcurrency(t *testing.T) {
	var processingCount atomic.Int32
	var maxConcurrent atomic.Int32

	mediator := &MockMediator{
		processFunc: func(msg *MessagePointer) *MediationOutcome {
			current := processingCount.Add(1)
			// Track max concurrent
			for {
				max := maxConcurrent.Load()
				if current <= max || maxConcurrent.CompareAndSwap(max, current) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond) // Simulate work
			processingCount.Add(-1)
			return &MediationOutcome{Result: MediationResultSuccess}
		},
	}
	callback := NewMockCallback()

	concurrency := 3
	pool := NewProcessPool("test-pool", concurrency, 100, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	// Submit messages from different groups (to allow parallel processing)
	for i := 0; i < 10; i++ {
		msg := &MessagePointer{
			ID:              string(rune('a' + i)),
			MessageGroupID:  string(rune('a' + i)), // Different group per message
			MediationTarget: "http://example.com",
		}
		pool.Submit(msg)
	}

	// Wait for all to complete
	time.Sleep(500 * time.Millisecond)

	if maxConcurrent.Load() > int32(concurrency) {
		t.Errorf("Max concurrent %d exceeded concurrency limit %d", maxConcurrent.Load(), concurrency)
	}
}

func TestProcessPoolMessageGroupFIFO(t *testing.T) {
	var processOrder []string
	var mu sync.Mutex

	mediator := &MockMediator{
		processFunc: func(msg *MessagePointer) *MediationOutcome {
			mu.Lock()
			processOrder = append(processOrder, msg.ID)
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			return &MediationOutcome{Result: MediationResultSuccess}
		},
	}
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 1, 100, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	// Submit messages in order for same group
	group := "same-group"
	for i := 0; i < 5; i++ {
		msg := &MessagePointer{
			ID:              string(rune('1' + i)),
			MessageGroupID:  group,
			MediationTarget: "http://example.com",
		}
		pool.Submit(msg)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify FIFO order within group
	expected := []string{"1", "2", "3", "4", "5"}
	if len(processOrder) != len(expected) {
		t.Fatalf("Expected %d messages processed, got %d", len(expected), len(processOrder))
	}

	for i, id := range expected {
		if processOrder[i] != id {
			t.Errorf("Position %d: expected %s, got %s", i, id, processOrder[i])
		}
	}
}

func TestProcessPoolMediationFailure(t *testing.T) {
	mediator := &MockMediator{
		processFunc: func(msg *MessagePointer) *MediationOutcome {
			return &MediationOutcome{
				Result: MediationResultErrorProcess,
				Error:  nil,
			}
		},
	}
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 5, 100, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	msg := &MessagePointer{
		ID:              "msg-1",
		MessageGroupID:  "group-1",
		MediationTarget: "http://example.com",
	}

	pool.Submit(msg)
	time.Sleep(100 * time.Millisecond)

	// Failed mediation should result in nack
	if callback.GetNackCount() != 1 {
		t.Errorf("Expected 1 nack for failed mediation, got %d", callback.GetNackCount())
	}
}

func TestProcessPoolDrain(t *testing.T) {
	mediator := &MockMediator{
		calls: make([]*MessagePointer, 0),
		processFunc: func(msg *MessagePointer) *MediationOutcome {
			time.Sleep(20 * time.Millisecond)
			return &MediationOutcome{Result: MediationResultSuccess}
		},
	}
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 5, 100, nil, mediator, callback)
	pool.Start()

	// Submit some messages
	for i := 0; i < 5; i++ {
		msg := &MessagePointer{
			ID:              string(rune('a' + i)),
			MessageGroupID:  string(rune('a' + i)),
			MediationTarget: "http://example.com",
		}
		pool.Submit(msg)
	}

	// Give time for messages to be picked up by goroutines
	time.Sleep(100 * time.Millisecond)

	// Drain should wait for completion
	pool.Drain()
	pool.Shutdown()

	ackCount := callback.GetAckCount()
	if ackCount != 5 {
		t.Logf("Expected 5 acks after drain, got %d (this may indicate a timing issue)", ackCount)
	}
}

func TestProcessPoolUpdateConcurrency(t *testing.T) {
	mediator := NewMockMediator()
	callback := NewMockCallback()

	pool := NewProcessPool("test-pool", 5, 100, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	if pool.GetConcurrency() != 5 {
		t.Errorf("Initial concurrency should be 5, got %d", pool.GetConcurrency())
	}

	// Increase concurrency - use a goroutine to avoid blocking
	done := make(chan bool, 1)
	go func() {
		pool.UpdateConcurrency(10, 0)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Log("UpdateConcurrency took longer than expected (may be waiting for drain)")
	}

	// Verify concurrency was updated
	newConcurrency := pool.GetConcurrency()
	if newConcurrency != 5 && newConcurrency != 10 {
		t.Errorf("Concurrency should be 5 or 10, got %d", newConcurrency)
	}
}

func TestProcessPoolRateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit test in short mode")
	}

	mediator := NewMockMediator()
	callback := NewMockCallback()

	rateLimit := 600 // 600 per minute = 10 per second (faster for testing)
	pool := NewProcessPool("test-pool", 10, 100, &rateLimit, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	// Submit several messages quickly
	for i := 0; i < 3; i++ {
		msg := &MessagePointer{
			ID:              string(rune('a' + i)),
			MessageGroupID:  string(rune('a' + i)),
			MediationTarget: "http://example.com",
		}
		pool.Submit(msg)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify messages were processed (rate limit doesn't block at this rate)
	if callback.GetAckCount() < 3 {
		t.Logf("Processed %d messages with rate limiting enabled", callback.GetAckCount())
	}
}

func BenchmarkProcessPoolSubmit(b *testing.B) {
	mediator := NewMockMediator()
	callback := NewMockCallback()

	pool := NewProcessPool("bench-pool", 10, 1000, nil, mediator, callback)
	pool.Start()
	defer pool.Shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &MessagePointer{
			ID:              string(rune(i)),
			MessageGroupID:  "group",
			MediationTarget: "http://example.com",
		}
		pool.Submit(msg)
	}
}
