package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Field-worker photos on the admin API (docs/field-worker-photo-spec.md section 5).

// Error codes of the photo routes besides the use cases' own
// (fieldidentity.CodePhotosDisabledForClient and friends).
const (
	// CodePhotosDisabled: FC_FIELD_PHOTOS=off (404).
	CodePhotosDisabled = "PHOTOS_DISABLED"
	// CodePhotoNotFound: the worker has no photo (404).
	CodePhotoNotFound = "PHOTO_NOT_FOUND"
	// The upload refusals, one per imageproc error.
	CodePhotoEmpty         = "PHOTO_EMPTY"
	CodePhotoTooLarge      = "PHOTO_TOO_LARGE"
	CodePhotoUnsupported   = "PHOTO_UNSUPPORTED_TYPE"
	CodePhotoTypeMismatch  = "PHOTO_TYPE_MISMATCH"
	CodePhotoDimensions    = "PHOTO_DIMENSIONS_TOO_LARGE"
	CodePhotoUnreadable    = "PHOTO_UNREADABLE"
	CodePhotoBusy          = "PHOTO_BUSY"
	photoReadTimeout       = 2 * time.Minute
	photoSlotWait          = 5 * time.Second
	photoCacheControl      = "private, no-cache"
	photoDefaultSize       = "thumb"
	photoUploadContentType = "application/octet-stream"
)

// FieldWorkerPhoto says a worker has a photo: Version changes whenever the
// picture does (use it in ?v= and to know when to refetch).
type FieldWorkerPhoto struct {
	Version   string          `json:"version" doc:"Changes whenever the photo does; 16 hex characters"`
	UpdatedAt httpcompat.Time `json:"updatedAt"`
}

func photoRef(i *fieldidentity.Identity, shown bool) *FieldWorkerPhoto {
	if !shown || i.PhotoVersion == "" || i.PhotoUpdatedAt == nil {
		return nil
	}
	return &FieldWorkerPhoto{Version: i.PhotoVersion, UpdatedAt: jsontime.New(*i.PhotoUpdatedAt)}
}

// photoError maps an imageproc refusal to the wire.
func photoError(err error, max int64) error {
	switch {
	case errors.Is(err, imageproc.ErrEmpty):
		return usecase.Validation(CodePhotoEmpty, "the body is empty; send the image's bytes")
	case errors.Is(err, imageproc.ErrTooLarge):
		return httpcompat.NewStatusError(http.StatusRequestEntityTooLarge, CodePhotoTooLarge,
			"a photo may be at most "+strconv.FormatInt(max, 10)+" bytes; send a smaller image")
	case errors.Is(err, imageproc.ErrUnsupportedType):
		return httpcompat.NewStatusError(http.StatusUnsupportedMediaType, CodePhotoUnsupported,
			"send a JPEG, PNG or WebP image")
	case errors.Is(err, imageproc.ErrTypeMismatch):
		return usecase.Validation(CodePhotoTypeMismatch, "the Content-Type does not match the image's own type")
	case errors.Is(err, imageproc.ErrTooManyPixels):
		return usecase.Validation(CodePhotoDimensions, "the image has too many pixels; send a smaller one")
	case errors.Is(err, imageproc.ErrCorrupt):
		return usecase.Validation(CodePhotoUnreadable, "the image cannot be read; send it again or use another file")
	case errors.Is(err, imageproc.ErrBusy):
		return httpcompat.NewStatusError(http.StatusServiceUnavailable, CodePhotoBusy,
			"too many photos are being processed; try again in a moment")
	}
	return usecase.Internal("PHOTO_PROCESS", "processing the photo failed", err)
}

func photosDisabled() error {
	return usecase.NotFound(CodePhotosDisabled, "field-worker photos are switched off on this platform")
}

// registerPhotos mounts the three photo routes.
func registerPhotos(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "setFieldWorkerPhoto",
		Method:      http.MethodPut,
		Path:        "/api/field-workers/{id}/photo",
		Summary:     "Set or replace a field worker's photo",
		Description: "The body is the image itself (JPEG, PNG or WebP; at most FC_FIELD_PHOTO_MAX_BYTES, 10 MiB by default). " +
			"The platform checks the bytes are an image of that type, applies the orientation, scales it down to a full (512 px) " +
			"and a thumbnail (128 px) JPEG, and keeps only those: every other byte of the upload, metadata included, is dropped. " +
			"Answers the worker. 409 PHOTOS_DISABLED_FOR_CLIENT when the client has photos off; 404 PHOTOS_DISABLED when the platform has.",
		Tags:          []string{workersTag},
		DefaultStatus: http.StatusOK,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				photoUploadContentType: {Schema: &huma.Schema{Type: "string", Format: "binary"}},
				"image/jpeg":           {Schema: &huma.Schema{Type: "string", Format: "binary"}},
				"image/png":            {Schema: &huma.Schema{Type: "string", Format: "binary"}},
				"image/webp":           {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			},
		},
	}, s.putPhoto)

	huma.Register(api, huma.Operation{
		OperationID: "getFieldWorkerPhoto",
		Method:      http.MethodGet,
		Path:        "/api/field-workers/{id}/photo",
		Summary:     "A field worker's photo (the image)",
		Description: "image/jpeg; size=thumb (128 px, the default) or full (512 px). ETag is \"<version>-<size>\", Cache-Control " +
			"is private, no-cache, so a removal or a lost permission takes effect at once; send If-None-Match for a 304.",
		Tags:          []string{workersTag},
		DefaultStatus: http.StatusOK,
		Responses: map[string]*huma.Response{
			"200": {Description: "The image", Content: map[string]*huma.MediaType{
				"image/jpeg": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			}},
			"304": {Description: "Not modified (If-None-Match matched)"},
		},
	}, s.getPhoto)

	huma.Register(api, huma.Operation{
		OperationID:   "removeFieldWorkerPhoto",
		Method:        http.MethodDelete,
		Path:          "/api/field-workers/{id}/photo",
		Summary:       "Remove a field worker's photo",
		Description:   "Answers the worker; removing a photo that is not there is not an error.",
		Tags:          []string{workersTag},
		DefaultStatus: http.StatusOK,
	}, s.removePhoto)
}

type putPhotoInput struct {
	ID   string `path:"id"`
	hctx huma.Context
}

// Resolve keeps the request for the handler, which reads the body itself so
// that huma does not buffer it and the size is checked first.
func (in *putPhotoInput) Resolve(ctx huma.Context) []error {
	in.hctx = ctx
	return nil
}

func (s *State) photoLimits() imageproc.Limits {
	if s.Photos == nil {
		return imageproc.DefaultLimits()
	}
	return s.Photos.Limits()
}

func (s *State) putPhoto(ctx context.Context, in *putPhotoInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if s.PhotosDisabled || s.Photos == nil {
		return nil, photosDisabled()
	}
	// The client's switch is read before the body is, so a refused upload costs
	// nothing; the use case checks it again in its transaction.
	pol, err := s.Ops.R.Policies.Effective(ctx, ident.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	if !pol.PhotosEnabled {
		return nil, usecase.Conflict(fieldidentity.CodePhotosDisabledForClient, "this client has switched field-worker photos off")
	}
	lim := s.photoLimits()
	// The declared length is judged before a byte of the body is read.
	if cl := in.hctx.Header("Content-Length"); cl != "" {
		if n, perr := strconv.ParseInt(cl, 10, 64); perr == nil && n > lim.MaxBytes {
			return nil, photoError(imageproc.ErrTooLarge, lim.MaxBytes)
		}
	}
	_ = in.hctx.SetReadDeadline(time.Now().Add(photoReadTimeout))
	data, rerr := io.ReadAll(io.LimitReader(in.hctx.BodyReader(), lim.MaxBytes+1))
	_ = in.hctx.SetReadDeadline(time.Time{})
	if rerr != nil {
		return nil, usecase.Validation("UPLOAD_INCOMPLETE", "the upload ended before the whole image was received; send it again")
	}
	pctx, cancel := context.WithTimeout(ctx, photoSlotWait)
	defer cancel()
	photo, perr := s.Photos.Process(pctx, data, in.hctx.Header("Content-Type"))
	if perr != nil {
		return nil, photoError(perr, lim.MaxBytes)
	}
	ac := auth.FromContext(ctx)
	cmd := fieldidentity.NewSetPhotoCommand(refOf(ident), fieldidentity.PhotoActorOf(ac), ac.PrincipalID, photo)
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.SetWorkerPhoto(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getWorker(ctx, &apicommon.IDInput{ID: ident.ID})
}

func (s *State) removePhoto(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if s.PhotosDisabled {
		return nil, photosDisabled()
	}
	ac := auth.FromContext(ctx)
	cmd := fieldidentity.RemovePhotoCommand{WorkerRef: refOf(ident), Actor: fieldidentity.PhotoActorOf(ac), ActorID: ac.PrincipalID}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RemoveWorkerPhoto(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getWorker(ctx, &apicommon.IDInput{ID: ident.ID})
}

type getPhotoInput struct {
	ID          string `path:"id"`
	Size        string `query:"size" enum:"thumb,full" default:"thumb" doc:"thumb (128 px) or full (512 px)"`
	IfNoneMatch string `header:"If-None-Match" doc:"An ETag from an earlier answer; 304 when the photo is unchanged"`
}

type photoOutput struct {
	Status       int
	ContentType  string `header:"Content-Type"`
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	NoSniff      string `header:"X-Content-Type-Options"`
	Disposition  string `header:"Content-Disposition"`
	Body         []byte
}

// etagMatches reports whether an If-None-Match header names etag (a list, weak
// validators, or "*").
func etagMatches(header, etag string) bool {
	for v := range strings.SplitSeq(header, ",") {
		v = strings.TrimPrefix(strings.TrimSpace(v), "W/")
		if v == "*" || v == etag {
			return true
		}
	}
	return false
}

func photoETag(version string, size fieldidentity.PhotoSize) string {
	return `"` + version + "-" + string(size) + `"`
}

func (s *State) getPhoto(ctx context.Context, in *getPhotoInput) (*photoOutput, error) {
	ident, err := s.loadWorker(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	if s.PhotosDisabled {
		return nil, photosDisabled()
	}
	size := fieldidentity.PhotoSize(in.Size)
	if in.Size == "" {
		size = fieldidentity.PhotoSize(photoDefaultSize)
	}
	if !size.Valid() {
		return nil, usecase.Validation("PHOTO_SIZE_INVALID", "size is thumb or full")
	}
	notFound := usecase.NotFound(CodePhotoNotFound, "this worker has no photo")
	if ident.PhotoVersion == "" {
		return nil, notFound
	}
	headers := photoOutput{
		CacheControl: photoCacheControl, NoSniff: "nosniff", Disposition: `inline; filename="photo.jpg"`,
	}
	// A matching validator is answered from the worker row, without the bytes.
	if etag := photoETag(ident.PhotoVersion, size); etagMatches(in.IfNoneMatch, etag) {
		headers.Status, headers.ETag = http.StatusNotModified, etag
		return &headers, nil
	}
	p, ok, err := s.Ops.R.Photos.Rendition(ctx, ident.ID, size)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_worker_photo failed", err)
	}
	if !ok {
		return nil, notFound
	}
	headers.Status, headers.ContentType, headers.ETag, headers.Body = http.StatusOK, p.ContentType, photoETag(p.Version, size), p.Bytes
	return &headers, nil
}
