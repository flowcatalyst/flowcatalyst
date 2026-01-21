package outbox

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockRepository implements Repository for testing
type MockRepository struct {
	mu                sync.Mutex
	items             map[string]*OutboxItem
	fetchCalls        int
	completedIDs      []string
	failedIDs         []string
	retryIDs          []string
	recoveredCount    int64
	fetchAndLockFunc  func(ctx context.Context, itemType OutboxItemType, limit int) ([]*OutboxItem, error)
	markCompletedFunc func(ctx context.Context, itemType OutboxItemType, ids []string) error
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		items:        make(map[string]*OutboxItem),
		completedIDs: make([]string, 0),
		failedIDs:    make([]string, 0),
		retryIDs:     make([]string, 0),
	}
}

func (r *MockRepository) FetchAndLockPending(ctx context.Context, itemType OutboxItemType, limit int) ([]*OutboxItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fetchCalls++

	if r.fetchAndLockFunc != nil {
		return r.fetchAndLockFunc(ctx, itemType, limit)
	}

	var items []*OutboxItem
	for _, item := range r.items {
		if item.Type == itemType && item.Status == OutboxStatusPending {
			item.Status = OutboxStatusProcessing
			items = append(items, item)
			if len(items) >= limit {
				break
			}
		}
	}
	return items, nil
}

func (r *MockRepository) MarkCompleted(ctx context.Context, itemType OutboxItemType, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.markCompletedFunc != nil {
		return r.markCompletedFunc(ctx, itemType, ids)
	}

	r.completedIDs = append(r.completedIDs, ids...)
	for _, id := range ids {
		if item, ok := r.items[id]; ok {
			item.Status = OutboxStatusCompleted
		}
	}
	return nil
}

func (r *MockRepository) MarkFailed(ctx context.Context, itemType OutboxItemType, ids []string, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failedIDs = append(r.failedIDs, ids...)
	for _, id := range ids {
		if item, ok := r.items[id]; ok {
			item.Status = OutboxStatusFailed
			item.ErrorMessage = errorMessage
		}
	}
	return nil
}

func (r *MockRepository) ScheduleRetry(ctx context.Context, itemType OutboxItemType, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retryIDs = append(r.retryIDs, ids...)
	for _, id := range ids {
		if item, ok := r.items[id]; ok {
			item.Status = OutboxStatusPending
			item.RetryCount++
		}
	}
	return nil
}

func (r *MockRepository) RecoverStuckItems(ctx context.Context, itemType OutboxItemType, timeoutSeconds int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recoveredCount, nil
}

func (r *MockRepository) GetTableName(itemType OutboxItemType) string {
	switch itemType {
	case OutboxItemTypeEvent:
		return "outbox_events"
	case OutboxItemTypeDispatchJob:
		return "outbox_dispatch_jobs"
	default:
		return "outbox_events"
	}
}

func (r *MockRepository) AddItem(item *OutboxItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
}

func (r *MockRepository) GetFetchCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fetchCalls
}

func (r *MockRepository) GetCompletedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.completedIDs...)
}

// MockAPIClient implements APIClient behavior for testing
type MockAPIClient struct {
	mu             sync.Mutex
	eventBatches   [][]*OutboxItem
	dispatchBatches [][]*OutboxItem
	sendEventFunc  func(ctx context.Context, items []*OutboxItem) (*BatchResult, error)
	sendDispatchFunc func(ctx context.Context, items []*OutboxItem) (*BatchResult, error)
}

func NewMockAPIClient() *MockAPIClient {
	return &MockAPIClient{
		eventBatches:   make([][]*OutboxItem, 0),
		dispatchBatches: make([][]*OutboxItem, 0),
	}
}

func (c *MockAPIClient) SendEventBatch(ctx context.Context, items []*OutboxItem) (*BatchResult, error) {
	c.mu.Lock()
	c.eventBatches = append(c.eventBatches, items)
	c.mu.Unlock()

	if c.sendEventFunc != nil {
		return c.sendEventFunc(ctx, items)
	}

	// Default: all succeed
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return &BatchResult{SuccessIDs: ids}, nil
}

func (c *MockAPIClient) SendDispatchJobBatch(ctx context.Context, items []*OutboxItem) (*BatchResult, error) {
	c.mu.Lock()
	c.dispatchBatches = append(c.dispatchBatches, items)
	c.mu.Unlock()

	if c.sendDispatchFunc != nil {
		return c.sendDispatchFunc(ctx, items)
	}

	// Default: all succeed
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return &BatchResult{SuccessIDs: ids}, nil
}

func (c *MockAPIClient) GetEventBatchCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.eventBatches)
}

func TestNewProcessor(t *testing.T) {
	repo := NewMockRepository()
	apiClient := &APIClient{}

	processor := NewProcessor(repo, apiClient, nil)

	if processor == nil {
		t.Fatal("NewProcessor returned nil")
	}

	if processor.config.PollInterval != time.Second {
		t.Errorf("Expected default poll interval 1s, got %v", processor.config.PollInterval)
	}

	if processor.config.PollBatchSize != 500 {
		t.Errorf("Expected default batch size 500, got %d", processor.config.PollBatchSize)
	}
}

func TestProcessorStartStop(t *testing.T) {
	repo := NewMockRepository()
	apiClient := &APIClient{}
	config := &ProcessorConfig{
		Enabled:          true,
		PollInterval:     100 * time.Millisecond,
		PollBatchSize:    10,
		GlobalBufferSize: 100,
		MaxConcurrentGroups: 5,
		RecoveryInterval: time.Hour, // Long to prevent during test
	}

	processor := NewProcessor(repo, apiClient, config)

	processor.Start()
	time.Sleep(50 * time.Millisecond)

	// Should be running
	processor.runningMu.Lock()
	running := processor.running
	processor.runningMu.Unlock()

	if !running {
		t.Error("Processor should be running after Start()")
	}

	processor.Stop()

	processor.runningMu.Lock()
	running = processor.running
	processor.runningMu.Unlock()

	if running {
		t.Error("Processor should not be running after Stop()")
	}
}

func TestProcessorDisabled(t *testing.T) {
	repo := NewMockRepository()
	apiClient := &APIClient{}
	config := &ProcessorConfig{
		Enabled:          false,
		PollInterval:     100 * time.Millisecond,
		GlobalBufferSize: 100,
		MaxConcurrentGroups: 5,
	}

	processor := NewProcessor(repo, apiClient, config)
	processor.Start()
	defer processor.Stop()

	time.Sleep(200 * time.Millisecond)

	// Should not have polled when disabled
	if repo.GetFetchCalls() > 0 {
		t.Errorf("Disabled processor should not poll, got %d calls", repo.GetFetchCalls())
	}
}

func TestProcessorPolling(t *testing.T) {
	repo := NewMockRepository()
	apiClient := &APIClient{}
	config := &ProcessorConfig{
		Enabled:                  true,
		PollInterval:             50 * time.Millisecond,
		PollBatchSize:            10,
		GlobalBufferSize:         100,
		MaxConcurrentGroups:      5,
		RecoveryInterval:         time.Hour,
		ProcessingTimeoutSeconds: 300,
	}

	processor := NewProcessor(repo, apiClient, config)
	processor.Start()
	defer processor.Stop()

	// Wait for a few poll cycles
	time.Sleep(200 * time.Millisecond)

	fetchCalls := repo.GetFetchCalls()
	if fetchCalls < 2 {
		t.Errorf("Expected at least 2 fetch calls, got %d", fetchCalls)
	}
}

func TestDefaultProcessorConfig(t *testing.T) {
	config := DefaultProcessorConfig()

	if !config.Enabled {
		t.Error("Default config should be enabled")
	}

	if config.PollInterval != time.Second {
		t.Errorf("Expected poll interval 1s, got %v", config.PollInterval)
	}

	if config.PollBatchSize != 500 {
		t.Errorf("Expected batch size 500, got %d", config.PollBatchSize)
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected max retries 3, got %d", config.MaxRetries)
	}
}

func TestOutboxItem(t *testing.T) {
	item := &OutboxItem{
		ID:           "test-123",
		Type:         OutboxItemTypeEvent,
		MessageGroup: "",
		Payload:      `{"test": true}`,
		Status:       OutboxStatusPending,
		RetryCount:   0,
		CreatedAt:    time.Now(),
	}

	// Test GetEffectiveMessageGroup with empty group - returns "default"
	if item.GetEffectiveMessageGroup() != "default" {
		t.Errorf("Expected 'default' as message group when empty, got %s", item.GetEffectiveMessageGroup())
	}

	// Test with explicit group
	item.MessageGroup = "my-group"
	if item.GetEffectiveMessageGroup() != "my-group" {
		t.Errorf("Expected 'my-group', got %s", item.GetEffectiveMessageGroup())
	}
}

func TestProcessorBufferBackpressure(t *testing.T) {
	repo := NewMockRepository()
	apiClient := &APIClient{}

	// Small buffer to test backpressure
	config := &ProcessorConfig{
		Enabled:             true,
		PollInterval:        time.Hour, // Manual polling
		PollBatchSize:       100,
		GlobalBufferSize:    5, // Very small buffer
		MaxConcurrentGroups: 1,
		RecoveryInterval:    time.Hour,
	}

	// Add many items
	for i := 0; i < 20; i++ {
		repo.AddItem(&OutboxItem{
			ID:      string(rune('a' + i)),
			Type:    OutboxItemTypeEvent,
			Status:  OutboxStatusPending,
			Payload: `{}`,
		})
	}

	processor := NewProcessor(repo, apiClient, config)

	// Don't start the distributor - manually test buffer
	ctx := context.Background()
	processor.pollItemType(ctx, OutboxItemTypeEvent)

	// Buffer should be at capacity (5) and some items rejected
	bufSize := atomic.LoadInt32(&processor.bufferSize)
	if bufSize > 5 {
		t.Errorf("Buffer size %d exceeds capacity 5", bufSize)
	}
}

func TestMessageGroupProcessor(t *testing.T) {
	var processedCount atomic.Int32

	repo := NewMockRepository()
	apiClient := &APIClient{}
	config := &ProcessorConfig{
		Enabled:             true,
		PollInterval:        time.Hour,
		PollBatchSize:       10,
		APIBatchSize:        5,
		GlobalBufferSize:    100,
		MaxConcurrentGroups: 5,
		MaxRetries:          3,
		RecoveryInterval:    time.Hour,
	}

	processor := NewProcessor(repo, apiClient, config)

	mgp := &MessageGroupProcessor{
		groupKey:  "test:group1",
		itemType:  OutboxItemTypeEvent,
		queue:     make(chan *OutboxItem, 100),
		processor: processor,
	}

	// Add items to group queue
	for i := 0; i < 3; i++ {
		item := &OutboxItem{
			ID:      string(rune('a' + i)),
			Type:    OutboxItemTypeEvent,
			Status:  OutboxStatusProcessing,
			Payload: `{}`,
		}
		repo.AddItem(item)
		mgp.queue <- item
		processedCount.Add(1)
	}

	// Collect batch
	batch := mgp.collectBatch()
	if len(batch) != 3 {
		t.Errorf("Expected batch of 3, got %d", len(batch))
	}
}
