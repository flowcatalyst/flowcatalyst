package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// developerRoleName is the seeded role (internal/platform/seed/roles.go)
// that gates the self-service developer client_credentials flow.
const developerRoleName = "platform:developer"

// hasRole reports whether p currently holds roleName, checked against the
// freshly-loaded principal's hydrated Roles. This is the same live
// re-verification the /oauth/token principal-branch performs at mint time —
// checking it here too means SetDeveloperCredential fails fast with a clear
// error instead of silently minting a secret nothing can ever use.
func hasRole(p *principal.Principal, roleName string) bool {
	for _, ra := range p.Roles {
		if ra.Role == roleName {
			return true
		}
	}
	return false
}

// requireSelfOrUserAdmin allows a principal to manage its own developer
// credential unconditionally (the coarse platform:developer:api-credential:manage
// permission is already checked at the controller); managing someone else's
// credential — the Developer Users admin page granting/rotating on behalf of
// another user — requires the same user-admin bar role assignment does.
func requireSelfOrUserAdmin(ctx context.Context, p *principal.Principal) error {
	ac := auth.FromContext(ctx)
	if ac != nil && ac.PrincipalID == p.ID {
		return nil
	}
	return requireUserAdmin(ctx, p)
}

// ── Set (create-or-rotate) ──────────────────────────────────────────────

type SetDeveloperCredentialCommand struct {
	PrincipalID string `json:"principalId"`
}

// SetDeveloperCredential creates or rotates a developer's self-service
// client_credentials secret. The plaintext is returned exactly once via the
// process-local stash (PopDevClientSecret) — the same one-time-disclosure
// pattern service-account secret rotation uses.
func SetDeveloperCredential(repo *principal.Repository) usecaseop.Operation[SetDeveloperCredentialCommand, DeveloperCredentialSet] {
	return usecaseop.Operation[SetDeveloperCredentialCommand, DeveloperCredentialSet]{
		Name: "SetDeveloperCredential",
		Validate: func(_ context.Context, cmd SetDeveloperCredentialCommand) error {
			if strings.TrimSpace(cmd.PrincipalID) == "" {
				return usecase.Validation("PRINCIPAL_ID_REQUIRED", "Principal ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[SetDeveloperCredentialCommand],
		Execute: func(ctx context.Context, cmd SetDeveloperCredentialCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DeveloperCredentialSet], error) {
			p, err := repo.FindByID(ctx, cmd.PrincipalID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.PrincipalID)
			}
			if err := requireSelfOrUserAdmin(ctx, p); err != nil {
				return nil, err
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER",
					"Developer credentials can only be set on USER type principals")
			}
			if !hasRole(p, developerRoleName) {
				return nil, usecase.BusinessRule("NOT_A_DEVELOPER",
					"Principal does not hold the developer role")
			}

			plaintext, ref, err := generateDevClientSecret()
			if err != nil {
				return nil, usecase.Internal("SECRET", "generate developer client secret failed", err)
			}
			p.SetDevClientSecretRef(ref)
			stashDevSecret(p.ID, plaintext)

			event := DeveloperCredentialSet{
				Metadata: usecase.NewEventMetadata(ec, DeveloperCredentialSetType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}

// ── Revoke ───────────────────────────────────────────────────────────────

type RevokeDeveloperCredentialCommand struct {
	PrincipalID string `json:"principalId"`
}

// RevokeDeveloperCredential clears a principal's developer client_credentials
// secret without touching their role — lets a user or admin kill a leaked
// secret and set a fresh one later.
func RevokeDeveloperCredential(repo *principal.Repository) usecaseop.Operation[RevokeDeveloperCredentialCommand, DeveloperCredentialRevoked] {
	return usecaseop.Operation[RevokeDeveloperCredentialCommand, DeveloperCredentialRevoked]{
		Name: "RevokeDeveloperCredential",
		Validate: func(_ context.Context, cmd RevokeDeveloperCredentialCommand) error {
			if strings.TrimSpace(cmd.PrincipalID) == "" {
				return usecase.Validation("PRINCIPAL_ID_REQUIRED", "Principal ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[RevokeDeveloperCredentialCommand],
		Execute: func(ctx context.Context, cmd RevokeDeveloperCredentialCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DeveloperCredentialRevoked], error) {
			p, err := repo.FindByID(ctx, cmd.PrincipalID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.PrincipalID)
			}
			if err := requireSelfOrUserAdmin(ctx, p); err != nil {
				return nil, err
			}
			p.ClearDevClientSecretRef()

			event := DeveloperCredentialRevoked{
				Metadata: usecase.NewEventMetadata(ec, DeveloperCredentialRevokedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}

// ── secret generation + one-time disclosure stash ───────────────────────

// generateDevClientSecret mirrors serviceaccount/operations.generateOAuthClientSecret
// (duplicated rather than cross-imported — this package already follows that
// convention for small resource-checks like blockNonClientTarget): 32 random
// bytes, base64url plaintext, encrypted via the same encryption.Service OAuth
// client secrets use, so /oauth/token's verifyClientSecret can decrypt+compare
// either kind of secret through one shared helper.
func generateDevClientSecret() (plaintext, ref string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	enc, err := encryption.FromEnv()
	if err != nil {
		return "", "", err
	}
	if enc == nil {
		return "", "", errors.New("FLOWCATALYST_APP_KEY not configured; cannot encrypt developer client secret")
	}
	ref, err = enc.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}
	return plaintext, ref, nil
}

// devSecretStash is a process-local one-shot stash keyed by principal id —
// mirrors serviceaccount/operations' stash (duplicated: that one is
// package-private and this is a different aggregate root).
var devSecretStash sync.Map

type devStashEntry struct {
	plaintext string
	storedAt  time.Time
}

// devStashTTL bounds how long an un-popped plaintext may sit in process
// memory — mirrors serviceaccount/operations.stashTTL.
const devStashTTL = 2 * time.Minute

func stashDevSecret(principalID, value string) {
	now := time.Now()
	devSecretStash.Range(func(k, v any) bool {
		if e, ok := v.(devStashEntry); ok && now.Sub(e.storedAt) >= devStashTTL {
			devSecretStash.Delete(k)
		}
		return true
	})
	devSecretStash.Store(principalID, devStashEntry{plaintext: value, storedAt: now})
}

// PopDevClientSecret retrieves and removes a stashed plaintext developer
// secret. Used by the HTTP handler to return the newly-set/rotated secret in
// the response exactly once.
func PopDevClientSecret(principalID string) (string, bool) {
	v, ok := devSecretStash.LoadAndDelete(principalID)
	if !ok {
		return "", false
	}
	e := v.(devStashEntry)
	if time.Since(e.storedAt) >= devStashTTL {
		return "", false
	}
	return e.plaintext, true
}
