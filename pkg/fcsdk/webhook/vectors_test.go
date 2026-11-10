package webhook_test

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/webhook"
)

// vectorsPath is the shared webhook signature vectors, checked by the Rust,
// Java and Go SDKs. The file lives in the platform repository; when this
// package is built from the split go-sdk repository it is absent and the
// test skips.
const vectorsPath = "../../../clients/rust-sdk/tests/fixtures/wire/webhook-signatures.json"

type signatureVector struct {
	Name      string `json:"name"`
	Secret    string `json:"secret"`
	Timestamp string `json:"timestamp"`
	Body      string `json:"body"`
	Signature string `json:"signature"`
}

func TestSharedSignatureVectors(t *testing.T) {
	raw, err := os.ReadFile(vectorsPath)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("shared vectors not present (%s); split repository build", vectorsPath)
	}
	require.NoError(t, err)

	var vectors []signatureVector
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors)

	for _, vec := range vectors {
		t.Run(vec.Name, func(t *testing.T) {
			body := []byte(vec.Body)

			// Validator signs and verifies every vector, ISO and Unix-seconds
			// timestamps alike. The clock is pinned to the timestamp so the
			// freshness window passes.
			var at time.Time
			secs, perr := strconv.ParseInt(vec.Timestamp, 10, 64)
			unixSeconds := perr == nil
			if unixSeconds {
				at = time.Unix(secs, 0)
			} else {
				at, err = time.Parse(time.RFC3339Nano, vec.Timestamp)
				require.NoError(t, err)
			}
			v := webhook.NewValidator(vec.Secret, webhook.WithClock(func() time.Time { return at }))
			require.Equal(t, vec.Signature, v.ComputeSignature(vec.Timestamp, body))
			require.NoError(t, v.Validate(vec.Signature, vec.Timestamp, body))
			require.ErrorIs(t, v.Validate(vec.Signature, vec.Timestamp, append([]byte("x"), body...)),
				webhook.ErrValidatorInvalidSignature)

			// Verifier takes ISO8601 timestamps only; skew checking is off
			// (the vectors are fixed in the past).
			ver := webhook.NewVerifier(vec.Secret)
			ver.MaxClockSkew = 0
			if unixSeconds {
				require.ErrorIs(t, ver.Verify(body, vec.Signature, vec.Timestamp), webhook.ErrBadTimestamp)
				return
			}
			require.NoError(t, ver.Verify(body, vec.Signature, vec.Timestamp))
			require.ErrorIs(t, ver.Verify(append([]byte("x"), body...), vec.Signature, vec.Timestamp),
				webhook.ErrBadSignature)
		})
	}
}
