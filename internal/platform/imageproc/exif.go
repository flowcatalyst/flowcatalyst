package imageproc

import "encoding/binary"

// jpegOrientation reads the EXIF orientation (1 to 8) from a JPEG's APP1
// segment, or 1 when there is none or it is malformed. It walks the marker
// segments up to the start of scan and bounds-checks every read; it never
// reads anything but the orientation tag.
func jpegOrientation(data []byte) int {
	i := 2 // after SOI
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // scan or end: no more headers
			return 1
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 { // no length
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(data[i+2:]))
		if n < 2 || i+2+n > len(data) {
			return 1
		}
		if marker == 0xE1 {
			if o := tiffOrientation(data[i+4 : i+2+n]); o != 0 {
				return o
			}
		}
		i += 2 + n
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from the first IFD of an "Exif\0\0" APP1
// payload; 0 when absent or invalid.
func tiffOrientation(p []byte) int {
	if len(p) < 14 || string(p[:6]) != "Exif\x00\x00" {
		return 0
	}
	t := p[6:]
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	if bo.Uint16(t[2:]) != 42 {
		return 0
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 0
	}
	count := int(bo.Uint16(t[off:]))
	for k := range count {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:]) != 0x0112 {
			continue
		}
		if bo.Uint16(t[e+2:]) != 3 || bo.Uint32(t[e+4:]) != 1 { // SHORT, one value
			return 0
		}
		if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
			return v
		}
		return 0
	}
	return 0
}
