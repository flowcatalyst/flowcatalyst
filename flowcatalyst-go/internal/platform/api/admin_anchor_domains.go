package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/client"
)

// AnchorDomainHandler handles anchor domain admin API requests
type AnchorDomainHandler struct {
	clientRepo client.Repository
}

// NewAnchorDomainHandler creates a new anchor domain handler
func NewAnchorDomainHandler(clientRepo client.Repository) *AnchorDomainHandler {
	return &AnchorDomainHandler{
		clientRepo: clientRepo,
	}
}

// AnchorDomainDTO is the response DTO for anchor domain
type AnchorDomainDTO struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"createdAt"`
}

// CreateAnchorDomainRequest is the request to add an anchor domain
type CreateAnchorDomainRequest struct {
	Domain string `json:"domain"`
}

// List handles GET /api/admin/platform/anchor-domains
//
//	@Summary		List anchor domains
//	@Description	Returns all anchor domains that grant platform-wide access
//	@Tags			Anchor Domains
//	@Produce		json
//	@Success		200	{array}		AnchorDomainDTO
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/anchor-domains [get]
func (h *AnchorDomainHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	domains, err := h.clientRepo.FindAnchorDomains(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to fetch anchor domains")
		return
	}

	dtos := make([]AnchorDomainDTO, len(domains))
	for i, d := range domains {
		dtos[i] = AnchorDomainDTO{
			ID:        d.ID,
			Domain:    d.Domain,
			CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	WriteJSON(w, http.StatusOK, dtos)
}

// Create handles POST /api/admin/platform/anchor-domains
//
//	@Summary		Add anchor domain
//	@Description	Adds a new anchor domain that grants platform-wide access to users
//	@Tags			Anchor Domains
//	@Accept			json
//	@Produce		json
//	@Param			domain	body		CreateAnchorDomainRequest	true	"Anchor domain to add"
//	@Success		201		{object}	AnchorDomainDTO
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/anchor-domains [post]
func (h *AnchorDomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateAnchorDomainRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Domain == "" {
		WriteBadRequest(w, "domain is required")
		return
	}

	domain := &client.AnchorDomain{
		Domain: req.Domain,
	}

	if err := h.clientRepo.AddAnchorDomain(ctx, domain); err != nil {
		if errors.Is(err, client.ErrDuplicateDomain) {
			WriteConflict(w, "Anchor domain already exists")
			return
		}
		WriteInternalError(w, "Failed to add anchor domain")
		return
	}

	WriteJSON(w, http.StatusCreated, AnchorDomainDTO{
		ID:        domain.ID,
		Domain:    domain.Domain,
		CreatedAt: domain.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// Delete handles DELETE /api/admin/platform/anchor-domains/{domain}
//
//	@Summary		Remove anchor domain
//	@Description	Removes an anchor domain
//	@Tags			Anchor Domains
//	@Param			domain	path	string	true	"Domain to remove"
//	@Success		204		"No content"
//	@Failure		401		{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/anchor-domains/{domain} [delete]
func (h *AnchorDomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	domain := chi.URLParam(r, "domain")

	if err := h.clientRepo.RemoveAnchorDomain(ctx, domain); err != nil {
		WriteInternalError(w, "Failed to remove anchor domain")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
