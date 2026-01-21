package common

import (
	"net/http"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// TracingMiddleware extracts distributed tracing headers from incoming requests
// and adds them to the request context. It also ensures a correlation ID is
// present in responses for client-side tracking.
//
// Supported headers:
//   - X-Correlation-ID: Primary distributed tracing ID
//   - X-Request-ID: Alternative to correlation ID (some clients use this)
//   - X-Causation-ID: ID of the event that caused this request
//
// If no correlation ID is provided, one is generated automatically.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract or generate correlation ID
		correlationID := r.Header.Get(HeaderCorrelationID)
		if correlationID == "" {
			correlationID = r.Header.Get(HeaderRequestID)
		}
		if correlationID == "" {
			correlationID = "trace-" + tsid.Generate()
		}

		// Extract causation ID (may be empty)
		causationID := r.Header.Get(HeaderCausationID)

		// Add to context
		ctx := WithCorrelationID(r.Context(), correlationID)
		if causationID != "" {
			ctx = WithCausationID(ctx, causationID)
		}

		// Add correlation ID to response headers
		w.Header().Set(HeaderCorrelationID, correlationID)

		// Continue with updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ResponseWriter wrapper that captures status code for logging
type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newTracingResponseWriter(w http.ResponseWriter) *tracingResponseWriter {
	return &tracingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (trw *tracingResponseWriter) WriteHeader(code int) {
	if !trw.written {
		trw.statusCode = code
		trw.written = true
	}
	trw.ResponseWriter.WriteHeader(code)
}

func (trw *tracingResponseWriter) Write(b []byte) (int, error) {
	if !trw.written {
		trw.written = true
	}
	return trw.ResponseWriter.Write(b)
}

// TracingLoggingMiddleware combines tracing with request/response logging.
// It logs incoming requests with their correlation IDs and response status codes.
func TracingLoggingMiddleware(logger Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract or generate correlation ID
			correlationID := r.Header.Get(HeaderCorrelationID)
			if correlationID == "" {
				correlationID = r.Header.Get(HeaderRequestID)
			}
			if correlationID == "" {
				correlationID = "trace-" + tsid.Generate()
			}

			causationID := r.Header.Get(HeaderCausationID)

			// Add to context
			ctx := WithCorrelationID(r.Context(), correlationID)
			if causationID != "" {
				ctx = WithCausationID(ctx, causationID)
			}

			// Add correlation ID to response headers
			w.Header().Set(HeaderCorrelationID, correlationID)

			// Wrap response writer to capture status
			trw := newTracingResponseWriter(w)

			// Log incoming request
			if logger != nil {
				logger.Debug().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("correlationId", correlationID).
					Str("causationId", causationID).
					Msg("Incoming request")
			}

			// Process request
			next.ServeHTTP(trw, r.WithContext(ctx))

			// Log response
			if logger != nil {
				logger.Debug().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Int("status", trw.statusCode).
					Str("correlationId", correlationID).
					Msg("Request completed")
			}
		})
	}
}

// Logger interface for structured logging
type Logger interface {
	Debug() LogEvent
	Info() LogEvent
	Warn() LogEvent
	Error() LogEvent
}

// LogEvent interface for building log entries
type LogEvent interface {
	Str(key, val string) LogEvent
	Int(key string, val int) LogEvent
	Msg(msg string)
}

// PropagateTracingHeaders copies tracing headers to an outgoing request.
// Use this when making HTTP calls to other services to maintain the trace.
func PropagateTracingHeaders(ctx interface{ Value(any) any }, req *http.Request) {
	if correlationID, ok := ctx.Value(correlationIDKey).(string); ok && correlationID != "" {
		req.Header.Set(HeaderCorrelationID, correlationID)
	}
	if causationID, ok := ctx.Value(causationIDKey).(string); ok && causationID != "" {
		req.Header.Set(HeaderCausationID, causationID)
	}
}

// NewTracingHTTPClient creates an HTTP client that propagates tracing headers.
func NewTracingHTTPClient(base *http.Client) *TracingHTTPClient {
	if base == nil {
		base = http.DefaultClient
	}
	return &TracingHTTPClient{client: base}
}

// TracingHTTPClient wraps http.Client to automatically propagate tracing headers.
type TracingHTTPClient struct {
	client *http.Client
}

// Do executes an HTTP request with tracing headers propagated from context.
func (c *TracingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	PropagateTracingHeaders(req.Context(), req)
	return c.client.Do(req)
}
