package fieldauth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A signed-in worker's own photo (docs/field-worker-photo-spec.md section 10):
// POST /field/worker/photo (read), /field/worker/photo/set and
// /field/worker/photo/remove. All three sit behind the device proof and the
// access token and act on the token's own worker: nothing in the request
// names a worker, so there is no way to name another.

const (
	// photoBodySlack is what the set route's body may exceed the photo limit by
	// before the reader refuses it (the limit itself is judged by the image
	// package and, first, by Content-Length).
	photoBodySlack = 4 << 10
	photoSlotWait  = 5 * time.Second
)

// bodyLimit is the request body cap of a route: the protocol's small one, or
// the photo limit for the set route.
func (s *State) bodyLimit(rt route) int64 {
	if rt.photo {
		return s.photoMaxBytes() + photoBodySlack
	}
	return maxBody
}

func (s *State) photoMaxBytes() int64 {
	if s.Photos != nil {
		return s.Photos.Limits().MaxBytes
	}
	return imageproc.DefaultLimits().MaxBytes
}

// workerView is the worker of a session's answer: photoVersion when the
// platform and the client have photos on and the worker has one, and
// canSetOwnPhoto when the client's policy also lets the worker set their own
// and the session is the worker's own (a supervisor acting as the worker may
// not).
func (s *State) workerView(ident *fieldidentity.Identity, pol fieldidentity.Policy, impersonated bool) workerRef {
	ref := workerRef{ID: ident.ID, WorkerCode: ident.WorkerCode, Name: ident.Name}
	if s.PhotosDisabled || !pol.PhotosEnabled {
		return ref
	}
	if ident.PhotoVersion != "" {
		v := ident.PhotoVersion
		ref.PhotoVersion = &v
	}
	ref.CanSetOwnPhoto = pol.WorkerMaySetOwnPhoto && !impersonated && s.Photos != nil
	return ref
}

// photoGate is what every photo route checks, in this order: a PIN-change-only
// token is refused (401 invalid_token, as act-as); photos off on the platform
// (404 PHOTOS_DISABLED); the worker's per-hour budget (429); the client's
// enrolled-device rule (401 UNKNOWN_DEVICE). A suspended or revoked device, a
// bad token and an ended session (a suspended worker) were refused by the
// prelude. It writes the refusal itself; ok is false then.
func (s *State) photoGate(c *call, write bool) (fieldidentity.Policy, bool) {
	var none fieldidentity.Policy
	if c.claims.PCR {
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "a PIN-change-only token cannot use photos")
		return none, false
	}
	if s.PhotosDisabled || s.Photos == nil {
		fail(c.w, http.StatusNotFound, fieldidentity.CodePhotosDisabled, "field-worker photos are switched off on this platform")
		return none, false
	}
	bucket, pol := ratelimit.BucketFieldPhotoRead, s.Limits.FieldPhotoRead
	if write {
		bucket, pol = ratelimit.BucketFieldPhotoWrite, s.Limits.FieldPhotoWrite
	}
	if pol.Limit > 0 {
		if rej := ratelimit.Enforce(c.r.Context(), s.RateLimit, bucket, c.claims.Subject, pol); rej != nil {
			ratelimit.WriteTooManyRequests(c.w, rej.RetryAfterSecs, "too many photo requests; try again later")
			return none, false
		}
	}
	// The token's application's effective policy: photos and the enrolled
	// device rule can differ per application.
	app, err := s.findApp(c, c.claims.Aud)
	if err != nil {
		s.internal(c, "application", err)
		return none, false
	}
	worker, err := s.Ops.R.Identities.FindByID(c.r.Context(), c.claims.Subject)
	if err != nil {
		s.internal(c, "worker", err)
		return none, false
	}
	if worker == nil {
		worker = &fieldidentity.Identity{} // gone: the client's policy
	}
	policy, err := s.policyFor(c, c.claims.ClientID, app, worker)
	if err != nil {
		s.internal(c, "policy", err)
		return none, false
	}
	if c.device == nil && policy.RequireEnrolledDevice {
		unknownDevice(c)
		return none, false
	}
	return policy, true
}

// ownWriteGate adds what changing the photo needs: not an impersonating
// session (the opt-in is the worker's own, and the event's actor would be
// wrong), photos on for the client, and the client's opt-in.
func (s *State) ownWriteGate(c *call, pol fieldidentity.Policy) bool {
	switch {
	case c.claims.Act != nil:
		fail(c.w, http.StatusForbidden, "IMPERSONATING", "the photo cannot be changed while signed in as another worker")
	case !pol.PhotosEnabled:
		fail(c.w, http.StatusConflict, fieldidentity.CodePhotosDisabledForClient, "this client has switched field-worker photos off")
	case !pol.WorkerMaySetOwnPhoto:
		fail(c.w, http.StatusForbidden, fieldidentity.CodeOwnPhotoNotAllowed, "this client does not let a worker set their own photo")
	default:
		return true
	}
	return false
}

type readPhotoRequest struct {
	// Size is thumb (the default) or full.
	Size string `json:"size"`
	// Version is the photo version the device already holds: when it is still
	// current the answer is 304 with no body.
	Version string `json:"version"`
}

func etagOf(version string, size fieldidentity.PhotoSize) string {
	return `"` + version + "-" + string(size) + `"`
}

// readPhoto is POST /field/worker/photo: the token's own worker's photo as an
// image/jpeg body (with an ETag), 304 when the version the device holds is
// current, 404 PHOTO_NOT_FOUND when there is none (or the client has photos
// off). A supervisor acting as the worker may read it.
func (s *State) readPhoto(c *call) {
	var req readPhotoRequest
	if !c.decode(&req) {
		return
	}
	size := fieldidentity.PhotoSize(strings.TrimSpace(req.Size))
	if size == "" {
		size = fieldidentity.PhotoThumb
	}
	if !size.Valid() {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "size is thumb or full")
		return
	}
	pol, ok := s.photoGate(c, false)
	if !ok {
		return
	}
	ctx := c.r.Context()
	ident, err := s.Ops.R.Identities.FindByID(ctx, c.claims.Subject)
	if err != nil {
		s.internal(c, "worker", err)
		return
	}
	if ident == nil || ident.ClientID != c.claims.ClientID || !pol.PhotosEnabled || ident.PhotoVersion == "" {
		fail(c.w, http.StatusNotFound, fieldidentity.CodePhotoNotFound, "this worker has no photo")
		return
	}
	etag := etagOf(ident.PhotoVersion, size)
	c.w.Header().Set("ETag", etag)
	c.w.Header().Set("Cache-Control", "private, no-cache")
	if strings.TrimSpace(req.Version) == ident.PhotoVersion {
		c.w.WriteHeader(http.StatusNotModified)
		return
	}
	p, found, err := s.Ops.R.Photos.Rendition(ctx, ident.ID, size)
	if err != nil {
		s.internal(c, "photo", err)
		return
	}
	if !found {
		c.w.Header().Del("ETag")
		fail(c.w, http.StatusNotFound, fieldidentity.CodePhotoNotFound, "this worker has no photo")
		return
	}
	c.w.Header().Set("ETag", etagOf(p.Version, size))
	c.w.Header().Set("Content-Type", p.ContentType)
	c.w.Header().Set("X-Content-Type-Options", "nosniff")
	c.w.WriteHeader(http.StatusOK)
	_, _ = c.w.Write(p.Bytes)
}

// setPhoto is POST /field/worker/photo/set: the body is the image itself, its
// Content-Type the declared type; the same processing and limits as the admin
// API; acts on the token's own worker, as WORKER.
func (s *State) setPhoto(c *call) {
	pol, ok := s.photoGate(c, true)
	if !ok || !s.ownWriteGate(c, pol) {
		return
	}
	max := s.photoMaxBytes()
	refuse := func(err error) {
		if r, known := fieldidentity.PhotoRefusalOf(err, max); known {
			if r.Status == http.StatusServiceUnavailable {
				retryAfter(c.w, 5)
			}
			fail(c.w, r.Status, r.Code, r.Message)
			return
		}
		s.internal(c, "photo", err)
	}
	// The declared length is judged before a byte is read.
	if c.r.ContentLength > max {
		refuse(imageproc.ErrTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(c.r.Body, max+1))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		refuse(imageproc.ErrTooLarge)
		return
	case err != nil:
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "the request body could not be read")
		return
	}
	ctx, cancel := context.WithTimeout(c.r.Context(), photoSlotWait)
	defer cancel()
	photo, err := s.Photos.Process(ctx, data, c.r.Header.Get("Content-Type"))
	if err != nil {
		refuse(err)
		return
	}
	cmd := fieldidentity.NewSetOwnPhotoCommand(fieldidentity.WorkerRef{ID: c.claims.Subject, ClientID: c.claims.ClientID}, photo)
	if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.SetOwnPhoto(), cmd, usecase.NewExecutionContext(c.claims.Subject)); err != nil {
		s.photoUseCaseError(c, err)
		return
	}
	slog.InfoContext(c.r.Context(), "field worker set their own photo", "workerId", c.claims.Subject, "version", photo.Version)
	writeJSON(c.w, http.StatusOK, map[string]any{"photoVersion": photo.Version})
}

// removePhoto is POST /field/worker/photo/remove: the worker removes their own
// photo (the same rule as setting it); removing none is not an error.
func (s *State) removePhoto(c *call) {
	pol, ok := s.photoGate(c, true)
	if !ok || !s.ownWriteGate(c, pol) {
		return
	}
	cmd := fieldidentity.RemoveOwnPhotoCommand{ID: c.claims.Subject, ClientID: c.claims.ClientID}
	if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.RemoveOwnPhoto(), cmd, usecase.NewExecutionContext(c.claims.Subject)); err != nil {
		s.photoUseCaseError(c, err)
		return
	}
	writeJSON(c.w, http.StatusOK, map[string]any{"photoVersion": nil})
}

// photoUseCaseError maps the own-photo use cases' refusals (the gate checked
// the same rules first, so these are races: the policy changed meanwhile).
func (s *State) photoUseCaseError(c *call, err error) {
	if ue := usecase.AsError(err); ue != nil {
		switch ue.Code {
		case fieldidentity.CodePhotosDisabledForClient:
			fail(c.w, http.StatusConflict, ue.Code, ue.Message)
			return
		case fieldidentity.CodeOwnPhotoNotAllowed:
			fail(c.w, http.StatusForbidden, ue.Code, ue.Message)
			return
		}
	}
	useCaseError(c.w, err)
}
