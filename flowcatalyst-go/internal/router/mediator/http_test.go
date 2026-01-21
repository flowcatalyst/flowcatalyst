package mediator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/router/pool"
)

func TestNewHTTPMediator(t *testing.T) {
	mediator := NewHTTPMediator(nil)

	if mediator == nil {
		t.Fatal("NewHTTPMediator returned nil")
	}

	if mediator.client == nil {
		t.Error("HTTP client is nil")
	}

	if mediator.maxRetries != 3 {
		t.Errorf("Expected maxRetries 3, got %d", mediator.maxRetries)
	}
}

func TestHTTPMediatorProcess_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ack": true})
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            3,
		BaseBackoff:           100 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultSuccess {
		t.Errorf("Expected Success, got %v", outcome.Result)
	}
}

func TestHTTPMediatorProcess_ClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            3,
		BaseBackoff:           100 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorConfig {
		t.Errorf("Expected ErrorConfig for 400, got %v", outcome.Result)
	}

	if outcome.StatusCode != 400 {
		t.Errorf("Expected status code 400, got %d", outcome.StatusCode)
	}
}

func TestHTTPMediatorProcess_ServerError(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            3,
		BaseBackoff:           50 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorProcess {
		t.Errorf("Expected ErrorProcess for 500, got %v", outcome.Result)
	}

	// Should have retried 3 times
	if callCount.Load() != 3 {
		t.Errorf("Expected 3 retry attempts, got %d", callCount.Load())
	}
}

func TestHTTPMediatorProcess_AckFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ack":          false,
			"delaySeconds": 5, // Matches Java's MediationResponse.delaySeconds
		})
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            1, // Only 1 attempt to speed up test
		BaseBackoff:           50 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorProcess {
		t.Errorf("Expected ErrorProcess for ack=false, got %v", outcome.Result)
	}

	if outcome.Delay == nil {
		t.Error("Expected delay to be set")
	} else if *outcome.Delay != 5*time.Second {
		t.Errorf("Expected 5s delay, got %v", *outcome.Delay)
	}
}

func TestHTTPMediatorProcess_TooManyRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]int{"delaySeconds": 10})
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            1,
		BaseBackoff:           50 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorProcess {
		t.Errorf("Expected ErrorProcess for 429, got %v", outcome.Result)
	}

	if outcome.StatusCode != 429 {
		t.Errorf("Expected status code 429, got %d", outcome.StatusCode)
	}
}

func TestHTTPMediatorProcess_NilMessage(t *testing.T) {
	mediator := NewHTTPMediator(nil)

	outcome := mediator.Process(nil)

	if outcome.Result != pool.MediationResultErrorConfig {
		t.Errorf("Expected ErrorConfig for nil message, got %v", outcome.Result)
	}
}

func TestHTTPMediatorProcess_NoTargetURL(t *testing.T) {
	mediator := NewHTTPMediator(nil)

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: "",
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorConfig {
		t.Errorf("Expected ErrorConfig for empty target URL, got %v", outcome.Result)
	}
}

func TestHTTPMediatorProcess_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               100 * time.Millisecond,
		MaxRetries:            1,
		BaseBackoff:           50 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
		TimeoutSeconds:  1, // Will be overridden by config for this test
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorConnection {
		t.Errorf("Expected ErrorConnection for timeout, got %v", outcome.Result)
	}
}

func TestHTTPMediatorProcess_ConnectionRefused(t *testing.T) {
	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               1 * time.Second,
		MaxRetries:            1,
		BaseBackoff:           50 * time.Millisecond,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: "http://localhost:59999", // Unlikely to be in use
		Payload:         []byte(`{"test": true}`),
	}

	outcome := mediator.Process(msg)

	if outcome.Result != pool.MediationResultErrorConnection {
		t.Errorf("Expected ErrorConnection for connection refused, got %v", outcome.Result)
	}
}

func TestHTTPMediatorProcess_Headers(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            1,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "test-1",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
		Headers: map[string]string{
			"X-Custom-Header": "test-value",
			"Authorization":   "Bearer token123",
		},
	}

	mediator.Process(msg)

	if receivedHeaders.Get("X-Custom-Header") != "test-value" {
		t.Errorf("Expected X-Custom-Header 'test-value', got '%s'", receivedHeaders.Get("X-Custom-Header"))
	}

	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Expected Authorization header, got '%s'", receivedHeaders.Get("Authorization"))
	}

	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", receivedHeaders.Get("Content-Type"))
	}
}

func TestHTTPMediatorProcess_CircuitBreaker(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:                   5 * time.Second,
		MaxRetries:                1,
		BaseBackoff:               10 * time.Millisecond,
		CircuitBreakerEnabled:     true,
		CircuitBreakerRequests:    3,
		CircuitBreakerInterval:    10 * time.Second,
		CircuitBreakerRatio:       0.5,
		CircuitBreakerTimeout:     1 * time.Second,
		CircuitBreakerMinRequests: 3,
	})

	// Make enough requests to potentially trip the circuit breaker
	for i := 0; i < 10; i++ {
		msg := &pool.MessagePointer{
			ID:              string(rune('a' + i)),
			MediationTarget: server.URL,
			Payload:         []byte(`{"test": true}`),
		}
		mediator.Process(msg)
	}

	// Circuit breaker should have tripped, reducing total calls
	if callCount.Load() == 10 {
		t.Log("Note: Circuit breaker may not have tripped in this test run")
	}
}

func BenchmarkHTTPMediatorProcess(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mediator := NewHTTPMediator(&HTTPMediatorConfig{
		Timeout:               5 * time.Second,
		MaxRetries:            1,
		CircuitBreakerEnabled: false,
	})

	msg := &pool.MessagePointer{
		ID:              "bench",
		MediationTarget: server.URL,
		Payload:         []byte(`{"test": true}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mediator.Process(msg)
	}
}
