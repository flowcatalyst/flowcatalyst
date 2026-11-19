package apk

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	_ "crypto/sha512" // crypto.SHA512.New
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// APK Signature Scheme v2 and v3 (source.android.com/docs/security/features/apksigning).
const (
	blockIDv2      = 0x7109871a
	blockIDv3      = 0xf05368c0
	sigBlockMagic  = "APK Sig Block 42"
	maxSigBlock    = 16 << 20
	maxSigners     = 10
	digestChunk    = 1 << 20
	eocdSignature  = 0x06054b50
	eocdMinSize    = 22
	maxZipComment  = 0xffff
	debugSubjectCN = "Android Debug"
)

// Signature algorithm ids.
const (
	algRSAPSSSHA256   = 0x0101
	algRSAPSSSHA512   = 0x0102
	algRSAPKCS1SHA256 = 0x0103
	algRSAPKCS1SHA512 = 0x0104
	algECDSASHA256    = 0x0201
	algECDSASHA512    = 0x0202
)

// algStrength orders the supported algorithms (apksigner prefers the
// SHA-512 content digest); 0 is unsupported.
func algStrength(alg uint32) int {
	switch alg {
	case algRSAPSSSHA512, algRSAPKCS1SHA512, algECDSASHA512:
		return 2
	case algRSAPSSSHA256, algRSAPKCS1SHA256, algECDSASHA256:
		return 1
	}
	return 0
}

func algHash(alg uint32) crypto.Hash {
	if algStrength(alg) == 2 {
		return crypto.SHA512
	}
	return crypto.SHA256
}

// zipLayout is where the end of central directory record says things are.
type zipLayout struct {
	cdOffset   int64
	cdSize     int64
	eocdOffset int64
	eocd       []byte
	entries    int
}

func findEOCD(r io.ReaderAt, size int64) (*zipLayout, error) {
	if size < eocdMinSize {
		return nil, refuse(CodeNotAnAPK, "the file is too short to be a zip file")
	}
	n := min(size, eocdMinSize+maxZipComment)
	tail := make([]byte, n)
	if _, err := r.ReadAt(tail, size-n); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	for i := len(tail) - eocdMinSize; i >= 0; i-- {
		if le.Uint32(tail[i:]) != eocdSignature || i+eocdMinSize+int(le.Uint16(tail[i+20:])) != len(tail) {
			continue
		}
		l := &zipLayout{
			eocd: bytes.Clone(tail[i:]), eocdOffset: size - n + int64(i),
			cdSize: int64(le.Uint32(tail[i+12:])), cdOffset: int64(le.Uint32(tail[i+16:])),
			entries: int(le.Uint16(tail[i+10:])),
		}
		if l.cdOffset == 0xffffffff || l.cdSize == 0xffffffff || l.entries == 0xffff {
			return nil, refuse(CodeNotAnAPK, "ZIP64 files are not APKs Android installs")
		}
		if l.cdOffset+l.cdSize != l.eocdOffset {
			return nil, refuse(CodeNotAnAPK, "the central directory does not end where the end record begins")
		}
		return l, nil
	}
	return nil, refuse(CodeNotAnAPK, "not a zip file: no end of central directory record")
}

// signerResult is one verified signer.
type signerResult struct {
	certDER []byte
	cert    *x509.Certificate
	alg     uint32
	digest  []byte
}

// verifySigning finds the APK Signing Block and verifies its v2 and v3 signers.
func verifySigning(r io.ReaderAt, l *zipLayout, hasV1 bool) (*Signing, error) {
	blocks, sigStart, err := readSigningBlock(r, l)
	if err != nil {
		return nil, err
	}
	v2, v3 := blocks[blockIDv2], blocks[blockIDv3]
	if v2 == nil && v3 == nil {
		if hasV1 {
			return nil, refuse(CodeUnsupportedSignature,
				"the APK is signed with v1 (JAR) signing only; Android 11 and later need APK Signature Scheme v2 or v3 for apps targeting SDK 30 or later: sign with v2 enabled")
		}
		return nil, refuse(CodeUnsigned, "the APK is not signed (no APK Signature Scheme v2 or v3 block)")
	}
	digests := map[crypto.Hash][]byte{}
	contentDigest := func(h crypto.Hash) ([]byte, error) {
		if d, ok := digests[h]; ok {
			return d, nil
		}
		d, err := chunkedDigest(r, l, sigStart, h)
		if err != nil {
			return nil, err
		}
		digests[h] = d
		return d, nil
	}
	out := &Signing{}
	var used *signerResult
	for _, scheme := range []struct {
		n     int
		value []byte
	}{{2, v2}, {3, v3}} {
		if scheme.value == nil {
			continue
		}
		signers, err := verifySigners(scheme.value, scheme.n == 3)
		if err != nil {
			return nil, err
		}
		for _, s := range signers {
			want, err := contentDigest(algHash(s.alg))
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(want, s.digest) {
				return nil, refuse(CodeSignatureInvalid, "the APK's contents do not match its v%d signature: it was changed after signing", scheme.n)
			}
		}
		out.Schemes = append(out.Schemes, scheme.n)
		used = &signers[0]
	}
	sum := sha256.Sum256(used.certDER)
	out.CertificateSHA256 = hex.EncodeToString(sum[:])
	out.CertificateSubject = used.cert.Subject.String()
	out.DebugCertificate = used.cert.Subject.CommonName == debugSubjectCN
	return out, nil
}

// readSigningBlock returns the ID-value pairs of the APK Signing Block that
// ends where the central directory begins, and the block's offset (nil when
// there is none).
func readSigningBlock(r io.ReaderAt, l *zipLayout) (map[uint32][]byte, int64, error) {
	if l.cdOffset < 32 {
		return nil, l.cdOffset, nil
	}
	footer := make([]byte, 24)
	if _, err := r.ReadAt(footer, l.cdOffset-24); err != nil {
		return nil, 0, refuse(CodeNotAnAPK, "read: %v", err)
	}
	if string(footer[8:]) != sigBlockMagic {
		return nil, l.cdOffset, nil
	}
	size := le.Uint64(footer)
	if size < 24 || size > maxSigBlock || int64(size)+8 > l.cdOffset { //nolint:gosec // G115: size ≤ maxSigBlock
		return nil, 0, refuse(CodeSignatureInvalid, "the APK Signing Block is malformed")
	}
	start := l.cdOffset - int64(size) - 8 //nolint:gosec // G115: size ≤ maxSigBlock
	block := make([]byte, size+8)
	if _, err := r.ReadAt(block, start); err != nil {
		return nil, 0, refuse(CodeNotAnAPK, "read: %v", err)
	}
	if le.Uint64(block) != size {
		return nil, 0, refuse(CodeSignatureInvalid, "the APK Signing Block's two sizes differ")
	}
	pairs := block[8 : len(block)-24]
	out := map[uint32][]byte{}
	for len(pairs) > 0 {
		if len(pairs) < 12 {
			return nil, 0, refuse(CodeSignatureInvalid, "the APK Signing Block is malformed")
		}
		n := le.Uint64(pairs)
		if n < 4 || n > uint64(len(pairs)-8) {
			return nil, 0, refuse(CodeSignatureInvalid, "the APK Signing Block is malformed")
		}
		id := le.Uint32(pairs[8:])
		if _, dup := out[id]; !dup {
			out[id] = pairs[12 : 8+n]
		}
		pairs = pairs[8+n:]
	}
	return out, start, nil
}

// lp splits a little-endian uint32 length-prefixed value off b.
func lp(b []byte) (value, rest []byte, err error) {
	if len(b) < 4 {
		return nil, nil, errTruncated
	}
	n := le.Uint32(b)
	if uint64(n) > uint64(len(b)-4) {
		return nil, nil, errTruncated
	}
	return b[4 : 4+n], b[4+n:], nil
}

func u32(b []byte) (uint32, []byte, error) {
	if len(b) < 4 {
		return 0, nil, errTruncated
	}
	return le.Uint32(b), b[4:], nil
}

// verifySigners verifies every signer of a v2 or v3 block value.
func verifySigners(value []byte, v3 bool) ([]signerResult, error) {
	scheme := 2
	if v3 {
		scheme = 3
	}
	bad := func(format string, args ...any) error {
		return refuse(CodeSignatureInvalid, "v%d signature: "+format, append([]any{scheme}, args...)...)
	}
	seq, _, err := lp(value)
	if err != nil {
		return nil, bad("malformed")
	}
	var out []signerResult
	for len(seq) > 0 {
		if len(out) == maxSigners {
			return nil, bad("more than %d signers", maxSigners)
		}
		var signer []byte
		if signer, seq, err = lp(seq); err != nil {
			return nil, bad("malformed signer")
		}
		res, err := verifySigner(signer, v3)
		if err != nil {
			if _, ok := errors.AsType[*Error](err); ok {
				return nil, err
			}
			return nil, bad("%v", err)
		}
		out = append(out, *res)
	}
	if len(out) == 0 {
		return nil, bad("no signers")
	}
	return out, nil
}

type algValue struct {
	alg   uint32
	value []byte
}

func readAlgValues(seq []byte) ([]algValue, error) {
	var out []algValue
	for len(seq) > 0 {
		var rec []byte
		var err error
		if rec, seq, err = lp(seq); err != nil {
			return nil, err
		}
		alg, rest, err := u32(rec)
		if err != nil {
			return nil, err
		}
		v, _, err := lp(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, algValue{alg, v})
	}
	return out, nil
}

func verifySigner(signer []byte, v3 bool) (*signerResult, error) {
	signedData, rest, err := lp(signer)
	if err != nil {
		return nil, err
	}
	var minSDK, maxSDK uint32
	if v3 {
		if minSDK, rest, err = u32(rest); err != nil {
			return nil, err
		}
		if maxSDK, rest, err = u32(rest); err != nil {
			return nil, err
		}
		if minSDK > maxSDK {
			return nil, errors.New("min SDK above max SDK")
		}
	}
	sigSeq, rest, err := lp(rest)
	if err != nil {
		return nil, err
	}
	pubDER, _, err := lp(rest)
	if err != nil {
		return nil, err
	}
	sigs, err := readAlgValues(sigSeq)
	if err != nil || len(sigs) == 0 {
		return nil, errors.New("no signatures")
	}
	best := -1
	for i, s := range sigs {
		if algStrength(s.alg) > 0 && (best < 0 || algStrength(s.alg) > algStrength(sigs[best].alg)) {
			best = i
		}
	}
	if best < 0 {
		return nil, refuse(CodeUnsupportedSignature, "the APK is signed only with algorithms this platform does not verify (DSA or verity)")
	}
	pub, err := x509.ParsePKIXPublicKey(pubDER)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	if err := verifySignature(sigs[best].alg, pub, signedData, sigs[best].value); err != nil {
		return nil, err
	}

	digestSeq, sd, err := lp(signedData)
	if err != nil {
		return nil, err
	}
	certSeq, sd, err := lp(sd)
	if err != nil {
		return nil, err
	}
	if v3 {
		var smin, smax uint32
		if smin, sd, err = u32(sd); err != nil {
			return nil, err
		}
		if smax, sd, err = u32(sd); err != nil {
			return nil, err
		}
		if smin != minSDK || smax != maxSDK {
			return nil, errors.New("the signed SDK range differs from the signer's")
		}
	}
	if _, _, err := lp(sd); err != nil {
		return nil, err
	}
	digests, err := readAlgValues(digestSeq)
	if err != nil {
		return nil, err
	}
	if len(digests) != len(sigs) {
		return nil, errors.New("the digests and signatures name different algorithms")
	}
	var digest []byte
	for i, d := range digests {
		if d.alg != sigs[i].alg {
			return nil, errors.New("the digests and signatures name different algorithms")
		}
		if d.alg == sigs[best].alg {
			digest = d.value
		}
	}
	certDER, _, err := lp(certSeq)
	if err != nil || len(certDER) == 0 {
		return nil, errors.New("no certificate")
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("certificate: %w", err)
	}
	if !bytes.Equal(cert.RawSubjectPublicKeyInfo, pubDER) {
		return nil, errors.New("the public key is not the certificate's")
	}
	return &signerResult{certDER: certDER, cert: cert, alg: sigs[best].alg, digest: digest}, nil
}

func verifySignature(alg uint32, pub any, data, sig []byte) error {
	h := algHash(alg)
	hh := h.New()
	hh.Write(data)
	sum := hh.Sum(nil)
	var ok bool
	switch alg {
	case algRSAPKCS1SHA256, algRSAPKCS1SHA512:
		k, isRSA := pub.(*rsa.PublicKey)
		ok = isRSA && rsa.VerifyPKCS1v15(k, h, sum, sig) == nil
	case algRSAPSSSHA256, algRSAPSSSHA512:
		k, isRSA := pub.(*rsa.PublicKey)
		ok = isRSA && rsa.VerifyPSS(k, h, sum, sig, &rsa.PSSOptions{SaltLength: h.Size(), Hash: h}) == nil
	case algECDSASHA256, algECDSASHA512:
		k, isEC := pub.(*ecdsa.PublicKey)
		ok = isEC && ecdsa.VerifyASN1(k, sum, sig)
	}
	if !ok {
		return errors.New("the signature does not verify")
	}
	return nil
}

func cmpErr(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// chunkedDigest is the APK's content digest: 1 MiB chunks of the entries,
// the central directory and the end record (its central directory offset
// replaced by the signing block's), each hashed as 0xa5 || len || chunk, the
// whole as 0x5a || count || chunk digests.
func chunkedDigest(r io.ReaderAt, l *zipLayout, sigStart int64, h crypto.Hash) ([]byte, error) {
	eocd := bytes.Clone(l.eocd)
	le.PutUint32(eocd[16:], uint32(sigStart)) //nolint:gosec // G115: below cdOffset, a uint32
	sections := []io.ReaderAt{io.NewSectionReader(r, 0, sigStart), io.NewSectionReader(r, l.cdOffset, l.cdSize), bytes.NewReader(eocd)}
	lengths := []int64{sigStart, l.cdSize, int64(len(eocd))}
	var count int64
	for _, n := range lengths {
		count += (n + digestChunk - 1) / digestChunk
	}
	top := h.New()
	var hdr [5]byte
	hdr[0] = 0x5a
	le.PutUint32(hdr[1:], uint32(count)) //nolint:gosec // G115: at most 4096 chunks per GiB
	top.Write(hdr[:])
	buf := make([]byte, digestChunk)
	ch := h.New()
	for i, sec := range sections {
		for off := int64(0); off < lengths[i]; off += digestChunk {
			n := min(digestChunk, lengths[i]-off)
			if k, err := sec.ReadAt(buf[:n], off); int64(k) != n {
				return nil, fmt.Errorf("read: %w", cmpErr(err))
			}
			ch.Reset()
			hdr[0] = 0xa5
			le.PutUint32(hdr[1:], uint32(n)) //nolint:gosec // G115: at most 1 MiB
			ch.Write(hdr[:])
			ch.Write(buf[:n])
			top.Write(ch.Sum(nil))
		}
	}
	return top.Sum(nil), nil
}
