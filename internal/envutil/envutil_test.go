package envutil

import "testing"

func TestOr(t *testing.T) {
	t.Setenv("ENVUTIL_T_SET", "value")
	t.Setenv("ENVUTIL_T_EMPTY", "")

	if got := Or("ENVUTIL_T_SET", "def"); got != "value" {
		t.Errorf("Or(set) = %q, want value", got)
	}
	if got := Or("ENVUTIL_T_EMPTY", "def"); got != "def" {
		t.Errorf("Or(empty) = %q, want def", got)
	}
	if got := Or("ENVUTIL_T_UNSET", "def"); got != "def" {
		t.Errorf("Or(unset) = %q, want def", got)
	}
}

func TestInt(t *testing.T) {
	t.Setenv("ENVUTIL_T_INT", "42")
	t.Setenv("ENVUTIL_T_NEG", "-7")
	t.Setenv("ENVUTIL_T_GARBAGE", "nope")

	if got := Int("ENVUTIL_T_INT", 1); got != 42 {
		t.Errorf("Int(42) = %d", got)
	}
	if got := Int("ENVUTIL_T_NEG", 1); got != -7 {
		t.Errorf("Int(-7) = %d (negatives parse; callers clamp)", got)
	}
	if got := Int("ENVUTIL_T_GARBAGE", 1); got != 1 {
		t.Errorf("Int(garbage) = %d, want default", got)
	}
	if got := Int("ENVUTIL_T_UNSET", 1); got != 1 {
		t.Errorf("Int(unset) = %d, want default", got)
	}
}

func TestUint32(t *testing.T) {
	t.Setenv("ENVUTIL_T_U32", "300")
	t.Setenv("ENVUTIL_T_U32_NEG", "-1")
	t.Setenv("ENVUTIL_T_U32_BIG", "4294967296") // 2^32, out of range

	if got := Uint32("ENVUTIL_T_U32", 9); got != 300 {
		t.Errorf("Uint32(300) = %d", got)
	}
	if got := Uint32("ENVUTIL_T_U32_NEG", 9); got != 9 {
		t.Errorf("Uint32(-1) = %d, want default", got)
	}
	if got := Uint32("ENVUTIL_T_U32_BIG", 9); got != 9 {
		t.Errorf("Uint32(2^32) = %d, want default", got)
	}
	if got := Uint32("ENVUTIL_T_UNSET", 9); got != 9 {
		t.Errorf("Uint32(unset) = %d, want default", got)
	}
}

func TestUint(t *testing.T) {
	t.Setenv("ENVUTIL_T_UINT", "15")
	t.Setenv("ENVUTIL_T_UINT_BAD", "x")

	if v, ok := Uint("ENVUTIL_T_UINT"); !ok || v != 15 {
		t.Errorf("Uint(15) = (%d, %v)", v, ok)
	}
	if _, ok := Uint("ENVUTIL_T_UINT_BAD"); ok {
		t.Error("Uint(garbage) ok = true, want false")
	}
	if _, ok := Uint("ENVUTIL_T_UNSET"); ok {
		t.Error("Uint(unset) ok = true, want false")
	}
}
