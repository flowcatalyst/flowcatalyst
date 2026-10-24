package ingestion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSupportedContract(t *testing.T) {
	assert.True(t, SupportedContract(ContractVersion), "the current version")
	assert.True(t, SupportedContract(ContractVersion-1), "the one before it (N-1)")
	assert.False(t, SupportedContract(ContractVersion-2), "two back")
	assert.False(t, SupportedContract(ContractVersion+1), "a newer runtime")
	assert.False(t, SupportedContract(0))
	assert.Equal(t, ContractVersion-1, MinContractVersion)
}

func TestCarries(t *testing.T) {
	// A runtime that names no built-in kind carries all of them.
	for _, kind := range []string{KindHTTP, KindSFTP, KindS3, KindSQS, KindRabbitMQ, KindDatabasePoll, KindAPIPoll} {
		assert.True(t, Carries(nil, kind), kind)
	}
	assert.False(t, Carries(nil, "mix"), "an extension kind is carried only when reported")
	assert.False(t, Carries(nil, KindCustom), "a custom source is run by its type, never as custom")

	ext := []RuntimeKind{{Kind: "mix"}, {Kind: "camel-route", RequiredPermission: "x"}}
	assert.True(t, Carries(ext, "mix"))
	assert.True(t, Carries(ext, "camel-route"))
	assert.True(t, Carries(ext, KindSFTP), "extensions alone leave the built-ins carried")
	assert.False(t, Carries(ext, "webiq"))

	// Naming built-ins narrows a runtime to those.
	limited := []RuntimeKind{{Kind: KindSQS}, {Kind: "mix"}}
	assert.True(t, Carries(limited, KindSQS))
	assert.True(t, Carries(limited, "mix"))
	assert.False(t, Carries(limited, KindSFTP))
	assert.False(t, Carries(limited, KindHTTP))
}

func TestRunKind(t *testing.T) {
	assert.Equal(t, KindSFTP, RunKind(Sftp{}))
	assert.Equal(t, KindSQS, RunKind(Sqs{}))
	assert.Equal(t, "camel-route", RunKind(Custom{Type: "camel-route"}))
}

func TestTestRequest_EffectiveStatus(t *testing.T) {
	at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, status := range []string{TestPending, TestRunning} {
		req := TestRequest{Status: status, ExpiresAt: at.Add(TestRequestTTL)}
		assert.Equal(t, status, req.EffectiveStatus(at), status)
		assert.Equal(t, status, req.EffectiveStatus(at.Add(TestRequestTTL-time.Second)), status)
		assert.Equal(t, TestExpired, req.EffectiveStatus(at.Add(TestRequestTTL)), "open past expiry: "+status)
	}
	done := TestRequest{Status: TestDone, ExpiresAt: at}
	assert.Equal(t, TestDone, done.EffectiveStatus(at.Add(time.Hour)), "an answer does not expire")
}
