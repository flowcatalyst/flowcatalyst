package apk

import (
	"archive/zip"
	"errors"
	"fmt"
)

// Flags of ResTable_type and ResTable_entry.
const (
	typeFlagSparse   = 0x01
	typeFlagOffset16 = 0x02
	entryFlagComplex = 0x0001
	entryFlagCompact = 0x0008
)

// resources resolves references through resources.arsc, read on first use.
type resources struct {
	file   *zip.File
	loaded bool
	err    error
	table  *resTable
}

type resTable struct {
	strings *stringPool
	// types holds, per package id and resource type id, the type chunks.
	types map[uint16][]typeChunk
}

type typeChunk struct {
	chunk   []byte
	hs      uint32
	flags   uint8
	count   uint32
	start   uint32
	isFirst bool // the default configuration
}

// resolve is the value of resource id, following at most one further
// reference.
func (r *resources) resolve(id uint32) (attrValue, error) {
	if !r.loaded {
		r.loaded = true
		r.table, r.err = r.load()
	}
	if r.err != nil {
		return attrValue{}, r.err
	}
	v, err := r.table.lookup(id)
	if err != nil {
		return attrValue{}, err
	}
	if v.typ == typeReference || v.typ == typeDynamicReference {
		if v, err = r.table.lookup(v.data); err != nil {
			return attrValue{}, err
		}
		if v.typ == typeReference || v.typ == typeDynamicReference {
			return attrValue{}, errors.New("a reference to a reference to a reference")
		}
	}
	return v, nil
}

func (r *resources) load() (*resTable, error) {
	if r.file == nil {
		return nil, errors.New("the APK has no resources.arsc")
	}
	b, err := readEntry(r.file, maxResourcesBytes)
	if err != nil {
		return nil, fmt.Errorf("resources.arsc: %w", err)
	}
	return parseTable(b)
}

func parseTable(b []byte) (*resTable, error) {
	if len(b) < 12 || le.Uint16(b) != chunkTable {
		return nil, errors.New("resources.arsc is not a resource table")
	}
	hs := uint32(le.Uint16(b[2:]))
	size := le.Uint32(b[4:])
	if hs < 12 || size > uint32(len(b)) || hs > size {
		return nil, errors.New("resources.arsc header out of bounds")
	}
	t := &resTable{types: map[uint16][]typeChunk{}}
	for off := hs; off < size; {
		chunk, typ, chs, err := childChunk(b[:size], off)
		if err != nil {
			return nil, err
		}
		switch typ {
		case chunkStringPool:
			if t.strings == nil {
				if t.strings, err = parseStringPool(chunk); err != nil {
					return nil, err
				}
			}
		case chunkTablePackage:
			if err := t.addPackage(chunk, chs); err != nil {
				return nil, err
			}
		}
		off += uint32(len(chunk))
	}
	return t, nil
}

// childChunk is the chunk at off within b.
func childChunk(b []byte, off uint32) (chunk []byte, typ uint16, hs uint32, err error) {
	if uint64(off)+8 > uint64(len(b)) {
		return nil, 0, 0, errTruncated
	}
	typ = le.Uint16(b[off:])
	hs = uint32(le.Uint16(b[off+2:]))
	sz := le.Uint32(b[off+4:])
	if sz < 8 || uint64(off)+uint64(sz) > uint64(len(b)) || hs > sz || hs < 8 {
		return nil, 0, 0, fmt.Errorf("resource chunk at %d out of bounds", off)
	}
	return b[off : off+sz], typ, hs, nil
}

func (t *resTable) addPackage(p []byte, hs uint32) error {
	if hs < 12 {
		return errTruncated
	}
	id := le.Uint32(p[8:])
	if id > 0xff {
		return errors.New("package id out of range")
	}
	var typeIDOffset uint32
	if hs >= 288 {
		typeIDOffset = le.Uint32(p[284:])
	}
	for off := hs; off < uint32(len(p)); {
		chunk, typ, chs, err := childChunk(p, off)
		if err != nil {
			return err
		}
		if typ == chunkTableType && chs >= 20 && len(chunk) >= 24 {
			tc := typeChunk{
				chunk: chunk, hs: chs, flags: chunk[9],
				count: le.Uint32(chunk[12:]), start: le.Uint32(chunk[16:]),
			}
			cfgSize := le.Uint32(chunk[20:])
			if cfgSize >= 4 && 20+uint64(cfgSize) <= uint64(chs) {
				tc.isFirst = allZero(chunk[24 : 20+cfgSize])
			}
			typeID := uint32(chunk[8]) + typeIDOffset
			if typeID <= 0xff {
				key := uint16(id)<<8 | uint16(typeID) //nolint:gosec // G115: both checked to fit a byte
				t.types[key] = append(t.types[key], tc)
			}
		}
		off += uint32(len(chunk)) //nolint:gosec // G115: a chunk is within p, whose length came from a uint32 size
	}
	return nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// lookup is the simple value of resource id in its default configuration, or
// in the first configuration that has it.
func (t *resTable) lookup(id uint32) (attrValue, error) {
	key := uint16(id >> 16) //nolint:gosec // G115: the package and type bytes
	entry := id & 0xffff
	var found *attrValue
	for _, tc := range t.types[key] {
		v, ok := tc.value(entry, t.strings)
		if !ok {
			continue
		}
		if tc.isFirst {
			return v, nil
		}
		if found == nil {
			found = &v
		}
	}
	if found == nil {
		return attrValue{}, fmt.Errorf("resource 0x%08x is not in resources.arsc", id)
	}
	return *found, nil
}

func (tc typeChunk) value(entry uint32, strs *stringPool) (attrValue, bool) {
	c := tc.chunk
	idx := uint64(tc.hs)
	var off uint64
	switch {
	case tc.flags&typeFlagSparse != 0:
		hit := false
		for i := range uint64(tc.count) {
			p := idx + i*4
			if p+4 > uint64(len(c)) {
				return attrValue{}, false
			}
			if uint32(le.Uint16(c[p:])) == entry {
				off, hit = uint64(le.Uint16(c[p+2:]))*4, true
				break
			}
		}
		if !hit {
			return attrValue{}, false
		}
	case tc.flags&typeFlagOffset16 != 0:
		p := idx + uint64(entry)*2
		if entry >= tc.count || p+2 > uint64(len(c)) {
			return attrValue{}, false
		}
		o := le.Uint16(c[p:])
		if o == 0xffff {
			return attrValue{}, false
		}
		off = uint64(o) * 4
	default:
		p := idx + uint64(entry)*4
		if entry >= tc.count || p+4 > uint64(len(c)) {
			return attrValue{}, false
		}
		o := le.Uint32(c[p:])
		if o == noIndex {
			return attrValue{}, false
		}
		off = uint64(o)
	}
	e := uint64(tc.start) + off
	if e+8 > uint64(len(c)) {
		return attrValue{}, false
	}
	size := uint64(le.Uint16(c[e:]))
	flags := le.Uint16(c[e+2:])
	var v attrValue
	switch {
	case flags&entryFlagCompact != 0:
		v = attrValue{present: true, typ: uint8(flags >> 8), data: le.Uint32(c[e+4:])}
	case flags&entryFlagComplex != 0:
		return attrValue{}, false
	default:
		r := e + size
		if size < 8 || r+8 > uint64(len(c)) {
			return attrValue{}, false
		}
		v = attrValue{present: true, typ: c[r+3], data: le.Uint32(c[r+4:])}
	}
	if v.typ == typeString {
		s, ok := strs.get(v.data)
		if !ok {
			return attrValue{}, false
		}
		v.str = s
	}
	if v.typ == typeNull {
		return attrValue{}, false
	}
	return v, true
}
