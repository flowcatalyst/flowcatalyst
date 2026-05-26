package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateOAuthClientCommand struct {
	ClientID     string   `json:"clientId"`
	ClientName   string   `json:"clientName"`
	ClientType   string   `json:"clientType"`
	RedirectURIs []string `json:"redirectUris,omitempty"`
	GrantTypes   []string `json:"grantTypes,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	PrincipalID  *string  `json:"principalId,omitempty"`
}

type CreateOAuthClientResult struct {
	Event        OAuthClientCreated
	InitialSecret string // returned once for CONFIDENTIAL clients; nil for PUBLIC.
}

type CreateOAuthClientUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewCreateOAuthClientUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *CreateOAuthClientUseCase {
	return &CreateOAuthClientUseCase{repo: repo, uow: uow}
}

func (uc *CreateOAuthClientUseCase) Validate(_ context.Context, cmd CreateOAuthClientCommand) error {
	if strings.TrimSpace(cmd.ClientID) == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if strings.TrimSpace(cmd.ClientName) == "" {
		return usecase.Validation("CLIENT_NAME_REQUIRED", "clientName is required")
	}
	return nil
}

func (uc *CreateOAuthClientUseCase) Authorize(_ context.Context, _ CreateOAuthClientCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateOAuthClientUseCase) Execute(ctx context.Context, cmd CreateOAuthClientCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientCreated] {
	existing, err := uc.repo.FindByClientID(ctx, cmd.ClientID)
	if err != nil {
		return usecase.Failure[OAuthClientCreated](usecase.Internal("REPO", "find_by_client_id failed", err))
	}
	if existing != nil {
		return usecase.Failure[OAuthClientCreated](usecase.Conflict(
			"CLIENT_ID_EXISTS", "OAuth client_id '"+cmd.ClientID+"' already exists"))
	}
	t := auth.ParseOAuthClientType(cmd.ClientType)
	c := auth.NewOAuthClient(cmd.ClientID, cmd.ClientName, t)
	c.RedirectURIs = cmd.RedirectURIs
	c.GrantTypes = cmd.GrantTypes
	c.Scopes = cmd.Scopes
	c.PrincipalID = cmd.PrincipalID
	if t == auth.OAuthClientConfidential {
		// Generate an initial secret. The plaintext is returned to the
		// admin once via the create response; only the hash is stored.
		plaintext, hash := generateSecret()
		c.SetSecretHash(hash)
		// We can't return both an event and a secret through the UoW
		// commit (it returns only the event). The handler reads the
		// plaintext out of band — see api/api.go: after Run() returns
		// success the handler issues a follow-up read of the freshly
		// committed client and the secret comes from this stash.
		stashSecret(c.ID, plaintext)
	}

	event := OAuthClientCreated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientCreatedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
		ClientID:      c.ClientID,
		ClientName:    c.ClientName,
	}
	return usecasepgx.Commit[auth.OAuthClient, OAuthClientCreated, CreateOAuthClientCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateOAuthClientCommand, OAuthClientCreated] = (*CreateOAuthClientUseCase)(nil)

// ── Update ────────────────────────────────────────────────────────────────

type UpdateOAuthClientCommand struct {
	ID           string   `json:"id"`
	ClientName   *string  `json:"clientName,omitempty"`
	RedirectURIs []string `json:"redirectUris,omitempty"`
	GrantTypes   []string `json:"grantTypes,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type UpdateOAuthClientUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewUpdateOAuthClientUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *UpdateOAuthClientUseCase {
	return &UpdateOAuthClientUseCase{repo: repo, uow: uow}
}

func (uc *UpdateOAuthClientUseCase) Validate(_ context.Context, cmd UpdateOAuthClientCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.ClientName != nil && strings.TrimSpace(*cmd.ClientName) == "" {
		return usecase.Validation("CLIENT_NAME_REQUIRED", "clientName cannot be empty")
	}
	return nil
}

func (uc *UpdateOAuthClientUseCase) Authorize(_ context.Context, _ UpdateOAuthClientCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateOAuthClientUseCase) Execute(ctx context.Context, cmd UpdateOAuthClientCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientUpdated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[OAuthClientUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[OAuthClientUpdated](httperror.NotFound("OAuthClient", cmd.ID))
	}
	if cmd.ClientName != nil {
		c.ClientName = strings.TrimSpace(*cmd.ClientName)
	}
	if cmd.RedirectURIs != nil {
		c.RedirectURIs = cmd.RedirectURIs
	}
	if cmd.GrantTypes != nil {
		c.GrantTypes = cmd.GrantTypes
	}
	if cmd.Scopes != nil {
		c.Scopes = cmd.Scopes
	}

	event := OAuthClientUpdated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientUpdatedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
		ClientName:    c.ClientName,
	}
	return usecasepgx.Commit[auth.OAuthClient, OAuthClientUpdated, UpdateOAuthClientCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateOAuthClientCommand, OAuthClientUpdated] = (*UpdateOAuthClientUseCase)(nil)

// ── Activate ──────────────────────────────────────────────────────────────

type ActivateOAuthClientCommand struct {
	ID string `json:"id"`
}

type ActivateOAuthClientUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewActivateOAuthClientUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *ActivateOAuthClientUseCase {
	return &ActivateOAuthClientUseCase{repo: repo, uow: uow}
}

func (uc *ActivateOAuthClientUseCase) Validate(_ context.Context, cmd ActivateOAuthClientCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *ActivateOAuthClientUseCase) Authorize(_ context.Context, _ ActivateOAuthClientCommand, _ usecase.ExecutionContext) error {
	return nil
}
func (uc *ActivateOAuthClientUseCase) Execute(ctx context.Context, cmd ActivateOAuthClientCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientActivated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[OAuthClientActivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[OAuthClientActivated](httperror.NotFound("OAuthClient", cmd.ID))
	}
	c.Activate()
	event := OAuthClientActivated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientActivatedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
	}
	return usecasepgx.Commit[auth.OAuthClient, OAuthClientActivated, ActivateOAuthClientCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ActivateOAuthClientCommand, OAuthClientActivated] = (*ActivateOAuthClientUseCase)(nil)

// ── Deactivate ────────────────────────────────────────────────────────────

type DeactivateOAuthClientCommand struct {
	ID string `json:"id"`
}

type DeactivateOAuthClientUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewDeactivateOAuthClientUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *DeactivateOAuthClientUseCase {
	return &DeactivateOAuthClientUseCase{repo: repo, uow: uow}
}

func (uc *DeactivateOAuthClientUseCase) Validate(_ context.Context, cmd DeactivateOAuthClientCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *DeactivateOAuthClientUseCase) Authorize(_ context.Context, _ DeactivateOAuthClientCommand, _ usecase.ExecutionContext) error {
	return nil
}
func (uc *DeactivateOAuthClientUseCase) Execute(ctx context.Context, cmd DeactivateOAuthClientCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientDeactivated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[OAuthClientDeactivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[OAuthClientDeactivated](httperror.NotFound("OAuthClient", cmd.ID))
	}
	c.Deactivate()
	event := OAuthClientDeactivated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientDeactivatedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
	}
	return usecasepgx.Commit[auth.OAuthClient, OAuthClientDeactivated, DeactivateOAuthClientCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeactivateOAuthClientCommand, OAuthClientDeactivated] = (*DeactivateOAuthClientUseCase)(nil)

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteOAuthClientCommand struct {
	ID string `json:"id"`
}

type DeleteOAuthClientUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewDeleteOAuthClientUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *DeleteOAuthClientUseCase {
	return &DeleteOAuthClientUseCase{repo: repo, uow: uow}
}

func (uc *DeleteOAuthClientUseCase) Validate(_ context.Context, cmd DeleteOAuthClientCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *DeleteOAuthClientUseCase) Authorize(_ context.Context, _ DeleteOAuthClientCommand, _ usecase.ExecutionContext) error {
	return nil
}
func (uc *DeleteOAuthClientUseCase) Execute(ctx context.Context, cmd DeleteOAuthClientCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientDeleted] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[OAuthClientDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[OAuthClientDeleted](httperror.NotFound("OAuthClient", cmd.ID))
	}
	event := OAuthClientDeleted{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientDeletedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
		ClientID:      c.ClientID,
	}
	return usecasepgx.CommitDelete[auth.OAuthClient, OAuthClientDeleted, DeleteOAuthClientCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteOAuthClientCommand, OAuthClientDeleted] = (*DeleteOAuthClientUseCase)(nil)

// ── RotateSecret ──────────────────────────────────────────────────────────

type RotateOAuthClientSecretCommand struct {
	ID string `json:"id"`
}

type RotateOAuthClientSecretUseCase struct {
	repo *auth.OAuthClientRepo
	uow  *usecasepgx.UnitOfWork
}

func NewRotateOAuthClientSecretUseCase(repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork) *RotateOAuthClientSecretUseCase {
	return &RotateOAuthClientSecretUseCase{repo: repo, uow: uow}
}

func (uc *RotateOAuthClientSecretUseCase) Validate(_ context.Context, cmd RotateOAuthClientSecretCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *RotateOAuthClientSecretUseCase) Authorize(_ context.Context, _ RotateOAuthClientSecretCommand, _ usecase.ExecutionContext) error {
	return nil
}
func (uc *RotateOAuthClientSecretUseCase) Execute(ctx context.Context, cmd RotateOAuthClientSecretCommand, ec usecase.ExecutionContext) usecase.Result[OAuthClientSecretRotated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[OAuthClientSecretRotated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[OAuthClientSecretRotated](httperror.NotFound("OAuthClient", cmd.ID))
	}
	if c.ClientType != auth.OAuthClientConfidential {
		return usecase.Failure[OAuthClientSecretRotated](usecase.Conflict(
			"NOT_CONFIDENTIAL", "Only CONFIDENTIAL clients have rotatable secrets"))
	}
	plaintext, hash := generateSecret()
	c.SetSecretHash(hash)
	stashSecret(c.ID, plaintext)

	event := OAuthClientSecretRotated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientSecretRotatedType, Source, oauthSubject(c.ID)),
		OAuthClientID: c.ID,
	}
	return usecasepgx.Commit[auth.OAuthClient, OAuthClientSecretRotated, RotateOAuthClientSecretCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[RotateOAuthClientSecretCommand, OAuthClientSecretRotated] = (*RotateOAuthClientSecretUseCase)(nil)

// ── helpers ───────────────────────────────────────────────────────────────

// generateSecret returns a (plaintext, argon2-PHC-hash) pair for a fresh
// 32-byte URL-safe base64 secret. The plaintext is returned once via
// stashSecret + the API handler reading it out of band. The hash uses
// the shared passwordhash scheme — same envelope that fosite's
// `Argon2idHasher.Compare` validates against on every token request.
func generateSecret() (plaintext, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	h, err := passwordhash.Hash(plaintext)
	if err != nil {
		// crypto/rand failure during salt generation — extremely rare.
		// Panic surfaces the broken RNG immediately rather than minting
		// an unverifiable client.
		panic("oauth_client: passwordhash.Hash: " + err.Error())
	}
	hash = h
	return plaintext, hash
}

// stashSecret keeps the plaintext available to the HTTP handler that
// initiated the operation. The cmd is the same goroutine; we use a
// process-local sync.Map keyed by clientID. The handler reads it back
// once and removes the entry.
//
// This is a deliberately scoped escape hatch: the UoW Commit returns
// the event (no plaintext); the handler needs the plaintext to return
// to the admin in the HTTP response.
//
// TODO(auth-runtime): replace with a context-scoped channel or a
// caller-supplied SecretEcho closure passed into the use case.
func stashSecret(clientID, plaintext string) { secretStash.Store(clientID, plaintext) }

// PopStashedSecret returns the once-readable plaintext for clientID.
// Called by the HTTP handler immediately after Run() succeeds.
func PopStashedSecret(clientID string) (string, bool) {
	v, ok := secretStash.LoadAndDelete(clientID)
	if !ok {
		return "", false
	}
	return v.(string), true
}
