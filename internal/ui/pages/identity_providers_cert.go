package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/bridge"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The client credential section of the identity provider drawer
// (docs/oidc-client-credentials-spec.md): a client secret, a platform-generated
// certificate, or a federated credential the platform signs itself.

const (
	idpCertPromoteDialog = "idp-cert-promote"
	idpCertDiscardDialog = "idp-cert-discard"

	// idpCertExpiryWarning is how far ahead an ACTIVE certificate's expiry is
	// warned about.
	idpCertExpiryWarning = 60 * 24 * time.Hour
)

// idpCertRow is one certificate in the drawer's table.
type idpCertRow struct {
	Status       string
	Thumbprint   string
	Expires      string
	ExpiresISO   string
	DownloadHref string
	Expired      bool
	ExpiresSoon  bool
}

// idpCredential is what the drawer shows of how the provider authenticates.
type idpCredential struct {
	Method      string
	MethodLabel string

	// Certificate method. Controls is false when the page was built without the
	// certificate store and operations.
	Controls  bool
	ReadError bool
	Certs     []idpCertRow
	// CanGenerate: no certificate yet. CanGenerateNext: ACTIVE and no NEXT.
	// CanPromote and CanDiscard: a NEXT exists.
	CanGenerate, CanGenerateNext, CanPromote, CanDiscard bool
	// ExpiryWarning is set when the ACTIVE certificate is expired or expires
	// within 60 days.
	ExpiryWarning string
	GenerateAct   string
	PromoteAct    string
	DiscardAct    string

	// Federated method: what to enter at the provider.
	FedIssuer, FedSubject, FedAudience string
	// FedIssuerWarn: the platform issuer is not a public https URL, so the
	// provider cannot reach the signing keys.
	FedIssuerWarn bool
}

func idpMethodLabel(m identityprovider.ClientAuthMethod) string {
	switch m {
	case identityprovider.ClientAuthCertificate:
		return "Certificate"
	case identityprovider.ClientAuthFederated:
		return "Federated credential"
	case identityprovider.ClientAuthSecret:
		return "Client secret"
	}
	return "Client secret"
}

// credentialFor builds the drawer's client credential view of an OIDC provider.
func (h *IdentityProviders) credentialFor(ctx context.Context, p *identityprovider.IdentityProvider, perms idpPerms) idpCredential {
	method := p.OIDCClientAuthMethod
	if method == "" {
		method = identityprovider.ClientAuthSecret
	}
	c := idpCredential{
		Method: string(method), MethodLabel: idpMethodLabel(method),
		GenerateAct: idpURL(p.ID) + "/certificates/generate",
		PromoteAct:  idpURL(p.ID) + "/certificates/promote",
		DiscardAct:  idpURL(p.ID) + "/certificates/discard",
	}
	switch method {
	case identityprovider.ClientAuthFederated:
		c.FedIssuer = h.Issuer
		c.FedSubject = bridge.FederatedSubjectPrefix + p.ID
		c.FedAudience = bridge.FederatedAudience
		c.FedIssuerWarn = !bridge.IssuerIsPublicHTTPS(h.Issuer)
	case identityprovider.ClientAuthCertificate:
		c.Controls = h.Certs != nil && h.CertOps != nil
		if h.Certs == nil {
			return c
		}
		certs, err := h.Certs.FindCertificates(ctx, p.ID)
		if err != nil {
			slog.WarnContext(ctx, "ui: certificates unavailable for the identity provider drawer", "err", err)
			c.ReadError = true
			return c
		}
		now := time.Now()
		var hasActive, hasNext bool
		for i := range certs {
			cert := &certs[i]
			row := idpCertRow{
				Status: string(cert.Status), Thumbprint: cert.ThumbprintSHA1,
				Expires: cert.NotAfter.UTC().Format("2006-01-02"), ExpiresISO: cert.NotAfter.UTC().Format("2006-01-02T15:04:05Z"),
				DownloadHref: idpURL(p.ID) + "/certificates/" + url.PathEscape(cert.ID) + "/download",
				Expired:      cert.NotAfter.Before(now), ExpiresSoon: cert.ExpiresWithin(now, idpCertExpiryWarning),
			}
			c.Certs = append(c.Certs, row)
			switch cert.Status {
			case identityprovider.CertificateActive:
				hasActive = true
				switch {
				case row.Expired:
					c.ExpiryWarning = "The active certificate expired on " + row.Expires + ". Sign-ins through this provider fail until a new certificate is active."
				case row.ExpiresSoon:
					c.ExpiryWarning = "The active certificate expires on " + row.Expires + ". Generate the next certificate, upload it to the provider, then make it active."
				}
			case identityprovider.CertificateNext:
				hasNext = true
			}
		}
		if c.Controls && perms.Update {
			c.CanGenerate = !hasActive && !hasNext
			c.CanGenerateNext = hasActive && !hasNext
			c.CanPromote, c.CanDiscard = hasNext, hasNext
		}
	case identityprovider.ClientAuthSecret:
		// The client secret has its own field on the form; nothing to show here.
	}
	return c
}

type certOp int

const (
	certGenerate certOp = iota
	certPromote
	certDiscard
)

// certAction runs one certificate operation from the drawer and answers with
// the drawer re-rendered. The check is the update permission, as the provider's
// edit.
func (h *IdentityProviders) certAction(op certOp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac := auth.FromContext(ctx)
		if err := firstError(auth.CanReadIdentityProviders(ac), auth.CanUpdateIdentityProviders(ac)); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		id, err := idpID(r)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		if h.CertOps == nil {
			h.Shell.Fail(w, r, usecase.BusinessRule("CERTIFICATES_UNAVAILABLE", "Client certificates are not available on this server"))
			return
		}
		var flash string
		switch op {
		case certGenerate:
			_, err = h.CertOps.GenerateCertificate(ctx, id)
			flash = flashIdpCertGenerated
		case certPromote:
			_, err = h.CertOps.PromoteCertificate(ctx, id)
			flash = flashIdpCertPromoted
		case certDiscard:
			_, err = h.CertOps.DiscardCertificate(ctx, id)
			flash = flashIdpCertDiscarded
		}
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		if !ds.IsDatastar(r) {
			h.Shell.SetFlash(w, flash)
			http.Redirect(w, r, idpURL(id), http.StatusSeeOther) //nolint:gosec // G710: a path under idpPath, the id path-escaped
			return
		}
		sse := ds.NewSSE(w, r)
		p, err := h.load(ctx, id)
		if err != nil {
			h.Shell.FailSSE(sse, r, err)
			return
		}
		if err := errors.Join(
			sse.Patch(idpDrawerContent(h.toDetail(ctx, ac, p)), ds.Inner("#"+idpDrawerID)),
			flashKey(sse, flash),
		); err != nil {
			slog.DebugContext(ctx, "ui: identity provider certificate patch not delivered", "err", err)
		}
	}
}

// certDownload serves a certificate as DER-encoded X.509 (a ".cer" file) for
// uploading to the provider. The check is the page's read permission.
func (h *IdentityProviders) certDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanReadIdentityProviders(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := idpID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	certID := chi.URLParam(r, "certId")
	if !kit.ValidID(certID) || h.Certs == nil {
		h.Shell.Fail(w, r, httperror.NotFound("Certificate", certID))
		return
	}
	p, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cert, err := h.Certs.FindCertificateByID(ctx, certID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find certificate failed", err))
		return
	}
	if cert == nil || cert.IdentityProviderID != p.ID {
		h.Shell.Fail(w, r, httperror.NotFound("Certificate", certID))
		return
	}
	der, err := cert.DER()
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("CERTIFICATE", "stored certificate is unreadable", err))
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="`+certFileName(p.Code, cert.ThumbprintSHA1)+`.cer"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(der) //nolint:gosec // G705: DER bytes served as application/x-x509-ca-cert, never HTML
}

// certFileName is "<code>-<thumbprint>" reduced to characters safe in a header.
func certFileName(code, thumbprint string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
				return r
			}
			return '-'
		}, s)
	}
	return clean(code) + "-" + clean(thumbprint)
}

// idpCertConfirm is one shared confirm dialog of the certificate section; the
// trigger sets the provider's action.
func idpCertConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: idpPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(idpDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
