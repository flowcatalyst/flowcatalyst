package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// TestWebhookCredentialsDTO_ToEntity_UnknownAuthTypeRejects is the X-06 write
// boundary: an unrecognised authType on the wire must fail the request with
// a validation error rather than silently becoming NONE (which would ship an
// unauthenticated webhook with no error surfaced to the caller).
func TestWebhookCredentialsDTO_ToEntity_UnknownAuthTypeRejects(t *testing.T) {
	dto := WebhookCredentialsDTO{AuthType: "BEARER_TOEKN"} // typo
	_, err := dto.toEntity()
	require.Error(t, err)

	var ucErr *usecase.Error
	require.ErrorAs(t, err, &ucErr)
	assert.Equal(t, usecase.KindValidation, ucErr.Kind, "must be a 400-shaped validation error, not silently coerced")
	assert.Equal(t, "INVALID_AUTH_TYPE", ucErr.Code)
}

// TestWebhookCredentialsDTO_ToEntity_KnownAuthTypePasses pins the happy path.
func TestWebhookCredentialsDTO_ToEntity_KnownAuthTypePasses(t *testing.T) {
	dto := WebhookCredentialsDTO{AuthType: "BEARER_TOKEN"}
	got, err := dto.toEntity()
	require.NoError(t, err)
	assert.Equal(t, serviceaccount.AuthBearer, got.AuthType)
}

// TestWebhookCredentialsDTO_ToEntity_EmptyAuthTypeMeansNone preserves the
// existing "authType omitted" behaviour: it is a legitimate explicit NONE,
// not a rejected value.
func TestWebhookCredentialsDTO_ToEntity_EmptyAuthTypeMeansNone(t *testing.T) {
	dto := WebhookCredentialsDTO{}
	got, err := dto.toEntity()
	require.NoError(t, err)
	assert.Equal(t, serviceaccount.AuthNone, got.AuthType)
}

// TestCreateServiceAccountRequest_ToCommand_PropagatesAuthTypeError ensures
// the rejection actually reaches the handler boundary through toCommand, not
// just the leaf DTO helper.
func TestCreateServiceAccountRequest_ToCommand_PropagatesAuthTypeError(t *testing.T) {
	req := CreateServiceAccountRequest{
		Code: "svc",
		Name: "Svc",
		WebhookCredentials: &WebhookCredentialsDTO{
			AuthType: "NOT_A_REAL_TYPE",
		},
	}
	_, err := req.toCommand()
	require.Error(t, err)
	var ucErr *usecase.Error
	require.ErrorAs(t, err, &ucErr)
	assert.Equal(t, usecase.KindValidation, ucErr.Kind)
}

// TestUpdateServiceAccountRequest_ToCommand_PropagatesAuthTypeError mirrors
// the create-path test for the update DTO.
func TestUpdateServiceAccountRequest_ToCommand_PropagatesAuthTypeError(t *testing.T) {
	req := UpdateServiceAccountRequest{
		WebhookCredentials: &WebhookCredentialsDTO{
			AuthType: "NOT_A_REAL_TYPE",
		},
	}
	_, err := req.toCommand("sa_123")
	require.Error(t, err)
	var ucErr *usecase.Error
	require.ErrorAs(t, err, &ucErr)
	assert.Equal(t, usecase.KindValidation, ucErr.Kind)
}
