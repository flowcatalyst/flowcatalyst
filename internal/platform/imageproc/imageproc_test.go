package imageproc

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeJPEG(t testing.TB, b []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(b))
	require.NoError(t, err)
	return img
}

// near reports whether the pixel is close to c (JPEG is lossy).
func near(got color.Color, want color.RGBA) bool {
	r, g, b, _ := got.RGBA()
	d := func(a uint32, w uint8) int { return int(a>>8) - int(w) }
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return abs(d(r, want.R)) < 40 && abs(d(g, want.G)) < 40 && abs(d(b, want.B)) < 40
}

// corners samples the middle of each quadrant of img: TL, TR, BL, BR.
func corners(img image.Image) [4]color.Color {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	return [4]color.Color{
		img.At(b.Min.X+w/4, b.Min.Y+h/4), img.At(b.Min.X+3*w/4, b.Min.Y+h/4),
		img.At(b.Min.X+w/4, b.Min.Y+3*h/4), img.At(b.Min.X+3*w/4, b.Min.Y+3*h/4),
	}
}

func TestFormats(t *testing.T) {
	lossy, _ := base64.StdEncoding.DecodeString(webpLossyB64)
	lossless, _ := base64.StdEncoding.DecodeString(webpLosslessB64)
	src := quadrants(80, 40, red, green, blue, yellow)
	for name, tc := range map[string]struct {
		data []byte
		typ  Type
	}{
		"jpeg":          {mustJPEG(t, src), TypeJPEG},
		"png":           {mustPNG(t, src), TypePNG},
		"webp lossy":    {lossy, TypeWebP},
		"webp lossless": {lossless, TypeWebP},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := Process(tc.data, tc.typ.MIME(), DefaultLimits())
			require.NoError(t, err)
			assert.Equal(t, tc.typ, p.Source)
			assert.Len(t, p.Version, 16)
			require.NotEmpty(t, p.Full.JPEG)
			full := decodeJPEG(t, p.Full.JPEG)
			assert.Equal(t, p.Full.Width, full.Bounds().Dx())
			assert.Equal(t, p.Full.Height, full.Bounds().Dy())
			thumb := decodeJPEG(t, p.Thumb.JPEG)
			assert.Equal(t, p.Thumb.Width, thumb.Bounds().Dx())
			// Sniffed type works with no declared type too.
			_, err = Process(tc.data, "", DefaultLimits())
			assert.NoError(t, err)
		})
	}
}

func TestQuadrantsSurvive(t *testing.T) {
	p, err := Process(mustPNG(t, quadrants(80, 40, red, green, blue, yellow)), "", DefaultLimits())
	require.NoError(t, err)
	c := corners(decodeJPEG(t, p.Full.JPEG))
	for i, want := range []color.RGBA{red, green, blue, yellow} {
		assert.True(t, near(c[i], want), "quadrant %d", i)
	}
}

func TestScalingFitsAndNeverEnlarges(t *testing.T) {
	big, err := Process(mustJPEG(t, quadrants(2000, 1000, red, green, blue, yellow)), "", DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, [2]int{512, 256}, [2]int{big.Full.Width, big.Full.Height})
	assert.Equal(t, [2]int{128, 64}, [2]int{big.Thumb.Width, big.Thumb.Height})

	tall, err := Process(mustJPEG(t, quadrants(300, 900, red, green, blue, yellow)), "", DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, [2]int{170, 512}, [2]int{tall.Full.Width, tall.Full.Height})
	assert.Equal(t, [2]int{42, 128}, [2]int{tall.Thumb.Width, tall.Thumb.Height})

	small, err := Process(mustJPEG(t, quadrants(100, 60, red, green, blue, yellow)), "", DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, [2]int{100, 60}, [2]int{small.Full.Width, small.Full.Height}, "never enlarged")
	assert.Equal(t, [2]int{100, 60}, [2]int{small.Thumb.Width, small.Thumb.Height}, "under 128 stays")

	one, err := Process(mustPNG(t, image.NewRGBA(image.Rect(0, 0, 1, 1))), "", DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, [2]int{1, 1}, [2]int{one.Full.Width, one.Full.Height})
}

func TestVersionFollowsThePicture(t *testing.T) {
	a := mustPNG(t, quadrants(40, 40, red, green, blue, yellow))
	b := mustPNG(t, quadrants(40, 40, yellow, green, blue, red))
	pa, _ := Process(a, "", DefaultLimits())
	pa2, _ := Process(a, "", DefaultLimits())
	pb, _ := Process(b, "", DefaultLimits())
	assert.Equal(t, pa.Version, pa2.Version)
	assert.NotEqual(t, pa.Version, pb.Version)
}

// The expected placement of the upright picture's quadrants (TL, TR, BL, BR)
// in the stored file, for each EXIF orientation (the EXIF specification's
// table, written out by hand: not derived from the code under test).
func TestOrientationsComeOutUpright(t *testing.T) {
	// table[o] says which upright quadrant (0 TL, 1 TR, 2 BL, 3 BR) the stored
	// file holds at its TL, TR, BL, BR.
	table := map[int][4]int{
		1: {0, 1, 2, 3},
		2: {1, 0, 3, 2},
		3: {3, 2, 1, 0},
		4: {2, 3, 0, 1},
		5: {0, 2, 1, 3},
		6: {1, 3, 0, 2},
		7: {3, 1, 2, 0},
		8: {2, 0, 3, 1},
	}
	upright := [4]color.RGBA{red, green, blue, yellow}
	for o := 1; o <= 8; o++ {
		t.Run(string(rune('0'+o)), func(t *testing.T) {
			w, h := 80, 40
			if o >= 5 {
				w, h = 40, 80
			}
			h4 := table[o]
			stored := quadrants(w, h, upright[h4[0]], upright[h4[1]], upright[h4[2]], upright[h4[3]])
			j := withSegments(mustJPEG(t, stored), segment(0xE1, exifWithOrientation(o, "x")))
			require.Equal(t, o, jpegOrientation(j))
			p, err := Process(j, "image/jpeg", DefaultLimits())
			require.NoError(t, err)
			assert.Equal(t, [2]int{80, 40}, [2]int{p.Full.Width, p.Full.Height}, "landscape upright")
			c := corners(decodeJPEG(t, p.Full.JPEG))
			for i := range upright {
				assert.True(t, near(c[i], upright[i]), "quadrant %d", i)
			}
		})
	}
}
