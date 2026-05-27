package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RegisterCommand is the input DTO for the FINISH-registration step.
// The BEGIN step is a read-only handler (no UoW commit) that issues the
// challenge — see api/api.go.
type RegisterCommand struct {
	StateID  string                `json:"stateId"`
	Response gowebauthn.Credential `json:"-"` // assembled from the *http.Request before being called
	Name     *string               `json:"name,omitempty"`
}

// Register persists the verified credential and emits [PasskeyRegistered].
//
// The caller (the HTTP handler in api/api.go) has already:
//  1. Consumed the ceremony state from CeremonyRepository
//  2. Invoked webauthn.Service.WebAuthn().FinishRegistration to verify
//     the attestation and produce the gowebauthn.Credential
//  3. Populated cmd.Response and cmd.Name
//
// Register's only job is to wrap the verified credential in our Credential
// entity and commit via UoW. This split keeps the use case body free of
// HTTP / library state — single-responsibility.
func Register(
	ctx context.Context,
	creds *webauthn.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd RegisterCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[PasskeyRegistered], error) {
	var zero commit.Committed[PasskeyRegistered]

	if strings.TrimSpace(cmd.StateID) == "" {
		return zero, usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
	}

	credential := webauthn.New(ec.PrincipalID, cmd.Response, cmd.Name)

	event := PasskeyRegistered{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyRegisteredType, Source, subjectFor(credential.ID)),
		CredentialID: credential.ID,
		UserID:       ec.PrincipalID,
		Name:         cmd.Name,
	}
	return commit.Save(ctx, uow, credential, creds, event, cmd)
}
