// dispatch-creator: POST /dispatch with a JSON body (platform-authenticated)
// creates one DISPATCH JOB that delivers that body to the payload-logger
// function's /deliver webhook.
//
// A function holds no platform credential, so it cannot call the dispatch-job
// API. It uses the `dispatch` capability instead: it declares the one
// (function, path) it may dispatch to, names that target in fn.Dispatch, and
// the runner asks the platform to create the job on its behalf. The platform
// checks the target against this version's declared dispatches, requires it to
// be a live function of the SAME application with a webhook endpoint at the
// path, builds the delivery URL and signs it as the declared connection's
// service account (the job carries the connection, so pausing it holds the jobs).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

func init() {
	fn.Dispatches(loggerAddress, deliverPath, fn.DispatchConnection(connectionCode))
	fn.Open("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `{"status":"ok"}`) })
	fn.Platform("POST /dispatch", create)
}

func main() {}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func create(w http.ResponseWriter, r *http.Request) {
	// auth "platform": the runner has already verified the bearer token (401
	// otherwise). Authorisation is the function's own job: the verified
	// principal must hold the permission.
	principal := fn.CallerFrom(r.Context()).Principal()
	if principal == nil || !principal.HasPermission(requiredPermission) {
		writeJSON(w, 403, `{"error":"FORBIDDEN","message":"requires permission `+requiredPermission+`"}`)
		return
	}

	var body bytes.Buffer
	if _, err := body.ReadFrom(r.Body); err != nil || !json.Valid(body.Bytes()) {
		writeJSON(w, 400, `{"error":"INVALID_JSON"}`)
		return
	}
	if first := bytes.TrimLeft(body.Bytes(), " \t\r\n")[0]; first != '{' && first != '[' {
		writeJSON(w, 400, `{"error":"PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY"}`)
		return
	}

	jobID, err := fn.Dispatch(r.Context(), fn.Job{
		Target:       loggerAddress,
		Path:         deliverPath,
		Payload:      body.Bytes(),
		MessageGroup: "dispatch-demo",
		Descriptor:   "dispatch-demo payload",
		Metadata:     map[string]string{"requestedBy": principal.ID},
	})
	if err != nil {
		if errors.Is(err, fn.ErrRetryable) {
			writeJSON(w, 503, `{"error":"PLATFORM_UNAVAILABLE"}`)
			return
		}
		fn.Log(r.Context()).Error("dispatch refused", "error", err)
		writeJSON(w, 502, `{"error":"DISPATCH_REFUSED"}`)
		return
	}
	fn.Log(r.Context()).Info("job dispatched", "jobId", jobID)
	out, _ := json.Marshal(map[string]string{"jobId": jobID})
	writeJSON(w, 202, string(out))
}
