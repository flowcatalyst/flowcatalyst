package imageproc

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// secrets are strings planted in every metadata carrier; none may survive.
var secrets = []string{"SECRETGPS", "SECRETXMP", "SECRETCOMMENT", "SECRETICC", "SECRETIPTC", "SECRETTEXT", "SECRETEXIFCHUNK"}

func assertClean(t *testing.T, p *Photo) {
	t.Helper()
	for name, r := range map[string][]byte{"full": p.Full.JPEG, "thumb": p.Thumb.JPEG} {
		for _, s := range secrets {
			assert.False(t, bytes.Contains(r, []byte(s)), "%s holds %s", name, s)
		}
		for _, s := range []string{"Exif", "http://ns.adobe.com", "ICC_PROFILE", "Photoshop", "JFIF", "GPS"} {
			assert.False(t, bytes.Contains(r, []byte(s)), "%s holds %s", name, s)
		}
		// Only the encoder's own markers: DQT, SOF0, DHT, SOS.
		for _, m := range jpegMarkers(t, r) {
			assert.Contains(t, []byte{0xDB, 0xC0, 0xC4, 0xDA}, m, "%s has marker %02X", name, m)
		}
	}
}

func TestJPEGMetadataIsGone(t *testing.T) {
	j := mustJPEG(t, quadrants(64, 48, red, green, blue, yellow))
	j = withSegments(j,
		segment(0xE1, exifWithOrientation(1, "SECRETGPS 51.5N 0.1W")),
		segment(0xE1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), "<x:xmpmeta>SECRETXMP</x:xmpmeta>"...)),
		segment(0xE2, append([]byte("ICC_PROFILE\x00\x01\x01"), "SECRETICC"...)),
		segment(0xED, append([]byte("Photoshop 3.0\x00"), "8BIM SECRETIPTC"...)),
		segment(0xFE, []byte("SECRETCOMMENT")),
	)
	for _, s := range secrets[:5] {
		require.Contains(t, string(j), s, "the fixture carries %s", s)
	}
	p, err := Process(j, "image/jpeg", DefaultLimits())
	require.NoError(t, err)
	assertClean(t, p)
}

func TestPNGMetadataIsGone(t *testing.T) {
	p := mustPNG(t, quadrants(64, 48, red, green, blue, yellow))
	p = withPNGChunk(p, "tEXt", []byte("Comment\x00SECRETTEXT"))
	p = withPNGChunk(p, "eXIf", []byte("MM\x00*\x00\x00\x00\x08SECRETEXIFCHUNK"))
	p = withPNGChunk(p, "iCCP", []byte("icc\x00\x00SECRETICC"))
	for _, s := range []string{"SECRETTEXT", "SECRETEXIFCHUNK", "SECRETICC"} {
		require.Contains(t, string(p), s)
	}
	out, err := Process(p, "image/png", DefaultLimits())
	require.NoError(t, err)
	assertClean(t, out)
}

// refusal runs data through process with a decode that counts its calls.
func refusal(t *testing.T, data []byte, declared string, lim Limits) (int32, error) {
	t.Helper()
	var calls atomic.Int32
	_, err := process(data, declared, lim, func(ty Type, d []byte) (image.Image, error) {
		calls.Add(1)
		return decodePixels(ty, d)
	})
	return calls.Load(), err
}

func TestRefusals(t *testing.T) {
	ok := mustJPEG(t, quadrants(40, 40, red, green, blue, yellow))
	okPNG := mustPNG(t, quadrants(40, 40, red, green, blue, yellow))
	lim := DefaultLimits()
	heic := append([]byte{0, 0, 0, 0x18}, "ftypheic\x00\x00\x00\x00mif1heic"...)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>alert(1)</script></svg>`)
	small := Limits{MaxBytes: 100, MaxPixels: lim.MaxPixels, MaxSide: lim.MaxSide}
	lossless, _ := base64.StdEncoding.DecodeString(webpLosslessB64)

	tests := []struct {
		name     string
		data     []byte
		declared string
		lim      Limits
		want     error
		decoded  bool // true only where the refusal comes from the decode itself
	}{
		{"empty", nil, "image/jpeg", lim, ErrEmpty, false},
		{"text declared as an image", []byte("hello, I am not an image"), "image/jpeg", lim, ErrUnsupportedType, false},
		{"html declared as png", []byte("<html><body>x</body></html>"), "image/png", lim, ErrUnsupportedType, false},
		{"oversized bytes", ok, "image/jpeg", small, ErrTooLarge, false},
		{"svg", svg, "image/svg+xml", lim, ErrUnsupportedType, false},
		{"svg declared as png", svg, "image/png", lim, ErrUnsupportedType, false},
		{"gif still", stillGIF(t), "image/gif", lim, ErrUnsupportedType, false},
		{"gif animated", animatedGIF(t), "", lim, ErrUnsupportedType, false},
		{"gif declared as jpeg", animatedGIF(t), "image/jpeg", lim, ErrUnsupportedType, false},
		{"heic", heic, "image/heic", lim, ErrUnsupportedType, false},
		{"heic declared as jpeg", heic, "image/jpeg", lim, ErrUnsupportedType, false},
		{"png declared as jpeg", okPNG, "image/jpeg", lim, ErrTypeMismatch, false},
		{"jpeg declared as webp", ok, "image/webp", lim, ErrTypeMismatch, false},
		{"jpeg declared as svg", ok, "image/svg+xml", lim, ErrUnsupportedType, false},
		{"declared type garbage", ok, "image/jpeg;;;=", lim, ErrUnsupportedType, false},
		{"declared type of text", ok, "text/plain", lim, ErrUnsupportedType, false},
		{"png bomb 60000x60000", bombPNG(t, 60000, 60000), "image/png", lim, ErrTooManyPixels, false},
		{"jpeg bomb 60000x60000", bombJPEG(t, 60000, 60000), "image/jpeg", lim, ErrTooManyPixels, false},
		{"png long side", bombPNG(t, 20000, 1), "", lim, ErrTooManyPixels, false},
		{"jpeg long side", bombJPEG(t, 1, 20000), "", lim, ErrTooManyPixels, false},
		{"png just over the pixel budget", bombPNG(t, 5001, 5000), "", lim, ErrTooManyPixels, false},
		{"jpeg magic then html", append([]byte{0xFF, 0xD8, 0xFF}, "<html></html>"...), "image/jpeg", lim, ErrCorrupt, false},
		{"truncated header", ok[:5], "image/jpeg", lim, ErrCorrupt, false},
		{"png truncated header", okPNG[:20], "", lim, ErrCorrupt, false},
		{"jpeg truncated pixels", ok[:len(ok)*2/3], "image/jpeg", lim, ErrCorrupt, true},
		{"png truncated pixels", okPNG[:len(okPNG)-30], "image/png", lim, ErrCorrupt, true},
		{"webp truncated", lossless[:len(lossless)-6], "image/webp", lim, ErrCorrupt, true},
		{"riff but not webp", []byte("RIFF\x04\x00\x00\x00WAVEfmt "), "", lim, ErrUnsupportedType, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls, err := refusal(t, tc.data, tc.declared, tc.lim)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.want), "got %v, want %v", err, tc.want)
			if !tc.decoded {
				assert.Zero(t, calls, "the refusal came before any pixel decode")
			}
		})
	}
}

func TestBombsAreRefusedAtTheHeader(t *testing.T) {
	// The proof that no decode happened is the counter in TestRefusals; this
	// one also shows a bomb costs nothing: the call returns at once and the
	// default limits are untouched.
	for _, b := range [][]byte{bombPNG(t, 60000, 60000), bombJPEG(t, 65535, 65535)} {
		start := time.Now()
		calls, err := refusal(t, b, "", DefaultLimits())
		assert.ErrorIs(t, err, ErrTooManyPixels)
		assert.Zero(t, calls)
		assert.Less(t, time.Since(start), 200*time.Millisecond)
	}
}

func TestPolyglotsLoseTheirTail(t *testing.T) {
	tail := []byte("PK\x03\x04SECRETTEXT<script>SECRETTEXT</script><?php SECRETTEXT ?>")
	j := append(mustJPEG(t, quadrants(40, 40, red, green, blue, yellow)), tail...)
	p := append(mustPNG(t, quadrants(40, 40, red, green, blue, yellow)), tail...)
	for name, data := range map[string][]byte{"jpeg + tail": j, "png + tail": p} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(data), "SECRETTEXT")
			out, err := Process(data, "", DefaultLimits())
			require.NoError(t, err)
			assertClean(t, out)
			for _, r := range [][]byte{out.Full.JPEG, out.Thumb.JPEG} {
				assert.False(t, bytes.Contains(r, []byte("PK\x03\x04")))
				assert.False(t, bytes.Contains(r, []byte("<script>")))
			}
		})
	}
	// A JPEG whose comment segment is a zip is just as gone.
	z := withSegments(mustJPEG(t, quadrants(40, 40, red, green, blue, yellow)), segment(0xFE, tail))
	out, err := Process(z, "", DefaultLimits())
	require.NoError(t, err)
	assertClean(t, out)
}

func TestColourModels(t *testing.T) {
	// 16-bit PNG: a real file written by the encoder from RGBA64.
	img64 := image.NewRGBA64(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img64.SetRGBA64(x, y, color64(0xFFFF, uint16(x*2000), 0, 0xFFFF))
		}
	}
	out, err := Process(mustPNG(t, img64), "image/png", DefaultLimits())
	require.NoError(t, err)
	assert.True(t, near(decodeJPEG(t, out.Full.JPEG).At(30, 16), color.RGBA{255, 230, 0, 255}) ||
		near(decodeJPEG(t, out.Full.JPEG).At(30, 16), color.RGBA{255, 233, 0, 255}))

	// Paletted with a transparent entry: the transparent half comes out white.
	pal := color.Palette{color.NRGBA{0, 0, 0, 0}, color.NRGBA{200, 0, 0, 255}}
	pi := image.NewPaletted(image.Rect(0, 0, 40, 40), pal)
	for y := range 40 {
		for x := 20; x < 40; x++ {
			pi.SetColorIndex(x, y, 1)
		}
	}
	out, err = Process(mustPNG(t, pi), "image/png", DefaultLimits())
	require.NoError(t, err)
	got := decodeJPEG(t, out.Full.JPEG)
	assert.True(t, near(got.At(8, 20), color.RGBA{255, 255, 255, 255}), "transparent becomes white, not black")
	assert.True(t, near(got.At(32, 20), color.RGBA{200, 0, 0, 255}))

	// NRGBA with partial alpha is blended over white.
	nr := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := range nr.Pix {
		nr.Pix[i] = 0
	}
	for y := range 8 {
		for x := range 8 {
			nr.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 128})
		}
	}
	out, err = Process(mustPNG(t, nr), "", DefaultLimits())
	require.NoError(t, err)
	r, _, _, _ := decodeJPEG(t, out.Full.JPEG).At(4, 4).RGBA()
	assert.InDelta(t, 127, int(r>>8), 20, "half-transparent black over white is mid grey")

	// CMYK: the standard library can decode a CMYK JPEG to image.CMYK but not
	// encode one, so the decode is injected; the header is a real JPEG's.
	cmyk := image.NewCMYK(image.Rect(0, 0, 40, 40))
	for i := 0; i < len(cmyk.Pix); i += 4 {
		cmyk.Pix[i], cmyk.Pix[i+1], cmyk.Pix[i+2], cmyk.Pix[i+3] = 0, 255, 255, 0 // red
	}
	p, err := process(mustJPEG(t, quadrants(40, 40, red, red, red, red)), "", DefaultLimits(),
		func(Type, []byte) (image.Image, error) { return cmyk, nil })
	require.NoError(t, err)
	assert.True(t, near(decodeJPEG(t, p.Full.JPEG).At(20, 20), color.RGBA{255, 0, 0, 255}))
}

func color64(r, g, b, a uint16) color.RGBA64 { return color.RGBA64{R: r, G: g, B: b, A: a} }

func TestSniffAndDeclared(t *testing.T) {
	_, err := Sniff(nil)
	assert.ErrorIs(t, err, ErrEmpty)
	for _, d := range []string{"", " ", "application/octet-stream", "Application/Octet-Stream; x=y", "image/jpeg", "IMAGE/JPEG; charset=binary"} {
		assert.NoError(t, CheckDeclared(d, TypeJPEG), d)
	}
	assert.ErrorIs(t, CheckDeclared("image/png", TypeJPEG), ErrTypeMismatch)
	assert.ErrorIs(t, CheckDeclared("image/webp", TypePNG), ErrTypeMismatch)
	assert.ErrorIs(t, CheckDeclared("image/gif", TypeJPEG), ErrUnsupportedType)
	assert.ErrorIs(t, CheckDeclared("image/jpg", TypeJPEG), ErrUnsupportedType, "only the registered name")
	for _, ty := range []Type{TypeJPEG, TypePNG, TypeWebP} {
		assert.NotEmpty(t, ty.MIME())
	}
	assert.Empty(t, Type("gif").MIME())
}

func TestProcessorBoundsConcurrency(t *testing.T) {
	data := mustJPEG(t, quadrants(40, 40, red, green, blue, yellow))
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	p := NewProcessor(DefaultLimits(), 1)
	p.decode = func(ty Type, d []byte) (image.Image, error) {
		entered <- struct{}{}
		<-release
		return decodePixels(ty, d)
	}
	done := make(chan error, 1)
	go func() { _, err := p.Process(context.Background(), data, ""); done <- err }()
	<-entered // the one slot is taken

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Process(ctx, data, "")
	assert.ErrorIs(t, err, ErrBusy, "a second decode waits for the slot, then gives up")

	// Cheap refusals do not need a slot.
	_, err = p.Process(ctx, nil, "")
	assert.ErrorIs(t, err, ErrEmpty)

	close(release)
	require.NoError(t, <-done)
	_, err = p.Process(context.Background(), data, "") // slot free again
	assert.NoError(t, err)
	assert.Equal(t, DefaultLimits(), p.Limits())
}

func FuzzProcess(f *testing.F) {
	lossy, _ := base64.StdEncoding.DecodeString(webpLossyB64)
	lossless, _ := base64.StdEncoding.DecodeString(webpLosslessB64)
	src := quadrants(24, 16, red, green, blue, yellow)
	f.Add(mustJPEG(f, src), "image/jpeg")
	f.Add(withSegments(mustJPEG(f, src), segment(0xE1, exifWithOrientation(6, "x"))), "")
	f.Add(mustPNG(f, src), "image/png")
	f.Add(lossy, "image/webp")
	f.Add(lossless, "")
	f.Add(bombPNG(f, 60000, 60000), "")
	f.Add(bombJPEG(f, 60000, 60000), "")
	f.Add([]byte("GIF89a"), "image/gif")
	lim := Limits{MaxBytes: 64 << 10, MaxPixels: 1 << 20, MaxSide: 2000}
	f.Fuzz(func(t *testing.T, data []byte, declared string) {
		p, err := Process(data, declared, lim)
		if err != nil {
			known := false
			for _, e := range []error{ErrEmpty, ErrTooLarge, ErrUnsupportedType, ErrTypeMismatch, ErrTooManyPixels, ErrCorrupt} {
				known = known || errors.Is(err, e)
			}
			if !known {
				t.Fatalf("unnamed error: %v", err)
			}
			return
		}
		for _, r := range []Rendition{p.Full, p.Thumb} {
			img, err := jpeg.Decode(bytes.NewReader(r.JPEG))
			if err != nil {
				t.Fatalf("output is not a JPEG: %v", err)
			}
			if img.Bounds().Dx() > FullSide || img.Bounds().Dy() > FullSide {
				t.Fatalf("output too large: %v", img.Bounds())
			}
			for _, m := range jpegMarkers(t, r.JPEG) {
				if m != 0xDB && m != 0xC0 && m != 0xC4 && m != 0xDA {
					t.Fatalf("unexpected marker %02X", m)
				}
			}
		}
	})
}
