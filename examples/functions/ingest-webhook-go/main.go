// ingest-webhook: a public endpoint for a third party that signs what it sends.
// It verifies an HMAC-SHA256 signature over the raw body against a declared
// secret, then creates one DISPATCH JOB that delivers the payload to a webhook
// function of the same application (see names.go).
//
//	sender --POST /ingest  X-Signature: sha256=<hex>--> ingest-webhook
//	  --fn.Dispatch--> DISPATCH JOB (pool + connection declared below)
//	  --> <target function> POST /deliver
//
// The endpoint is auth "none": the runner cannot know the sender's scheme, so
// the signature is this function's whole authentication. A request that fails
// it is answered 401 and nothing is dispatched.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

var signingSecret = fn.Secret(signingSecretKey)

func init() {
	fn.Dispatches(targetAddress, deliverPath, fn.DispatchPool(dispatchPool), fn.DispatchConnection(dispatchConnection))
	fn.Open("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, `{"status":"ok"}`) })
	fn.Open("POST "+ingestPath, ingest, fn.MaxBody(1<<20))
}

func main() {}

func reply(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// validSignature reports whether header is "sha256=" + hex(HMAC-SHA256(secret, body)),
// compared in constant time.
func validSignature(secret string, body []byte, header string) bool {
	hexSum, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(hexSum)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

func ingest(w http.ResponseWriter, r *http.Request) {
	// The runner has already enforced the 1 MiB body limit (413).
	body, err := io.ReadAll(r.Body)
	if err != nil {
		reply(w, 400, `{"error":"UNREADABLE_BODY"}`)
		return
	}

	secret, found, err := signingSecret.Get(r.Context())
	if err != nil || !found || secret == "" {
		// Misconfiguration or platform trouble, not the sender's fault; say nothing more.
		fn.Log(r.Context()).Error("signing secret unavailable", "found", found, "error", err)
		reply(w, 500, `{"error":"NOT_CONFIGURED"}`)
		return
	}
	if !validSignature(secret, body, r.Header.Get(signatureHeader)) {
		reply(w, 401, `{"error":"INVALID_SIGNATURE"}`)
		return
	}

	if first := bytes.TrimSpace(body); !json.Valid(first) || (first[0] != '{' && first[0] != '[') {
		reply(w, 400, `{"error":"PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY"}`)
		return
	}

	group := defaultMessageGroup
	if g := r.Header.Get("X-Message-Group"); g != "" {
		if len(g) > 128 {
			reply(w, 400, `{"error":"INVALID_MESSAGE_GROUP"}`)
			return
		}
		group = g
	}
	meta := map[string]string{"source": "webhook"}
	if id := r.Header.Get("X-Delivery-Id"); id != "" {
		meta["deliveryId"] = id
	}

	// The correlation id defaults to the request's X-Correlation-Id.
	jobID, err := fn.Dispatch(r.Context(), fn.Job{
		Target:       targetAddress,
		Path:         deliverPath,
		Payload:      body,
		MessageGroup: group,
		Descriptor:   descriptor,
		Metadata:     meta,
	})
	if err != nil {
		if errors.Is(err, fn.ErrRetryable) {
			fn.Retry(w, 5*time.Second)
			return
		}
		fn.Log(r.Context()).Error("dispatch refused", "error", err)
		reply(w, 502, `{"error":"DISPATCH_REFUSED"}`)
		return
	}
	fn.Log(r.Context()).Info("payload dispatched", "jobId", jobID, "bytes", len(body))
	out, _ := json.Marshal(map[string]string{"jobId": jobID})
	reply(w, 202, string(out))
}
