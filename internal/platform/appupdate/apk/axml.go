package apk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Chunk types of Android's binary resource formats (ResourceTypes.h).
const (
	chunkStringPool   = 0x0001
	chunkTable        = 0x0002
	chunkXML          = 0x0003
	chunkXMLStartNS   = 0x0100
	chunkXMLEndNS     = 0x0101
	chunkXMLStart     = 0x0102
	chunkXMLEnd       = 0x0103
	chunkXMLResMap    = 0x0180
	chunkTablePackage = 0x0200
	chunkTableType    = 0x0201
)

// Res_value data types.
const (
	typeNull             = 0x00
	typeReference        = 0x01
	typeString           = 0x03
	typeDynamicReference = 0x07
	typeIntDec           = 0x10
	typeIntHex           = 0x11
	typeIntBoolean       = 0x12
)

// Framework attribute resource ids (android.R.attr).
const (
	idName             = 0x01010003
	idDebuggable       = 0x0101000f
	idMinSDKVersion    = 0x0101020c
	idVersionCode      = 0x0101021b
	idVersionName      = 0x0101021c
	idValue            = 0x01010024
	idTargetSDKVersion = 0x01010270
	idVersionCodeMajor = 0x01010576
)

const noIndex = 0xffffffff

// maxDepth bounds the element nesting the manifest reader follows.
const maxDepth = 256

var le = binary.LittleEndian

var errTruncated = errors.New("truncated")

// stringPool decodes a ResStringPool chunk lazily.
type stringPool struct {
	chunk   []byte
	offsets []byte
	start   uint32
	count   uint32
	utf8    bool
	cache   map[uint32]string
}

func parseStringPool(chunk []byte) (*stringPool, error) {
	if len(chunk) < 28 {
		return nil, errTruncated
	}
	hs := uint32(le.Uint16(chunk[2:]))
	count := le.Uint32(chunk[8:])
	flags := le.Uint32(chunk[16:])
	start := le.Uint32(chunk[20:])
	if hs < 28 || hs > uint32(len(chunk)) || uint64(count)*4 > uint64(len(chunk))-uint64(hs) {
		return nil, errors.New("string pool header out of bounds")
	}
	return &stringPool{
		chunk: chunk, offsets: chunk[hs : hs+count*4], start: start, count: count,
		utf8: flags&(1<<8) != 0, cache: map[uint32]string{},
	}, nil
}

func (p *stringPool) get(i uint32) (string, bool) {
	if p == nil || i >= p.count {
		return "", false
	}
	if s, ok := p.cache[i]; ok {
		return s, true
	}
	pos := uint64(p.start) + uint64(le.Uint32(p.offsets[i*4:]))
	var s string
	var ok bool
	if p.utf8 {
		s, ok = p.utf8At(pos)
	} else {
		s, ok = p.utf16At(pos)
	}
	if ok {
		p.cache[i] = s
	}
	return s, ok
}

func (p *stringPool) utf8At(pos uint64) (string, bool) {
	b := p.chunk
	// The UTF-16 length comes first (skipped), then the UTF-8 byte length;
	// each is one byte, or two when the first has its high bit set.
	var n uint64
	for range 2 {
		if pos >= uint64(len(b)) {
			return "", false
		}
		n = uint64(b[pos])
		pos++
		if n&0x80 != 0 {
			if pos >= uint64(len(b)) {
				return "", false
			}
			n = (n&0x7f)<<8 | uint64(b[pos])
			pos++
		}
	}
	if pos+n > uint64(len(b)) {
		return "", false
	}
	return string(b[pos : pos+n]), true
}

func (p *stringPool) utf16At(pos uint64) (string, bool) {
	b := p.chunk
	if pos+2 > uint64(len(b)) {
		return "", false
	}
	n := uint64(le.Uint16(b[pos:]))
	pos += 2
	if n&0x8000 != 0 {
		if pos+2 > uint64(len(b)) {
			return "", false
		}
		n = (n&0x7fff)<<16 | uint64(le.Uint16(b[pos:]))
		pos += 2
	}
	if pos+2*n > uint64(len(b)) {
		return "", false
	}
	units := make([]uint16, n)
	for k := range units {
		units[k] = le.Uint16(b[pos+uint64(k)*2:])
	}
	return string(utf16.Decode(units)), true
}

// attrValue is a typed attribute value.
type attrValue struct {
	present bool
	typ     uint8
	data    uint32
	str     string
}

func (v attrValue) isReference() bool {
	return v.present && (v.typ == typeReference || v.typ == typeDynamicReference)
}

// resolved follows a reference through the resource table.
func (v attrValue) resolved(res *resources) (attrValue, bool) {
	if !v.present {
		return v, false
	}
	if v.isReference() {
		rv, err := res.resolve(v.data)
		if err != nil {
			return attrValue{}, false
		}
		return rv, true
	}
	return v, true
}

func (v attrValue) stringValue(res *resources) (string, bool) {
	v, ok := v.resolved(res)
	if !ok || v.typ != typeString {
		return "", false
	}
	return v.str, true
}

func (v attrValue) uint32Value(res *resources) (uint32, bool) {
	v, ok := v.resolved(res)
	if !ok {
		return 0, false
	}
	switch v.typ {
	case typeIntDec, typeIntHex:
		return v.data, true
	case typeString:
		n, err := strconv.ParseUint(v.str, 10, 32)
		return uint32(n), err == nil
	}
	return 0, false
}

func (v attrValue) boolValue(res *resources) (bool, bool) {
	v, ok := v.resolved(res)
	if !ok {
		return false, false
	}
	switch v.typ {
	case typeIntBoolean, typeIntDec, typeIntHex:
		return v.data != 0, true
	case typeString:
		b, err := strconv.ParseBool(v.str)
		return b, err == nil
	}
	return false, false
}

// sdkLevel is an SDK level, or 0 for a codename or anything else.
func (v attrValue) sdkLevel(res *resources) int {
	n, ok := v.uint32Value(res)
	if !ok || n > 10_000 {
		return 0
	}
	return int(n)
}

// manifest is what Parse needs of AndroidManifest.xml.
type manifest struct {
	pkg                            string
	versionCode, versionCodeMajor  attrValue
	versionName, minSDK, targetSDK attrValue
	debuggable                     attrValue
	meta                           map[string]attrValue
}

type xmlAttr struct {
	ns, name uint32
	value    attrValue
}

// parseManifest reads an Android binary XML document.
func parseManifest(b []byte) (*manifest, error) {
	if len(b) < 8 || le.Uint16(b) != chunkXML {
		return nil, errors.New("not an Android binary XML document")
	}
	hs := uint32(le.Uint16(b[2:]))
	size := le.Uint32(b[4:])
	if hs < 8 || size > uint32(len(b)) || hs > size {
		return nil, errors.New("binary XML header out of bounds")
	}
	m := &manifest{meta: map[string]attrValue{}}
	var pool *stringPool
	var resMap []byte
	var stack []string
	for off := hs; off < size; {
		if size-off < 8 {
			return nil, errTruncated
		}
		typ := le.Uint16(b[off:])
		chs := uint32(le.Uint16(b[off+2:]))
		csz := le.Uint32(b[off+4:])
		if csz < 8 || csz > size-off || chs > csz {
			return nil, fmt.Errorf("chunk at %d out of bounds", off)
		}
		chunk := b[off : off+csz]
		switch typ {
		case chunkStringPool:
			if pool == nil {
				p, err := parseStringPool(chunk)
				if err != nil {
					return nil, err
				}
				pool = p
			}
		case chunkXMLResMap:
			if chs < 8 {
				return nil, errTruncated
			}
			resMap = chunk[chs:]
		case chunkXMLStart:
			if pool == nil {
				return nil, errors.New("element before the string pool")
			}
			name, attrs, err := readStart(chunk, chs, pool)
			if err != nil {
				return nil, err
			}
			if len(stack) >= maxDepth {
				return nil, errors.New("elements nested too deep")
			}
			stack = append(stack, name)
			m.element(stack, attrs, pool, resMap)
		case chunkXMLEnd:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		off += csz
	}
	if pool == nil {
		return nil, errors.New("no string pool")
	}
	return m, nil
}

func readStart(chunk []byte, hs uint32, pool *stringPool) (string, []xmlAttr, error) {
	if hs < 16 || uint64(hs)+20 > uint64(len(chunk)) {
		return "", nil, errTruncated
	}
	ext := chunk[hs:]
	name, _ := pool.get(le.Uint32(ext[4:]))
	start := uint64(le.Uint16(ext[8:]))
	asz := uint64(le.Uint16(ext[10:]))
	count := uint64(le.Uint16(ext[12:]))
	if count > 0 && asz < 20 {
		return "", nil, errors.New("attribute size too small")
	}
	if start+asz*count > uint64(len(ext)) {
		return "", nil, errors.New("attributes out of bounds")
	}
	attrs := make([]xmlAttr, 0, count)
	for i := range count {
		a := ext[start+i*asz:]
		v := attrValue{present: true, typ: a[15], data: le.Uint32(a[16:])}
		if v.typ == typeString {
			s, ok := pool.get(v.data)
			if !ok {
				return "", nil, errors.New("attribute string out of bounds")
			}
			v.str = s
		}
		attrs = append(attrs, xmlAttr{ns: le.Uint32(a), name: le.Uint32(a[4:]), value: v})
	}
	return name, attrs, nil
}

// element records what the manifest reader wants of one start element.
func (m *manifest) element(stack []string, attrs []xmlAttr, pool *stringPool, resMap []byte) {
	path := strings.Join(stack, "/")
	find := func(id uint32, name string) attrValue {
		for _, a := range attrs {
			if rid := resID(resMap, a.name); rid != 0 {
				if rid == id {
					return a.value
				}
				continue
			}
			if n, _ := pool.get(a.name); n == name {
				return a.value
			}
		}
		return attrValue{}
	}
	switch path {
	case "manifest":
		for _, a := range attrs {
			if resID(resMap, a.name) != 0 || a.ns != noIndex {
				continue
			}
			if n, _ := pool.get(a.name); n == "package" && a.value.typ == typeString {
				m.pkg = a.value.str
			}
		}
		m.versionCode = find(idVersionCode, "versionCode")
		m.versionCodeMajor = find(idVersionCodeMajor, "versionCodeMajor")
		m.versionName = find(idVersionName, "versionName")
	case "manifest/uses-sdk":
		m.minSDK = find(idMinSDKVersion, "minSdkVersion")
		m.targetSDK = find(idTargetSDKVersion, "targetSdkVersion")
	case "manifest/application":
		m.debuggable = find(idDebuggable, "debuggable")
	case "manifest/application/meta-data":
		n := find(idName, "name")
		if n.typ != typeString || n.str == "" {
			return
		}
		if v := find(idValue, "value"); v.present {
			m.meta[n.str] = v
		}
	}
}

func resID(resMap []byte, nameIndex uint32) uint32 {
	if uint64(nameIndex)*4+4 > uint64(len(resMap)) {
		return 0
	}
	return le.Uint32(resMap[nameIndex*4:])
}
