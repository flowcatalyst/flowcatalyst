package fn

import (
	"net/http"
	"strconv"
	"time"
)

// Retry answers the caller with 429 and a Retry-After header. The dispatch
// job / scheduled-job delivery paths treat this as a deferral that spends
// no retry budget (plan §6.2 step 5, §13.2).
func Retry(w http.ResponseWriter, d time.Duration) {
	secs := max(int(d/time.Second), 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	w.WriteHeader(http.StatusTooManyRequests)
}

// Reject answers the caller with 422 and the FlowCatalyst-Outcome: reject
// header, the terminal "don't retry" outcome adopted in plan §13.2. reason
// is written as the response body for diagnostics; it may be empty.
func Reject(w http.ResponseWriter, reason string) {
	w.Header().Set("FlowCatalyst-Outcome", "reject")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if reason != "" {
		_, _ = w.Write([]byte(reason))
	}
}
