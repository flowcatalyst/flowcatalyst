package operations

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// CertificateCommand is the input of the three certificate operations: the
// provider whose certificates change.
type CertificateCommand struct {
	IdentityProviderID string `json:"identityProviderId"`
}

// CertificateResult names the certificate an operation produced or removed.
type CertificateResult struct {
	CertificateID string `json:"certificateId"`
	// Status is the status the certificate holds after the operation (empty for
	// a discard, which removes it).
	Status string `json:"status,omitempty"`
}

func validateCertificateCommand(_ context.Context, cmd CertificateCommand) error {
	if strings.TrimSpace(cmd.IdentityProviderID) == "" {
		return usecase.Validation("ID_REQUIRED", "identityProviderId is required")
	}
	return nil
}

// loadOIDCProvider finds the provider a certificate operation acts on.
func loadOIDCProvider(ctx context.Context, deps Deps, id string) (*identityprovider.IdentityProvider, error) {
	ip, err := deps.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", id)
	}
	if ip.Type != identityprovider.TypeOIDC {
		return nil, usecase.Validation("IDENTITY_PROVIDER_NOT_OIDC",
			"Client certificates exist only for OIDC identity providers")
	}
	return ip, nil
}

// GenerateIdentityProviderCertificate creates a platform-generated client
// certificate for an OIDC identity provider. With no ACTIVE certificate the
// new one becomes ACTIVE; otherwise it becomes NEXT, replacing any NEXT. The
// private key stays in the platform (sealed with the app key); the admin
// downloads the public certificate and registers it with the provider. The
// coarse anchor-only "may write identity providers" permission is enforced at
// the controller.
func GenerateIdentityProviderCertificate(deps Deps) usecaseop.TxOperation[CertificateCommand, CertificateResult] {
	return usecaseop.TxOperation[CertificateCommand, CertificateResult]{
		Name:      "GenerateIdentityProviderCertificate",
		Validate:  validateCertificateCommand,
		Authorize: usecaseop.Public[CertificateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CertificateCommand, ec usecase.ExecutionContext) (CertificateResult, error) {
			var zero CertificateResult
			ip, err := loadOIDCProvider(ctx, deps, cmd.IdentityProviderID)
			if err != nil {
				return zero, err
			}
			if deps.Enc == nil {
				return zero, usecase.Validation("ENCRYPTION_NOT_CONFIGURED",
					"cannot generate a certificate: FLOWCATALYST_APP_KEY is not configured")
			}
			active, err := deps.Repo.FindCertificate(ctx, ip.ID, identityprovider.CertificateActive)
			if err != nil {
				return zero, usecase.Internal("REPO", "find active certificate failed", err)
			}
			next, err := deps.Repo.FindCertificate(ctx, ip.ID, identityprovider.CertificateNext)
			if err != nil {
				return zero, usecase.Internal("REPO", "find next certificate failed", err)
			}
			cert, err := identityprovider.GenerateCertificate(deps.Enc, ip.ID, ip.Code, time.Now())
			if err != nil {
				return zero, usecase.Internal("CERTIFICATE_GENERATE", "certificate generation failed", err)
			}
			cert.Status = identityprovider.CertificateNext
			if active == nil {
				cert.Status = identityprovider.CertificateActive
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if cert.Status == identityprovider.CertificateNext && next != nil {
					if err := deps.Repo.DeleteCertificate(ctx, tx, next.ID); err != nil {
						return err
					}
				}
				return deps.Repo.InsertCertificate(ctx, tx, cert)
			}); err != nil {
				return zero, usecase.Internal("REPO", "certificate store failed", err)
			}
			event := CertificateGenerated{
				certEventBase:  newCertEvent(ec, CertificateGeneratedType, ip.ID),
				Code:           ip.Code,
				CertificateID:  cert.ID,
				Status:         string(cert.Status),
				ThumbprintSHA1: cert.ThumbprintSHA1,
				NotAfter:       cert.NotAfter,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return CertificateResult{CertificateID: cert.ID, Status: string(cert.Status)}, nil
		},
	}
}

// PromoteIdentityProviderCertificate makes the NEXT certificate ACTIVE and
// deletes the certificate it replaces. Refused (NO_NEXT_CERTIFICATE) when there
// is no NEXT.
func PromoteIdentityProviderCertificate(deps Deps) usecaseop.TxOperation[CertificateCommand, CertificateResult] {
	return usecaseop.TxOperation[CertificateCommand, CertificateResult]{
		Name:      "PromoteIdentityProviderCertificate",
		Validate:  validateCertificateCommand,
		Authorize: usecaseop.Public[CertificateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CertificateCommand, ec usecase.ExecutionContext) (CertificateResult, error) {
			var zero CertificateResult
			ip, err := loadOIDCProvider(ctx, deps, cmd.IdentityProviderID)
			if err != nil {
				return zero, err
			}
			next, err := deps.Repo.FindCertificate(ctx, ip.ID, identityprovider.CertificateNext)
			if err != nil {
				return zero, usecase.Internal("REPO", "find next certificate failed", err)
			}
			if next == nil {
				return zero, usecase.BusinessRule("NO_NEXT_CERTIFICATE", "There is no next certificate to make active")
			}
			active, err := deps.Repo.FindCertificate(ctx, ip.ID, identityprovider.CertificateActive)
			if err != nil {
				return zero, usecase.Internal("REPO", "find active certificate failed", err)
			}
			retired := ""
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				// The (provider, status) pair is unique: the old ACTIVE goes first.
				if active != nil {
					retired = active.ID
					if err := deps.Repo.DeleteCertificate(ctx, tx, active.ID); err != nil {
						return err
					}
				}
				return deps.Repo.SetCertificateStatus(ctx, tx, next.ID, identityprovider.CertificateActive)
			}); err != nil {
				return zero, usecase.Internal("REPO", "certificate promote failed", err)
			}
			event := CertificatePromoted{
				certEventBase:        newCertEvent(ec, CertificatePromotedType, ip.ID),
				Code:                 ip.Code,
				CertificateID:        next.ID,
				RetiredCertificateID: retired,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return CertificateResult{CertificateID: next.ID, Status: string(identityprovider.CertificateActive)}, nil
		},
	}
}

// DiscardIdentityProviderCertificate deletes the NEXT certificate. Refused
// (NO_NEXT_CERTIFICATE) when there is none.
func DiscardIdentityProviderCertificate(deps Deps) usecaseop.TxOperation[CertificateCommand, CertificateResult] {
	return usecaseop.TxOperation[CertificateCommand, CertificateResult]{
		Name:      "DiscardIdentityProviderCertificate",
		Validate:  validateCertificateCommand,
		Authorize: usecaseop.Public[CertificateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CertificateCommand, ec usecase.ExecutionContext) (CertificateResult, error) {
			var zero CertificateResult
			ip, err := loadOIDCProvider(ctx, deps, cmd.IdentityProviderID)
			if err != nil {
				return zero, err
			}
			next, err := deps.Repo.FindCertificate(ctx, ip.ID, identityprovider.CertificateNext)
			if err != nil {
				return zero, usecase.Internal("REPO", "find next certificate failed", err)
			}
			if next == nil {
				return zero, usecase.BusinessRule("NO_NEXT_CERTIFICATE", "There is no next certificate to discard")
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return deps.Repo.DeleteCertificate(ctx, tx, next.ID)
			}); err != nil {
				return zero, usecase.Internal("REPO", "certificate discard failed", err)
			}
			event := CertificateDiscarded{
				certEventBase: newCertEvent(ec, CertificateDiscardedType, ip.ID),
				Code:          ip.Code,
				CertificateID: next.ID,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return CertificateResult{CertificateID: next.ID}, nil
		},
	}
}

func newCertEvent(ec usecase.ExecutionContext, eventType, idpID string) certEventBase {
	return certEventBase{
		Metadata:           usecase.NewEventMetadata(ec, eventType, Source, subjectFor(idpID)),
		eventType:          eventType,
		IdentityProviderID: idpID,
	}
}
