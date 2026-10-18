// payload-logger: the delivery target for dispatch-creator's jobs. The platform
// POSTs each job's payload here (the job is dataOnly, so the body is exactly
// what dispatch-creator was given), signed; the runner verifies the signature
// (webhook auth) before this handler runs. It logs the payload and acks.
//
// It declares no subscription: the job is created directly by dispatch-creator
// (fn.Dispatch), which names this function's address and this path.
package main

import (
	"io"
	"net/http"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

func init() {
	fn.Webhook("POST /deliver", deliver)
}

func main() {}

func deliver(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fn.Log(r.Context()).Info("payload received", "payload", string(body), "invocation", fn.Invocation(r.Context()).ID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ack":true}`))
}
