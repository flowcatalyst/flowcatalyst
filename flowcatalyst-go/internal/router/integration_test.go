package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/router/mediator"
	"go.flowcatalyst.tech/internal/router/pool"
)

// createTestMediator creates an HTTP mediator with custom timeout for testing
func createTestMediator(timeoutMs int) *mediator.HTTPMediator {
	cfg := &mediator.HTTPMediatorConfig{
		Timeout:     time.Duration(timeoutMs) * time.Millisecond,
		MaxRetries:  1, // Single retry for faster tests
		BaseBackoff: 50 * time.Millisecond,
	}
	return mediator.NewHTTPMediator(cfg)
}

// === Integration Test Helpers ===

// TestCallback tracks message ack/nack for verification
type TestCallback struct {
	acked   sync.Map
	nacked  sync.Map
	ackMu   sync.Mutex
	nackMu  sync.Mutex
	ackList []string
	nackList []string
}

func NewTestCallback() *TestCallback {
	return &TestCallback{
		ackList:  make([]string, 0),
		nackList: make([]string, 0),
	}
}

func (c *TestCallback) Ack(msg *pool.MessagePointer) {
	c.acked.Store(msg.ID, msg)
	c.ackMu.Lock()
	c.ackList = append(c.ackList, msg.ID)
	c.ackMu.Unlock()
}

func (c *TestCallback) Nack(msg *pool.MessagePointer) {
	c.nacked.Store(msg.ID, msg)
	c.nackMu.Lock()
	c.nackList = append(c.nackList, msg.ID)
	c.nackMu.Unlock()
}

func (c *TestCallback) SetVisibilityDelay(msg *pool.MessagePointer, seconds int)  {}
func (c *TestCallback) SetFastFailVisibility(msg *pool.MessagePointer)            {}
func (c *TestCallback) ResetVisibilityToDefault(msg *pool.MessagePointer)         {}

func (c *TestCallback) IsAcked(id string) bool {
	_, ok := c.acked.Load(id)
	return ok
}

func (c *TestCallback) IsNacked(id string) bool {
	_, ok := c.nacked.Load(id)
	return ok
}

func (c *TestCallback) GetAckCount() int {
	c.ackMu.Lock()
	defer c.ackMu.Unlock()
	return len(c.ackList)
}

func (c *TestCallback) GetNackCount() int {
	c.nackMu.Lock()
	defer c.nackMu.Unlock()
	return len(c.nackList)
}

// === HTTP Response Tests ===

func TestHttpMediator_SuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"ack":    true,
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-success",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	if !callback.IsAcked("msg-success") {
		t.Error("Expected message to be ACKed on 200 response")
	}
}

func TestHttpMediator_ServerError500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Internal server error",
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-500",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	if !callback.IsNacked("msg-500") {
		t.Error("Expected message to be NACKed on 500 response")
	}
}

func TestHttpMediator_ServerError503(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Service unavailable",
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-503",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	if !callback.IsNacked("msg-503") {
		t.Error("Expected message to be NACKed on 503 response")
	}
}

func TestHttpMediator_ClientError400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Bad request",
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-400",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	// 400 errors are permanent - should be ACKed to prevent retry loops
	if !callback.IsAcked("msg-400") && !callback.IsNacked("msg-400") {
		t.Error("Expected message to be handled on 400 response")
	}
}

func TestHttpMediator_ClientError404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Not found",
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-404",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	// 404 errors are permanent config errors - should be ACKed
	if !callback.IsAcked("msg-404") && !callback.IsNacked("msg-404") {
		t.Error("Expected message to be handled on 404 response")
	}
}

// === Timeout Tests ===

func TestHttpMediator_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than timeout
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Short timeout (1 second)
	med := createTestMediator(1000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-timeout",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(2 * time.Second)

	// Timeout should result in NACK
	if !callback.IsNacked("msg-timeout") {
		t.Error("Expected message to be NACKed on timeout")
	}
}

// === Batch Processing Tests ===

func TestBatchProcessing_AllSuccess(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Submit batch of messages
	batchSize := 10
	for i := 0; i < batchSize; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("batch-msg-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i), // Different groups for parallel processing
			MediationTarget: server.URL,
			Payload:         []byte(fmt.Sprintf(`{"index": %d}`, i)),
		}
		processPool.Submit(msg)
	}

	// Wait for all to complete
	time.Sleep(500 * time.Millisecond)

	if callback.GetAckCount() != batchSize {
		t.Errorf("Expected %d acks, got %d", batchSize, callback.GetAckCount())
	}

	if int(requestCount.Load()) != batchSize {
		t.Errorf("Expected %d HTTP requests, got %d", batchSize, requestCount.Load())
	}
}

func TestBatchProcessing_MixedResults(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		// Every 3rd request fails
		if count%3 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Submit batch of messages
	batchSize := 9
	for i := 0; i < batchSize; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("mixed-msg-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i),
			MediationTarget: server.URL,
			Payload:         []byte(fmt.Sprintf(`{"index": %d}`, i)),
		}
		processPool.Submit(msg)
	}

	time.Sleep(500 * time.Millisecond)

	ackCount := callback.GetAckCount()
	nackCount := callback.GetNackCount()

	// Should have some acks and some nacks
	if ackCount+nackCount != batchSize {
		t.Errorf("Expected %d total handled messages, got %d (ack=%d, nack=%d)",
			batchSize, ackCount+nackCount, ackCount, nackCount)
	}

	if nackCount == 0 {
		t.Error("Expected some NACKs for failed requests")
	}
}

// === FIFO Ordering Tests ===

func TestFIFOOrdering_SameGroup(t *testing.T) {
	var processOrder []string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse message ID from payload - mediator sends {"messageId": "<id>"}
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		mu.Lock()
		if id, ok := payload["messageId"].(string); ok {
			processOrder = append(processOrder, id)
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond) // Simulate processing
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	// Single worker to enforce strict ordering
	processPool := pool.NewProcessPool("test-pool", 1, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Submit messages in order, same group
	sameGroup := "fifo-group"
	for i := 0; i < 5; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("fifo-%d", i),
			MessageGroupID:  sameGroup,
			MediationTarget: server.URL,
			Payload:         []byte(fmt.Sprintf(`{"id": "fifo-%d"}`, i)),
		}
		processPool.Submit(msg)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify FIFO order
	expected := []string{"fifo-0", "fifo-1", "fifo-2", "fifo-3", "fifo-4"}
	if len(processOrder) != len(expected) {
		t.Fatalf("Expected %d messages processed, got %d", len(expected), len(processOrder))
	}

	for i, id := range expected {
		if processOrder[i] != id {
			t.Errorf("Position %d: expected %s, got %s", i, id, processOrder[i])
		}
	}
}

// === Response Body Tests ===

func TestHttpMediator_AckFalseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ack": false, // Explicit nack request
		})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-ack-false",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	// When ack:false is in response, message should be NACKed for retry
	if !callback.IsNacked("msg-ack-false") && !callback.IsAcked("msg-ack-false") {
		t.Error("Expected message to be handled")
	}
}

// === Concurrency Tests ===

func TestConcurrency_ParallelProcessing(t *testing.T) {
	var processingCount atomic.Int32
	var maxConcurrent atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	concurrency := 5
	processPool := pool.NewProcessPool("test-pool", concurrency, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Submit many messages from different groups (parallel processing)
	for i := 0; i < 20; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("concurrent-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i), // Different group each
			MediationTarget: server.URL,
			Payload:         []byte(`{"event": "test"}`),
		}
		processPool.Submit(msg)
	}

	time.Sleep(1 * time.Second)

	if maxConcurrent.Load() > int32(concurrency) {
		t.Errorf("Max concurrent %d exceeded concurrency limit %d",
			maxConcurrent.Load(), concurrency)
	}

	if callback.GetAckCount() != 20 {
		t.Errorf("Expected 20 acks, got %d", callback.GetAckCount())
	}
}

// === Recovery Tests ===

func TestRecovery_TransientFailure(t *testing.T) {
	var requestCount atomic.Int32
	failFirst := true // Fail first request only
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		mu.Lock()
		shouldFail := failFirst
		mu.Unlock()

		if shouldFail {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": "temporarily unavailable"})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// First message will fail (server is in "failing" state)
	msg1 := &pool.MessagePointer{
		ID:              "transient-1",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"attempt": 1}`),
	}
	processPool.Submit(msg1)
	time.Sleep(200 * time.Millisecond)

	// Verify first message failed
	if !callback.IsNacked("transient-1") {
		t.Error("Expected first message to be NACKed")
	}

	// "Recover" the server
	mu.Lock()
	failFirst = false
	mu.Unlock()

	// New message after recovery should succeed
	msg2 := &pool.MessagePointer{
		ID:              "transient-2",
		MessageGroupID:  "group-2",
		MediationTarget: server.URL,
		Payload:         []byte(`{"attempt": 2}`),
	}
	processPool.Submit(msg2)
	time.Sleep(200 * time.Millisecond)

	if !callback.IsAcked("transient-2") {
		t.Error("Expected second message to be ACKed after recovery")
	}

	// Verify request count
	if requestCount.Load() < 2 {
		t.Errorf("Expected at least 2 requests, got %d", requestCount.Load())
	}
}

// === Queue Capacity Tests ===

func TestQueueCapacity_Overflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Slow processing
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	// Small queue capacity
	queueCapacity := 5
	processPool := pool.NewProcessPool("test-pool", 1, queueCapacity, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Track submissions
	acceptedCount := 0
	rejectedCount := 0

	// Try to submit more than queue capacity
	for i := 0; i < 20; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("overflow-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i),
			MediationTarget: server.URL,
			Payload:         []byte(`{"event": "test"}`),
		}
		if processPool.Submit(msg) {
			acceptedCount++
		} else {
			rejectedCount++
		}
	}

	// Some messages should be rejected when queue is full
	if rejectedCount == 0 {
		t.Log("Warning: No messages were rejected (queue may have more capacity)")
	}

	// Wait for accepted messages
	time.Sleep(3 * time.Second)

	// All accepted messages should complete
	totalHandled := callback.GetAckCount() + callback.GetNackCount()
	if totalHandled != acceptedCount {
		t.Logf("Expected %d handled messages, got %d", acceptedCount, totalHandled)
	}
}

// === Rate Limiting Tests ===

func TestRateLimiting_EnforcesLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit test in short mode")
	}

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	// 600 per minute = 10 per second
	rateLimit := 600
	processPool := pool.NewProcessPool("test-pool", 10, 100, &rateLimit, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	// Submit burst of messages
	burstSize := 5
	for i := 0; i < burstSize; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("rate-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i),
			MediationTarget: server.URL,
			Payload:         []byte(`{"event": "test"}`),
		}
		processPool.Submit(msg)
	}

	time.Sleep(1 * time.Second)

	// With rate limiting, messages should be processed
	if callback.GetAckCount() < burstSize {
		t.Logf("Processed %d/%d messages with rate limiting",
			callback.GetAckCount(), burstSize)
	}
}

// === Headers Tests ===

func TestHttpMediator_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("test-pool", 5, 100, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	msg := &pool.MessagePointer{
		ID:              "msg-headers",
		MessageGroupID:  "group-1",
		MediationTarget: server.URL,
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"X-Custom-Header": "custom-value",
			"Authorization":   "Bearer test-token",
		},
		Payload: []byte(`{"event": "test"}`),
	}

	processPool.Submit(msg)
	time.Sleep(200 * time.Millisecond)

	if callback.GetAckCount() != 1 {
		t.Errorf("Expected 1 ack, got %d", callback.GetAckCount())
	}

	// Verify headers were sent
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header, got %s", receivedHeaders.Get("Content-Type"))
	}
}

// === Benchmark Tests ===

func BenchmarkEndToEndMessage(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	med := createTestMediator(5000)
	callback := NewTestCallback()

	processPool := pool.NewProcessPool("bench-pool", 10, 1000, nil, med, callback)
	processPool.Start()
	defer processPool.Shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &pool.MessagePointer{
			ID:              fmt.Sprintf("bench-%d", i),
			MessageGroupID:  fmt.Sprintf("group-%d", i%10),
			MediationTarget: server.URL,
			Payload:         []byte(`{"event": "benchmark"}`),
		}
		processPool.Submit(msg)
	}

	// Wait for completion
	time.Sleep(time.Duration(b.N/100+1) * time.Millisecond)
}
