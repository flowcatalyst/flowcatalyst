// Package apktest builds small APKs in memory for tests, offline: a binary
// AndroidManifest.xml with the expo-updates meta-data as the Expo config
// plugin writes it, a resources.arsc holding its @string values, assets, and
// an APK Signature Scheme v2 (and optionally v3) signature. `apksigner verify`
// and `aapt2 dump` accept what it builds (package apk, "Verified against").
package apktest

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"maps"
	"math/big"
	mrand "math/rand/v2"
	"sort"
	"time"
	"unicode/utf16"
)

// Options say what the APK holds.
type Options struct {
	Package     string // default com.example.driver
	VersionCode int64
	VersionName string
	MinSDK      int
	TargetSDK   int
	Debuggable  bool
	// RuntimeVersion is written as a @string reference through
	// resources.arsc unless RuntimeVersionLiteral; "" writes none.
	RuntimeVersion        string
	RuntimeVersionLiteral bool
	// Fingerprint, when set, makes EXPO_RUNTIME_VERSION a @string reference
	// to "file:fingerprint" and assets/fingerprint this text.
	Fingerprint string
	// UpdateURL, Channel and KeyID are the meta-data Expo writes: the update
	// URL, the request headers {"expo-channel-name": Channel} and the code
	// signing metadata {"keyid": KeyID, "alg": "rsa-v1_5-sha256"}.
	UpdateURL, Channel, KeyID string
	// Filler is how many bytes of incompressible filler an extra stored
	// asset holds.
	Filler int
	// Signer signs the APK; nil leaves it unsigned.
	Signer *Signer
	// V3 adds an APK Signature Scheme v3 block beside v2 (same signer).
	V3 bool
	// ExtraFiles adds entries, or replaces one of the same name (for
	// example AndroidManifest.xml).
	ExtraFiles map[string][]byte
}

// Signer is an RSA 2048 key and its self-signed certificate.
type Signer struct {
	key     *rsa.PrivateKey
	certDER []byte
}

// NewSigner makes a signer; commonName "Android Debug" makes the subject of
// the Android SDK's debug keystore.
func NewSigner(commonName string) (*Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	subject := pkix.Name{CommonName: commonName}
	if commonName == "Android Debug" {
		subject = pkix.Name{CommonName: commonName, Organization: []string{"Android"}, Country: []string{"US"}}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial.Add(serial, big.NewInt(1)), Subject: subject, Issuer: subject,
		NotBefore: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC),
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, certDER: der}, nil
}

// CertificateSHA256 is the lower-case hex SHA-256 of the DER certificate.
func (s *Signer) CertificateSHA256() string {
	sum := sha256.Sum256(s.certDER)
	return hex.EncodeToString(sum[:])
}

// CertificateDER is the certificate.
func (s *Signer) CertificateDER() []byte { return bytes.Clone(s.certDER) }

var le = binary.LittleEndian

// Build makes the APK.
func Build(o Options) ([]byte, error) {
	if o.Package == "" {
		o.Package = "com.example.driver"
	}
	res := &arsc{}
	appName := res.add("app_name", "Driver")
	man := &manifestDoc{o: o, res: res, appName: appName}
	manifest, err := man.encode()
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{
		"AndroidManifest.xml": manifest,
		"resources.arsc":      res.encode(o.Package),
		"classes.dex":         []byte("dex\n035\x00"),
	}
	if o.Fingerprint != "" {
		files["assets/fingerprint"] = []byte(o.Fingerprint)
	}
	if o.Filler > 0 {
		filler := make([]byte, o.Filler)
		r := mrand.NewChaCha8([32]byte{1, 2, 3})
		_, _ = r.Read(filler)
		files["assets/filler.bin"] = filler
	}
	maps.Copy(files, o.ExtraFiles)
	z, err := writeZip(files)
	if err != nil {
		return nil, err
	}
	if o.Signer == nil {
		return z, nil
	}
	return sign(z, o.Signer, o.V3)
}

// writeZip writes the entries without data descriptors: stored for
// resources.arsc and the filler (as Android wants resources.arsc), deflated
// for the rest, the manifest first.
func writeZip(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		if n != "AndroidManifest.xml" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	names = append([]string{"AndroidManifest.xml"}, names...)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		body, ok := files[name]
		if !ok {
			continue
		}
		h := &zip.FileHeader{
			Name: name, Method: zip.Deflate, CRC32: crc32.ChecksumIEEE(body), UncompressedSize64: uint64(len(body)),
			Modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		data := body
		if name == "resources.arsc" || name == "assets/filler.bin" {
			h.Method = zip.Store
		} else {
			var c bytes.Buffer
			fw, err := flate.NewWriter(&c, flate.BestCompression)
			if err != nil {
				return nil, err
			}
			if _, err := fw.Write(body); err != nil {
				return nil, err
			}
			if err := fw.Close(); err != nil {
				return nil, err
			}
			data = c.Bytes()
		}
		h.CompressedSize64 = uint64(len(data))
		w, err := zw.CreateRaw(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── binary XML ─────────────────────────────────────────────────────────────

const androidNS = "http://schemas.android.com/apk/res/android"

// The framework attributes the manifest uses, in resource-id order.
var attrIDs = []struct {
	name string
	id   uint32
}{
	{"label", 0x01010001},
	{"name", 0x01010003},
	{"debuggable", 0x0101000f},
	{"value", 0x01010024},
	{"minSdkVersion", 0x0101020c},
	{"versionCode", 0x0101021b},
	{"versionName", 0x0101021c},
	{"targetSdkVersion", 0x01010270},
}

type manifestDoc struct {
	o       Options
	res     *arsc
	appName uint32
	strs    []string
	index   map[string]uint32
	body    bytes.Buffer
}

func (m *manifestDoc) str(s string) uint32 {
	if i, ok := m.index[s]; ok {
		return i
	}
	i := uint32(len(m.strs)) //nolint:gosec // G115: a handful of strings
	m.strs = append(m.strs, s)
	m.index[s] = i
	return i
}

type xattr struct {
	ns      bool
	name    string
	typ     uint8
	data    uint32
	strVal  string
	isStr   bool
	sortKey uint32
}

func strAttr(name, v string) xattr {
	return xattr{ns: true, name: name, typ: 0x03, strVal: v, isStr: true}
}

func intAttr(name string, v uint32) xattr {
	return xattr{ns: true, name: name, typ: 0x10, data: v}
}

func boolAttr(name string, v bool) xattr {
	a := xattr{ns: true, name: name, typ: 0x12}
	if v {
		a.data = 0xffffffff
	}
	return a
}

func refAttr(name string, id uint32) xattr { return xattr{ns: true, name: name, typ: 0x01, data: id} }

func (m *manifestDoc) encode() ([]byte, error) {
	m.index = map[string]uint32{}
	for _, a := range attrIDs {
		m.str(a.name)
	}
	o := m.o
	m.namespace(0x0100)
	man := []xattr{intAttr("versionCode", uint32(o.VersionCode&0xffffffff)), {name: "package", typ: 0x03, strVal: o.Package, isStr: true}} //nolint:gosec // G115: the low 32 bits
	if o.VersionName != "" {
		man = append(man, strAttr("versionName", o.VersionName))
	}
	m.start("manifest", man)
	var sdk []xattr
	if o.MinSDK > 0 {
		sdk = append(sdk, intAttr("minSdkVersion", uint32(o.MinSDK))) //nolint:gosec // G115: an SDK level
	}
	if o.TargetSDK > 0 {
		sdk = append(sdk, intAttr("targetSdkVersion", uint32(o.TargetSDK))) //nolint:gosec // G115: an SDK level
	}
	m.start("uses-sdk", sdk)
	m.end("uses-sdk")
	app := []xattr{refAttr("label", 0x7f010000|m.appName)}
	if o.Debuggable {
		app = append(app, boolAttr("debuggable", true))
	}
	m.start("application", app)
	meta := func(name string, v xattr) {
		v.name = "value"
		m.start("meta-data", []xattr{strAttr("name", name), v})
		m.end("meta-data")
	}
	meta("com.facebook.soloader.enabled", boolAttr("", true))
	if o.KeyID != "" {
		b, _ := json.Marshal(map[string]string{"keyid": o.KeyID, "alg": "rsa-v1_5-sha256"})
		meta("expo.modules.updates.CODE_SIGNING_METADATA", strAttr("", string(b)))
	}
	expo := o.UpdateURL != "" || o.RuntimeVersion != "" || o.Fingerprint != ""
	if expo {
		meta("expo.modules.updates.ENABLED", boolAttr("", true))
	}
	switch {
	case o.Fingerprint != "":
		meta("expo.modules.updates.EXPO_RUNTIME_VERSION", refAttr("", 0x7f010000|m.res.add("expo_runtime_version", "file:fingerprint")))
	case o.RuntimeVersion != "" && o.RuntimeVersionLiteral:
		meta("expo.modules.updates.EXPO_RUNTIME_VERSION", strAttr("", o.RuntimeVersion))
	case o.RuntimeVersion != "":
		meta("expo.modules.updates.EXPO_RUNTIME_VERSION", refAttr("", 0x7f010000|m.res.add("expo_runtime_version", o.RuntimeVersion)))
	}
	if o.UpdateURL != "" {
		meta("expo.modules.updates.EXPO_UPDATE_URL", strAttr("", o.UpdateURL))
	}
	if o.Channel != "" {
		b, _ := json.Marshal(map[string]string{"expo-channel-name": o.Channel})
		meta("expo.modules.updates.UPDATES_CONFIGURATION_REQUEST_HEADERS_KEY", strAttr("", string(b)))
	}
	m.end("application")
	m.end("manifest")
	m.namespace(0x0101)

	pool, err := stringPoolChunk(m.strs, false)
	if err != nil {
		return nil, err
	}
	resMap := make([]byte, 8+4*len(attrIDs))
	le.PutUint16(resMap, 0x0180)
	le.PutUint16(resMap[2:], 8)
	le.PutUint32(resMap[4:], uint32(len(resMap))) //nolint:gosec // G115: small
	for i, a := range attrIDs {
		le.PutUint32(resMap[8+4*i:], a.id)
	}
	size := 8 + len(pool) + len(resMap) + m.body.Len()
	out := make([]byte, 8, size)
	le.PutUint16(out, 0x0003)
	le.PutUint16(out[2:], 8)
	le.PutUint32(out[4:], uint32(size)) //nolint:gosec // G115: small
	out = append(out, pool...)
	out = append(out, resMap...)
	return append(out, m.body.Bytes()...), nil
}

func (m *manifestDoc) node(typ uint16, size int) []byte {
	b := make([]byte, 16, size)
	le.PutUint16(b, typ)
	le.PutUint16(b[2:], 16)
	le.PutUint32(b[4:], uint32(size)) //nolint:gosec // G115: small
	le.PutUint32(b[8:], 1)
	le.PutUint32(b[12:], 0xffffffff)
	return b
}

func (m *manifestDoc) namespace(typ uint16) {
	b := m.node(typ, 24)
	b = le.AppendUint32(b, m.str("android"))
	b = le.AppendUint32(b, m.str(androidNS))
	m.body.Write(b)
}

func (m *manifestDoc) start(name string, attrs []xattr) {
	for i := range attrs {
		attrs[i].sortKey = 0xffffffff
		for _, a := range attrIDs {
			if a.name == attrs[i].name && attrs[i].ns {
				attrs[i].sortKey = a.id
			}
		}
	}
	sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].sortKey < attrs[j].sortKey })
	size := 16 + 20 + 20*len(attrs)
	b := m.node(0x0102, size)
	b = le.AppendUint32(b, 0xffffffff)
	b = le.AppendUint32(b, m.str(name))
	b = le.AppendUint16(b, 20)
	b = le.AppendUint16(b, 20)
	b = le.AppendUint16(b, uint16(len(attrs))) //nolint:gosec // G115: a few attributes
	b = append(b, 0, 0, 0, 0, 0, 0)
	for _, a := range attrs {
		ns := uint32(0xffffffff)
		if a.ns {
			ns = m.str(androidNS)
		}
		b = le.AppendUint32(b, ns)
		b = le.AppendUint32(b, m.str(a.name))
		data, raw := a.data, uint32(0xffffffff)
		if a.isStr {
			data = m.str(a.strVal)
			raw = data
		}
		b = le.AppendUint32(b, raw)
		b = le.AppendUint16(b, 8)
		b = append(b, 0, a.typ)
		b = le.AppendUint32(b, data)
	}
	m.body.Write(b)
}

func (m *manifestDoc) end(name string) {
	b := m.node(0x0103, 24)
	b = le.AppendUint32(b, 0xffffffff)
	b = le.AppendUint32(b, m.str(name))
	m.body.Write(b)
}

// stringPoolChunk encodes a ResStringPool, UTF-8 or UTF-16.
func stringPoolChunk(strs []string, utf8 bool) ([]byte, error) {
	var data []byte
	offsets := make([]uint32, len(strs))
	for i, s := range strs {
		offsets[i] = uint32(len(data)) //nolint:gosec // G115: small
		if utf8 {
			u16 := len(utf16.Encode([]rune(s)))
			for _, n := range []int{u16, len(s)} {
				switch {
				case n < 0x80:
					data = append(data, byte(n))
				case n < 0x8000:
					data = append(data, byte(0x80|n>>8), byte(n))
				default:
					return nil, errors.New("string too long")
				}
			}
			data = append(data, s...)
			data = append(data, 0)
			continue
		}
		units := utf16.Encode([]rune(s))
		if len(units) >= 0x8000 {
			return nil, errors.New("string too long")
		}
		data = le.AppendUint16(data, uint16(len(units))) //nolint:gosec // G115: checked
		for _, u := range units {
			data = le.AppendUint16(data, u)
		}
		data = append(data, 0, 0)
	}
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	hs := 28
	start := hs + 4*len(strs)
	size := start + len(data)
	b := make([]byte, 0, size)
	b = le.AppendUint16(b, 0x0001)
	b = le.AppendUint16(b, uint16(hs))        //nolint:gosec // G115: constant
	b = le.AppendUint32(b, uint32(size))      //nolint:gosec // G115: small
	b = le.AppendUint32(b, uint32(len(strs))) //nolint:gosec // G115: small
	b = le.AppendUint32(b, 0)                 // styles
	flags := uint32(0)
	if utf8 {
		flags = 1 << 8
	}
	b = le.AppendUint32(b, flags)
	b = le.AppendUint32(b, uint32(start)) //nolint:gosec // G115: small
	b = le.AppendUint32(b, 0)
	for _, o := range offsets {
		b = le.AppendUint32(b, o)
	}
	return append(b, data...), nil
}

// ── resources.arsc ─────────────────────────────────────────────────────────

// arsc is a resource table of one package holding string resources of
// type 1, all in the default configuration: resource 0x7f01xxxx.
type arsc struct {
	keys, values []string
}

func (a *arsc) add(key, value string) uint32 {
	a.keys = append(a.keys, key)
	a.values = append(a.values, value)
	return uint32(len(a.keys) - 1) //nolint:gosec // G115: a few entries
}

func (a *arsc) encode(pkg string) []byte {
	global, _ := stringPoolChunk(a.values, true)
	typePool, _ := stringPoolChunk([]string{"string"}, false)
	keyPool, _ := stringPoolChunk(a.keys, false)
	n := len(a.keys)

	spec := make([]byte, 0, 16+4*n)
	spec = le.AppendUint16(spec, 0x0202)
	spec = le.AppendUint16(spec, 16)
	spec = le.AppendUint32(spec, uint32(16+4*n)) //nolint:gosec // G115: small
	spec = append(spec, 1, 0, 0, 0)
	spec = le.AppendUint32(spec, uint32(n)) //nolint:gosec // G115: small
	for range n {
		spec = le.AppendUint32(spec, 0)
	}

	const cfgSize = 64
	hs := 20 + cfgSize
	entriesStart := hs + 4*n
	size := entriesStart + 16*n
	typ := make([]byte, 0, size)
	typ = le.AppendUint16(typ, 0x0201)
	typ = le.AppendUint16(typ, uint16(hs))   //nolint:gosec // G115: constant
	typ = le.AppendUint32(typ, uint32(size)) //nolint:gosec // G115: small
	typ = append(typ, 1, 0, 0, 0)
	typ = le.AppendUint32(typ, uint32(n))            //nolint:gosec // G115: small
	typ = le.AppendUint32(typ, uint32(entriesStart)) //nolint:gosec // G115: small
	typ = le.AppendUint32(typ, cfgSize)
	typ = append(typ, make([]byte, cfgSize-4)...)
	for i := range n {
		typ = le.AppendUint32(typ, uint32(16*i)) //nolint:gosec // G115: small
	}
	for i := range n {
		typ = le.AppendUint16(typ, 8)
		typ = le.AppendUint16(typ, 0)
		typ = le.AppendUint32(typ, uint32(i)) //nolint:gosec // G115: small
		typ = le.AppendUint16(typ, 8)
		typ = append(typ, 0, 0x03)
		typ = le.AppendUint32(typ, uint32(i)) //nolint:gosec // G115: small
	}

	const pkgHS = 288
	pkgSize := pkgHS + len(typePool) + len(keyPool) + len(spec) + len(typ)
	p := make([]byte, 0, pkgSize)
	p = le.AppendUint16(p, 0x0200)
	p = le.AppendUint16(p, pkgHS)
	p = le.AppendUint32(p, uint32(pkgSize)) //nolint:gosec // G115: small
	p = le.AppendUint32(p, 0x7f)
	name := make([]byte, 256)
	for i, u := range utf16.Encode([]rune(pkg)) {
		if i < 127 {
			le.PutUint16(name[2*i:], u)
		}
	}
	p = append(p, name...)
	p = le.AppendUint32(p, pkgHS)
	p = le.AppendUint32(p, 0)
	p = le.AppendUint32(p, uint32(pkgHS+len(typePool))) //nolint:gosec // G115: small
	p = le.AppendUint32(p, 0)
	p = le.AppendUint32(p, 0)
	p = append(p, typePool...)
	p = append(p, keyPool...)
	p = append(p, spec...)
	p = append(p, typ...)

	total := 12 + len(global) + len(p)
	out := make([]byte, 0, total)
	out = le.AppendUint16(out, 0x0002)
	out = le.AppendUint16(out, 12)
	out = le.AppendUint32(out, uint32(total)) //nolint:gosec // G115: small
	out = le.AppendUint32(out, 1)
	out = append(out, global...)
	return append(out, p...)
}

// ── APK Signature Scheme v2 / v3 ───────────────────────────────────────────

func lp(b []byte) []byte {
	return append(le.AppendUint32(nil, uint32(len(b))), b...) //nolint:gosec // G115: small
}

// sign inserts an APK Signing Block before the central directory of the
// unsigned zip z.
func sign(z []byte, s *Signer, v3 bool) ([]byte, error) {
	eocd := len(z) - 22
	if eocd < 0 || le.Uint32(z[eocd:]) != 0x06054b50 {
		return nil, errors.New("apktest: not a zip without a comment")
	}
	cd := int(le.Uint32(z[eocd+16:]))
	digest := chunkedSHA256(z[:cd], z[cd:eocd], z[eocd:])

	pub, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		return nil, err
	}
	const alg = 0x0103
	digests := lp(lp(append(le.AppendUint32(nil, alg), lp(digest)...)))
	certs := lp(lp(s.certDER))
	signer := func(signed []byte, sdkRange []byte) ([]byte, error) {
		h := sha256.Sum256(signed)
		sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, h[:])
		if err != nil {
			return nil, err
		}
		out := lp(signed)
		out = append(out, sdkRange...)
		out = append(out, lp(lp(append(le.AppendUint32(nil, alg), lp(sig)...)))...)
		return append(out, lp(pub)...), nil
	}
	pairs := map[uint32][]byte{}
	v2signed := append(append(bytes.Clone(digests), certs...), lp(nil)...)
	v2, err := signer(v2signed, nil)
	if err != nil {
		return nil, err
	}
	pairs[0x7109871a] = lp(lp(v2))
	if v3 {
		sdkRange := le.AppendUint32(le.AppendUint32(nil, 28), 0x7fffffff)
		signed := append(append(append(bytes.Clone(digests), certs...), sdkRange...), lp(nil)...)
		v3s, err := signer(signed, sdkRange)
		if err != nil {
			return nil, err
		}
		pairs[0xf05368c0] = lp(lp(v3s))
	}
	var body []byte
	for _, id := range []uint32{0x7109871a, 0xf05368c0} {
		v, ok := pairs[id]
		if !ok {
			continue
		}
		body = le.AppendUint64(body, uint64(4+len(v)))
		body = le.AppendUint32(body, id)
		body = append(body, v...)
	}
	size := uint64(len(body) + 8 + 16)
	block := le.AppendUint64(nil, size)
	block = append(block, body...)
	block = le.AppendUint64(block, size)
	block = append(block, "APK Sig Block 42"...)

	out := make([]byte, 0, len(z)+len(block))
	out = append(out, z[:cd]...)
	out = append(out, block...)
	out = append(out, z[cd:eocd]...)
	end := bytes.Clone(z[eocd:])
	le.PutUint32(end[16:], uint32(cd+len(block))) //nolint:gosec // G115: a small test file
	return append(out, end...), nil
}

func chunkedSHA256(sections ...[]byte) []byte {
	const chunk = 1 << 20
	var sums [][]byte
	for _, sec := range sections {
		for off := 0; off < len(sec); off += chunk {
			c := sec[off:min(off+chunk, len(sec))]
			h := sha256.New()
			h.Write([]byte{0xa5})
			h.Write(le.AppendUint32(nil, uint32(len(c)))) //nolint:gosec // G115: at most 1 MiB
			h.Write(c)
			sums = append(sums, h.Sum(nil))
		}
	}
	h := sha256.New()
	h.Write([]byte{0x5a})
	h.Write(le.AppendUint32(nil, uint32(len(sums)))) //nolint:gosec // G115: small
	for _, s := range sums {
		h.Write(s)
	}
	return h.Sum(nil)
}
