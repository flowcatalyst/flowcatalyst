package sdk

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// How an ingest route answers a payload the payload service would not store
// (docs/payload-offload-design.md, sections 7 and 8): PAYLOAD_TOO_LARGE
// whether or not the payload table is written (FC_PAYLOAD_TABLE_WRITES), the
// others only while it is. The error codes start the
// per-item error of a batch route and are the code of a single route's error.
const (
	// PayloadTooLargeCode: the payload is larger than the largest accepted.
	// Permanent.
	PayloadTooLargeCode = "PAYLOAD_TOO_LARGE"
	// PayloadNotTextCode: the payload is not UTF-8 text (or holds a NUL
	// byte), which the payload table cannot hold. Permanent.
	PayloadNotTextCode = "PAYLOAD_NOT_TEXT"
	// PayloadStoreUnavailableCode: the payload store could not be written and
	// the payload is too large to keep in the database. Retryable: send it
	// again later.
	PayloadStoreUnavailableCode = "PAYLOAD_STORE_UNAVAILABLE"
)

// The per-item statuses of the batch routes (the outbox processor's
// OutboxStatus): BAD_REQUEST is terminal, INTERNAL_ERROR retried.
const (
	ItemStatusBadRequest    = "BAD_REQUEST"
	ItemStatusInternalError = "INTERNAL_ERROR"
)

const storeUnavailableMessage = "the payload store could not be written and the payload is too large to keep in the database; send it again later"

// PayloadItemResult is a batch item's status and error for err, a refusal by
// the payload service (an error wrapping payloadstore.ErrTooLarge,
// ErrNotText or ErrStoreUnavailable): BAD_REQUEST for a payload refused for
// good, INTERNAL_ERROR (retried by the outbox processor) for one that may be
// stored later. The error starts with the code. Anything else is answered as
// the store being unavailable, so it is retried rather than dropped.
func PayloadItemResult(err error) (status, message string) {
	switch {
	case errors.Is(err, payloadstore.ErrTooLarge):
		return ItemStatusBadRequest, PayloadTooLargeCode + ": " + err.Error()
	case errors.Is(err, payloadstore.ErrNotText):
		return ItemStatusBadRequest, PayloadNotTextCode + ": " + err.Error()
	default:
		return ItemStatusInternalError, PayloadStoreUnavailableCode + ": " + storeUnavailableMessage
	}
}

// PayloadError is the error a single-item route returns for err, a refusal by
// the payload service: 400 PAYLOAD_TOO_LARGE or PAYLOAD_NOT_TEXT (the
// repository's other size refusals are 400s too), or 503
// PAYLOAD_STORE_UNAVAILABLE, for a huma route. Nil when err is not one.
func PayloadError(err error) error {
	switch {
	case errors.Is(err, payloadstore.ErrTooLarge):
		return usecase.Validation(PayloadTooLargeCode, err.Error())
	case errors.Is(err, payloadstore.ErrNotText):
		return usecase.Validation(PayloadNotTextCode, err.Error())
	case errors.Is(err, payloadstore.ErrStoreUnavailable):
		return httpcompat.NewStatusError(http.StatusServiceUnavailable, PayloadStoreUnavailableCode, storeUnavailableMessage)
	default:
		return nil
	}
}

// writePayloadError answers a chi route with PayloadError's error for err,
// in the canonical envelope; false (nothing written) when err is not a
// refusal by the payload service.
func writePayloadError(w http.ResponseWriter, err error) bool {
	pe := PayloadError(err)
	if pe == nil {
		return false
	}
	var se *httpcompat.ErrorModel
	if !errors.As(pe, &se) {
		httperror.Write(w, pe)
		return true
	}
	writeErrorModel(w, se)
	return true
}

// writeErrorModel answers a chi route with se's status, in the canonical
// envelope.
func writeErrorModel(w http.ResponseWriter, se *httpcompat.ErrorModel) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(se.GetStatus())
	_ = json.NewEncoder(w).Encode(httperror.Envelope{Code: se.Code, Message: se.Message})
}
