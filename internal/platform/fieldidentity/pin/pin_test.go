package pin

import (
	"errors"
	"slices"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name, pin, code string
		min             int
		want            []string
	}{
		{"ok", "482915", "W1", 6, nil},
		{"too short default", "48291", "W1", 0, []string{TooShort}},
		{"four allowed when client lowers it", "4829", "W1", 4, nil},
		{"min below range falls back to default", "4829", "W1", 3, []string{TooShort}},
		{"not digits", "48a915", "W1", 6, []string{NotDigits}},
		{"empty", "", "W1", 6, []string{NotDigits}},
		{"too long", "4829153748291", "W1", 6, []string{TooLong}},
		{"all same", "000000", "W1", 6, []string{AllSame}},
		{"ascending", "123456", "W1", 6, []string{Sequential}},
		{"descending", "654321", "W1", 6, []string{Sequential}},
		{"wrap ascending", "890123", "W1", 6, []string{Sequential}},
		{"wrap descending", "109876", "W1", 6, []string{Sequential}},
		{"common", "121212", "W1", 6, []string{Common}},
		{"common 696969", "696969", "W1", 6, []string{Common}},
		{"worker code", "482915", "482915", 6, []string{IsWorkerCode}},
		{"four-digit common", "2580", "W1", 4, []string{Common}},
	}
	for _, c := range cases {
		got := Check(c.pin, c.code, c.min)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Check(%q) = %v, want %v", c.name, c.pin, got, c.want)
		}
	}
}

func TestGenerateIsPolicyValid(t *testing.T) {
	for _, n := range []int{0, 4, 6, 8, 12} {
		for range 50 {
			p, err := Generate(n, "W1")
			if err != nil {
				t.Fatal(err)
			}
			want := n
			if !ValidMinLength(n) {
				want = DefaultMinLength
			}
			if len(p) != want {
				t.Fatalf("Generate(%d) length %d, want %d", n, len(p), want)
			}
			if v := Check(p, "W1", want); len(v) != 0 {
				t.Fatalf("Generate(%d) = %q violates policy: %v", n, p, v)
			}
		}
	}
}

func TestHashRoundTrip(t *testing.T) {
	h, err := NewHasher("test-app-key")
	if err != nil {
		t.Fatal(err)
	}
	d, err := h.Hash("clt_1", "fwu_1", "482915")
	if err != nil {
		t.Fatal(err)
	}
	if d.PepperID == "" || d.PepperID != h.PepperID() {
		t.Fatalf("digest pepper id %q, hasher's %q", d.PepperID, h.PepperID())
	}
	stored := Stored{Hash: d.Hash, Scheme: SchemePepperV1, PepperID: d.PepperID}
	ok, rehash, err := h.Verify("clt_1", "fwu_1", "482915", stored)
	if err != nil || !ok || rehash {
		t.Fatalf("right pin: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	for name, args := range map[string][3]string{
		"wrong pin":      {"clt_1", "fwu_1", "482916"},
		"other client":   {"clt_2", "fwu_1", "482915"},
		"other identity": {"clt_1", "fwu_2", "482915"},
	} {
		ok, _, err := h.Verify(args[0], args[1], args[2], stored)
		if err != nil || ok {
			t.Errorf("%s: ok=%v err=%v, want mismatch", name, ok, err)
		}
	}
	// A different key (different pepper) cannot verify.
	other, _ := NewHasher("another-key")
	if ok, _, _ := other.Verify("clt_1", "fwu_1", "482915", Stored{Hash: d.Hash, Scheme: SchemePepperV1}); ok {
		t.Error("a hash verified under a different pepper")
	}
	// Two hashes of the same PIN differ (salted).
	h2, _ := h.Hash("clt_1", "fwu_1", "482915")
	if h2.Hash == d.Hash {
		t.Error("hashes are not salted")
	}
}

func TestUnconfiguredFailsClosed(t *testing.T) {
	h, err := NewHasher("  ")
	if !errors.Is(err, ErrUnconfigured) || h != nil {
		t.Fatalf("NewHasher(empty) = %v, %v", h, err)
	}
	if _, err := h.Hash("c", "i", "482915"); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("Hash: %v", err)
	}
	if _, _, err := h.Verify("c", "i", "482915", Stored{Hash: "x", Scheme: SchemePepperV1}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("Verify: %v", err)
	}
	h.Dummy("482915") // must not panic
}

func TestLegacyVerifyAndRehash(t *testing.T) {
	h, _ := NewHasher("test-app-key")
	// An imported unpeppered hash of the raw PIN.
	legacy, err := passwordhash.HashWithParams("482915", Params)
	if err != nil {
		t.Fatal(err)
	}
	ok, rehash, err := h.Verify("clt_1", "fwu_1", "482915", Stored{Hash: legacy, Scheme: SchemeLegacy})
	if err != nil || !ok || !rehash {
		t.Fatalf("legacy right pin: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, _ = h.Verify("clt_1", "fwu_1", "000001", Stored{Hash: legacy, Scheme: SchemeLegacy})
	if ok {
		t.Error("legacy wrong pin verified")
	}
	// The same raw hash must NOT verify under the peppered scheme.
	if ok, _, _ := h.Verify("clt_1", "fwu_1", "482915", Stored{Hash: legacy, Scheme: SchemePepperV1}); ok {
		t.Error("an unpeppered hash verified as PEPPER_V1")
	}
}

// ── key rotation ─────────────────────────────────────────────────────────

// A hash made under a key that has since been rotated out (now listed as
// previous) still verifies, by its pepper id or, stored before ids, by trying
// the previous peppers; either way the caller is asked to re-hash.
func TestRotatedKeyStillVerifiesAndAsksForRehash(t *testing.T) {
	old, _ := NewHasher("old-app-key")
	d, err := old.Hash("clt_1", "fwu_1", "482915")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewHasher("new-app-key", "old-app-key")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.PepperID() == old.PepperID() {
		t.Fatal("a new key must give a new pepper id")
	}

	for name, stored := range map[string]Stored{
		"with the old pepper id":   {Hash: d.Hash, Scheme: SchemePepperV1, PepperID: d.PepperID},
		"stored before ids (none)": {Hash: d.Hash, Scheme: SchemePepperV1},
	} {
		ok, rehash, err := rotated.Verify("clt_1", "fwu_1", "482915", stored)
		if err != nil || !ok || !rehash {
			t.Errorf("%s: right pin ok=%v rehash=%v err=%v; want ok and rehash", name, ok, rehash, err)
		}
		ok, rehash, err = rotated.Verify("clt_1", "fwu_1", "000000", stored)
		if err != nil || ok || rehash {
			t.Errorf("%s: wrong pin ok=%v rehash=%v err=%v; want a plain mismatch", name, ok, rehash, err)
		}
	}

	// The re-hash lands under the current pepper and then needs nothing more.
	nd, _ := rotated.Hash("clt_1", "fwu_1", "482915")
	if nd.PepperID != rotated.PepperID() {
		t.Fatalf("rehash pepper id %q, want current %q", nd.PepperID, rotated.PepperID())
	}
	ok, rehash, err := rotated.Verify("clt_1", "fwu_1", "482915", Stored{Hash: nd.Hash, Scheme: SchemePepperV1, PepperID: nd.PepperID})
	if err != nil || !ok || rehash {
		t.Fatalf("current-pepper hash: ok=%v rehash=%v err=%v; want ok, no rehash", ok, rehash, err)
	}
}

// A hash stored before pepper ids existed, under the current key, verifies and
// is re-hashed once so it gets its id.
func TestHashWithoutIDUnderCurrentKeyGetsAnID(t *testing.T) {
	h, _ := NewHasher("app-key")
	d, _ := h.Hash("clt_1", "fwu_1", "482915")
	ok, rehash, err := h.Verify("clt_1", "fwu_1", "482915", Stored{Hash: d.Hash, Scheme: SchemePepperV1})
	if err != nil || !ok || !rehash {
		t.Fatalf("ok=%v rehash=%v err=%v; want ok and rehash (to stamp the id)", ok, rehash, err)
	}
}

// With a pepper id, Verify uses exactly that pepper: a hash whose id names the
// current pepper is never tried under a previous one, so a wrong PIN costs a
// single argon2id however many previous keys are configured.
func TestPepperIDSelectsExactlyOnePepper(t *testing.T) {
	old, _ := NewHasher("old-app-key")
	d, _ := old.Hash("clt_1", "fwu_1", "482915")
	rotated, _ := NewHasher("new-app-key", "old-app-key")
	// Claim the current pepper for a hash made under the old one: the old
	// pepper must not be tried, so the right PIN does not verify.
	ok, _, err := rotated.Verify("clt_1", "fwu_1", "482915", Stored{Hash: d.Hash, Scheme: SchemePepperV1, PepperID: rotated.PepperID()})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v; a hash labelled with the current pepper id must be checked under that pepper only", ok, err)
	}
}

// A hash whose pepper is no longer configured cannot be checked: an error,
// not a wrong PIN (which would count towards a lockout).
func TestRetiredPepperIsAnErrorNotAMismatch(t *testing.T) {
	old, _ := NewHasher("old-app-key")
	d, _ := old.Hash("clt_1", "fwu_1", "482915")
	current, _ := NewHasher("new-app-key")
	_, _, err := current.Verify("clt_1", "fwu_1", "482915", Stored{Hash: d.Hash, Scheme: SchemePepperV1, PepperID: d.PepperID})
	if !errors.Is(err, ErrPepperRetired) {
		t.Fatalf("err = %v, want ErrPepperRetired", err)
	}
}
