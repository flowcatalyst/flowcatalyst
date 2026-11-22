package fieldidentity

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
)

// The error codes of a photo upload, one per [imageproc] refusal, and the two
// switches' answers. The admin API, the console and the device protocol all
// answer these (docs/field-worker-photo-spec.md sections 3 and 5).
const (
	// CodePhotosDisabled: FC_FIELD_PHOTOS=off (404).
	CodePhotosDisabled = "PHOTOS_DISABLED"
	// CodePhotoNotFound: the worker has no photo (404).
	CodePhotoNotFound = "PHOTO_NOT_FOUND"

	CodePhotoEmpty        = "PHOTO_EMPTY"
	CodePhotoTooLarge     = "PHOTO_TOO_LARGE"
	CodePhotoUnsupported  = "PHOTO_UNSUPPORTED_TYPE"
	CodePhotoTypeMismatch = "PHOTO_TYPE_MISMATCH"
	CodePhotoDimensions   = "PHOTO_DIMENSIONS_TOO_LARGE"
	CodePhotoUnreadable   = "PHOTO_UNREADABLE"
	CodePhotoBusy         = "PHOTO_BUSY"
)

// PhotoRefusal is how an upload the image package refused is answered: the
// code, the HTTP status the API and the device protocol give it, and a message
// that says how to fix it.
type PhotoRefusal struct {
	Code    string
	Status  int
	Message string
}

// PhotoRefusalOf maps an [imageproc] refusal; ok is false for any other error
// (a failure of the platform, not of the upload). maxBytes is the byte limit in
// force, named in the "too large" message.
func PhotoRefusalOf(err error, maxBytes int64) (PhotoRefusal, bool) {
	switch {
	case errors.Is(err, imageproc.ErrEmpty):
		return PhotoRefusal{CodePhotoEmpty, http.StatusBadRequest, "The image is empty. Choose an image file and send it again."}, true
	case errors.Is(err, imageproc.ErrTooLarge):
		return PhotoRefusal{
			CodePhotoTooLarge, http.StatusRequestEntityTooLarge,
			"The photo is larger than " + strconv.FormatInt(maxBytes, 10) + " bytes. Choose a smaller image.",
		}, true
	case errors.Is(err, imageproc.ErrUnsupportedType):
		return PhotoRefusal{CodePhotoUnsupported, http.StatusUnsupportedMediaType, "Choose a JPEG, PNG or WebP image."}, true
	case errors.Is(err, imageproc.ErrTypeMismatch):
		return PhotoRefusal{
			CodePhotoTypeMismatch, http.StatusBadRequest,
			"The file is not the type its Content-Type says. Choose the image file again.",
		}, true
	case errors.Is(err, imageproc.ErrTooManyPixels):
		return PhotoRefusal{CodePhotoDimensions, http.StatusBadRequest, "The image has too many pixels. Choose a smaller one."}, true
	case errors.Is(err, imageproc.ErrCorrupt):
		return PhotoRefusal{CodePhotoUnreadable, http.StatusBadRequest, "The image cannot be read. Choose another file."}, true
	case errors.Is(err, imageproc.ErrBusy):
		return PhotoRefusal{CodePhotoBusy, http.StatusServiceUnavailable, "Too many photos are being processed. Try again in a moment."}, true
	}
	return PhotoRefusal{}, false
}
