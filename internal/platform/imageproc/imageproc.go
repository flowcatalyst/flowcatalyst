// Package imageproc turns an uploaded photograph of a person into the two
// renditions the platform keeps: a full one (fit within 512 px) and a
// thumbnail (fit within 128 px), both baseline JPEG.
//
// It is pure: bytes in, bytes out, no HTTP and no database. What it promises:
//
//   - Only JPEG, PNG and WebP are accepted, decided from the bytes (magic
//     numbers), never from a file name. A declared Content-Type may only agree
//     with the bytes ([CheckDeclared]).
//   - The byte, pixel and side limits are checked before any pixel is decoded:
//     the size from len(data), the dimensions from the image header. A
//     decompression bomb is refused at the header.
//   - The output is always this package's own encoder's output, made from
//     pixels, so every piece of metadata in the upload (EXIF with its GPS
//     position, XMP, IPTC, comments, ICC profiles) and every appended or
//     embedded payload (a polyglot) is gone. The one thing read from the
//     metadata, the JPEG EXIF orientation, is applied to the pixels first.
//   - Transparency is flattened onto white; images are scaled down to fit
//     (never up) and never cropped.
//
// Every refusal is one of the Err values below, so the API can map each to its
// own code. Decoding is memory-heavy (a 25 MP image is 100 MB as RGBA), so a
// [Processor] bounds the number of decodes in flight.
package imageproc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"mime"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Type is a kind of image the package accepts.
type Type string

// The accepted image types. These are the only members: anything else is
// [ErrUnsupportedType].
const (
	TypeJPEG Type = "jpeg"
	TypePNG  Type = "png"
	TypeWebP Type = "webp"
)

// MIME is the type's media type.
func (t Type) MIME() string {
	switch t {
	case TypeJPEG:
		return "image/jpeg"
	case TypePNG:
		return "image/png"
	case TypeWebP:
		return "image/webp"
	}
	return ""
}

// Sizes of the renditions: the longest side, in pixels.
const (
	FullSide  = 512
	ThumbSide = 128
	// JPEGQuality is the quality both renditions are encoded at.
	JPEGQuality = 82
)

// Refusals. Each is a distinct value to test with errors.Is.
var (
	// ErrEmpty: no bytes.
	ErrEmpty = errors.New("imageproc: the body is empty")
	// ErrTooLarge: more bytes than Limits.MaxBytes.
	ErrTooLarge = errors.New("imageproc: the file is too large")
	// ErrUnsupportedType: the bytes (or the declared type) are not JPEG, PNG
	// or WebP. SVG, GIF, HEIC, BMP, TIFF and text all land here.
	ErrUnsupportedType = errors.New("imageproc: only JPEG, PNG and WebP are accepted")
	// ErrTypeMismatch: the declared type names a supported type other than the
	// bytes' own.
	ErrTypeMismatch = errors.New("imageproc: the declared type does not match the file")
	// ErrTooManyPixels: the header claims more pixels, or a longer side, than
	// the limits allow (a decompression bomb or just a very large photograph).
	ErrTooManyPixels = errors.New("imageproc: the image has too many pixels")
	// ErrCorrupt: the header or the pixel data cannot be read (truncated,
	// malformed, or a format feature the decoder does not support such as an
	// animated WebP).
	ErrCorrupt = errors.New("imageproc: the image cannot be read")
	// ErrBusy: no decode slot became free before the context ended.
	ErrBusy = errors.New("imageproc: too many images are being processed")
)

// Limits bound an upload.
type Limits struct {
	// MaxBytes is the most bytes accepted.
	MaxBytes int64
	// MaxPixels is the most pixels (width times height) accepted.
	MaxPixels int64
	// MaxSide is the longest side accepted, in pixels.
	MaxSide int
}

// DefaultLimits are 10 MiB, 25 million pixels and 10 000 px a side.
func DefaultLimits() Limits {
	return Limits{MaxBytes: 10 << 20, MaxPixels: 25_000_000, MaxSide: 10_000}
}

// Rendition is one encoded size.
type Rendition struct {
	JPEG          []byte
	Width, Height int
}

// Photo is what Process makes.
type Photo struct {
	Full, Thumb Rendition
	// Source is the type of the upload.
	Source Type
	// Version is 16 hex characters of the SHA-256 of the full rendition: it
	// changes whenever the picture does.
	Version string
}

// Sniff decides the type from the bytes' magic numbers. It reads only the
// first bytes.
func Sniff(data []byte) (Type, error) {
	switch {
	case len(data) == 0:
		return "", ErrEmpty
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return TypeJPEG, nil
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return TypePNG, nil
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return TypeWebP, nil
	}
	return "", ErrUnsupportedType
}

// CheckDeclared compares a declared Content-Type with the sniffed type. An
// empty value or application/octet-stream declares nothing and is accepted.
// image/jpeg, image/png and image/webp must equal the sniffed type
// ([ErrTypeMismatch] otherwise); any other declared type is
// [ErrUnsupportedType]. Parameters (charset) and case are ignored.
func CheckDeclared(declared string, sniffed Type) error {
	declared = strings.TrimSpace(declared)
	if declared == "" {
		return nil
	}
	mt, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return ErrUnsupportedType
	}
	switch mt {
	case "application/octet-stream":
		return nil
	case TypeJPEG.MIME():
		return matches(TypeJPEG, sniffed)
	case TypePNG.MIME():
		return matches(TypePNG, sniffed)
	case TypeWebP.MIME():
		return matches(TypeWebP, sniffed)
	}
	return ErrUnsupportedType
}

func matches(want, got Type) error {
	if want != got {
		return ErrTypeMismatch
	}
	return nil
}

// decodeFunc turns bytes of a type into pixels; replaced in tests to prove
// that a refused upload was never decoded.
type decodeFunc func(t Type, data []byte) (image.Image, error)

func decodePixels(t Type, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch t {
	case TypeJPEG:
		return jpeg.Decode(r)
	case TypePNG:
		return png.Decode(r)
	case TypeWebP:
		return webp.Decode(r)
	}
	return nil, ErrUnsupportedType
}

// header reads the dimensions from the image header only.
func header(t Type, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	var cfg image.Config
	var err error
	switch t {
	case TypeJPEG:
		cfg, err = jpeg.DecodeConfig(r)
	case TypePNG:
		cfg, err = png.DecodeConfig(r)
	case TypeWebP:
		cfg, err = webp.DecodeConfig(r)
	default:
		return cfg, ErrUnsupportedType
	}
	if err != nil {
		return cfg, ErrCorrupt
	}
	return cfg, nil
}

// Process validates data and makes the two renditions. declared is the
// request's Content-Type (may be empty). It does not bound concurrency; use a
// [Processor] for that.
func Process(data []byte, declared string, lim Limits) (*Photo, error) {
	return process(data, declared, lim, decodePixels)
}

func process(data []byte, declared string, lim Limits, decode decodeFunc) (*Photo, error) {
	if len(data) == 0 {
		return nil, ErrEmpty
	}
	if int64(len(data)) > lim.MaxBytes {
		return nil, ErrTooLarge
	}
	t, err := Sniff(data)
	if err != nil {
		return nil, err
	}
	if err := CheckDeclared(declared, t); err != nil {
		return nil, err
	}
	cfg, err := header(t, data)
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrCorrupt
	}
	if cfg.Width > lim.MaxSide || cfg.Height > lim.MaxSide || int64(cfg.Width)*int64(cfg.Height) > lim.MaxPixels {
		return nil, ErrTooManyPixels
	}
	// Orientation is read before the decode so a malformed tag never matters
	// to the pixels; only JPEG carries one that we honour.
	orientation := 1
	if t == TypeJPEG {
		orientation = jpegOrientation(data)
	}
	src, err := decode(t, data)
	if err != nil {
		return nil, ErrCorrupt
	}
	// Scale first (cheap to orient afterwards), flattening onto white.
	full := orient(fit(src, FullSide), orientation)
	thumb := fit(full, ThumbSide)
	fullJPEG, err := encode(full)
	if err != nil {
		return nil, err
	}
	thumbJPEG, err := encode(thumb)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(fullJPEG)
	return &Photo{
		Full:    Rendition{JPEG: fullJPEG, Width: full.Bounds().Dx(), Height: full.Bounds().Dy()},
		Thumb:   Rendition{JPEG: thumbJPEG, Width: thumb.Bounds().Dx(), Height: thumb.Bounds().Dy()},
		Source:  t,
		Version: hex.EncodeToString(sum[:8]),
	}, nil
}

// Processor runs Process with at most a fixed number of decodes in flight.
type Processor struct {
	lim    Limits
	sem    chan struct{}
	decode decodeFunc
}

// NewProcessor bounds decodes to concurrency (at least 1).
func NewProcessor(lim Limits, concurrency int) *Processor {
	return &Processor{lim: lim, sem: make(chan struct{}, max(concurrency, 1)), decode: decodePixels}
}

// Limits returns the limits in force.
func (p *Processor) Limits() Limits { return p.lim }

// Process waits for a free slot until ctx ends ([ErrBusy]), then runs
// [Process]. Cheap refusals (empty, too large, type) are answered without a
// slot.
func (p *Processor) Process(ctx context.Context, data []byte, declared string) (*Photo, error) {
	if len(data) == 0 {
		return nil, ErrEmpty
	}
	if int64(len(data)) > p.lim.MaxBytes {
		return nil, ErrTooLarge
	}
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return nil, ErrBusy
	}
	return process(data, declared, p.lim, p.decode)
}

// fit scales src down so its longest side is at most side (never up, no
// crop), onto an opaque white background. The result is always a fresh
// *image.RGBA with origin (0,0).
func fit(src image.Image, side int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if long := max(w, h); long > side {
		nw = max(1, w*side/long)
		nh = max(1, h*side/long)
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if nw == w && nh == h {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	}
	return dst
}

func encode(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return nil, ErrCorrupt
	}
	return buf.Bytes(), nil
}

// orient applies an EXIF orientation (1 to 8) so the picture is upright.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	nw, nh := w, h
	if o >= 5 {
		nw, nh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored horizontally
				dx, dy = w-1-x, y
			case 3: // rotated 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored vertically
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // rotate 90 clockwise to be upright
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 90 counter-clockwise to be upright
				dx, dy = y, w-1-x
			}
			si := src.PixOffset(x, y)
			di := dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
