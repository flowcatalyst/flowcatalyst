package imageproc

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// Distinct quadrant colours: top-left, top-right, bottom-left, bottom-right.
var (
	red    = color.RGBA{220, 20, 20, 255}
	green  = color.RGBA{20, 200, 20, 255}
	blue   = color.RGBA{20, 20, 220, 255}
	yellow = color.RGBA{230, 230, 20, 255}
)

// quadrants draws four flat quadrants of w by h.
func quadrants(w, h int, tl, tr, bl, br color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill := func(r image.Rectangle, c color.RGBA) { draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src) }
	fill(image.Rect(0, 0, w/2, h/2), tl)
	fill(image.Rect(w/2, 0, w, h/2), tr)
	fill(image.Rect(0, h/2, w/2, h), bl)
	fill(image.Rect(w/2, h/2, w, h), br)
	return img
}

func mustJPEG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func mustPNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// segment is a JPEG marker segment: FF marker, length, payload.
func segment(marker byte, payload []byte) []byte {
	out := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(out[2:], uint16(len(payload)+2))
	return append(out, payload...)
}

// exifWithOrientation builds an Exif APP1 payload (big endian) whose IFD0 holds
// the orientation and, when extra is set, an ASCII string tag that stands in
// for GPS and device data.
func exifWithOrientation(o int, extra string) []byte {
	var tiff bytes.Buffer
	tiff.WriteString("MM")
	_ = binary.Write(&tiff, binary.BigEndian, uint16(42))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(8))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(2)) // two entries
	// orientation: tag 0x0112, SHORT, count 1, value in the first two bytes
	_ = binary.Write(&tiff, binary.BigEndian, uint16(0x0112))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(3))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(1))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(o))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(0))
	// a string tag (0x010F make) pointing at data after the IFD
	_ = binary.Write(&tiff, binary.BigEndian, uint16(0x010F))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(2))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(len(extra)+1))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(8+2+24+4))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(0)) // next IFD
	tiff.WriteString(extra)
	tiff.WriteByte(0)
	return append([]byte("Exif\x00\x00"), tiff.Bytes()...)
}

// withSegments inserts segments straight after the SOI of a JPEG.
func withSegments(jpg []byte, segs ...[]byte) []byte {
	out := append([]byte{}, jpg[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}

// pngChunk builds one PNG chunk with a correct CRC.
func pngChunk(typ string, data []byte) []byte {
	out := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(out, uint32(len(data)))
	copy(out[4:], typ)
	out = append(out, data...)
	crc := crc32.ChecksumIEEE(out[4:])
	return binary.BigEndian.AppendUint32(out, crc)
}

// withPNGChunk inserts a chunk after IHDR.
func withPNGChunk(p []byte, typ string, data []byte) []byte {
	const afterIHDR = 8 + 12 + 13
	out := append([]byte{}, p[:afterIHDR]...)
	out = append(out, pngChunk(typ, data)...)
	return append(out, p[afterIHDR:]...)
}

// bombPNG is a real PNG whose IHDR claims w by h (CRC fixed up); its pixel
// data is still that of a tiny image.
func bombPNG(t testing.TB, w, h uint32) []byte {
	p := mustPNG(t, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	ihdr := p[16:29]
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	binary.BigEndian.PutUint32(p[29:], crc32.ChecksumIEEE(p[12:29]))
	return p
}

// bombJPEG is a real JPEG whose SOF claims w by h.
func bombJPEG(t testing.TB, w, h uint16) []byte {
	j := mustJPEG(t, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	i := bytes.Index(j, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("no SOF0")
	}
	binary.BigEndian.PutUint16(j[i+5:], h)
	binary.BigEndian.PutUint16(j[i+7:], w)
	return j
}

// Tiny valid WebP files, as base64 (an encoder for WebP does not exist in Go,
// so these are the one unavoidable fixtures): a 1x1 lossy and a 1x1 lossless.
const (
	webpLossyB64    = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"
	webpLosslessB64 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="
)

func animatedGIF(t testing.TB) []byte {
	pal := color.Palette{color.Black, color.White}
	f := func() *image.Paletted { return image.NewPaletted(image.Rect(0, 0, 8, 8), pal) }
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{f(), f()}, Delay: []int{5, 5}}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func stillGIF(t testing.TB) []byte {
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// jpegMarkers lists the markers of a JPEG up to its scan.
func jpegMarkers(t testing.TB, j []byte) []byte {
	t.Helper()
	var ms []byte
	i := 2
	for i+4 <= len(j) {
		m := j[i+1]
		ms = append(ms, m)
		if m == 0xDA {
			return ms
		}
		i += 2 + int(binary.BigEndian.Uint16(j[i+2:]))
	}
	t.Fatal("no scan")
	return nil
}
