package fieldidentity

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestWorkerCodeAlphabets(t *testing.T) {
	t.Parallel()
	// No look-alikes (I, O, L; 0, 1) and no vowels (A, E, I, O, U, Y).
	for _, c := range "AEIOUYL" {
		assert.NotContains(t, WorkerCodeLetters, string(c))
	}
	assert.NotContains(t, WorkerCodeDigits, "0")
	assert.NotContains(t, WorkerCodeDigits, "1")
	assert.Len(t, WorkerCodeLetters, 19)
	assert.Len(t, WorkerCodeDigits, 8)
	for triple := range blockedWorkerCodeLetters {
		require.Len(t, triple, 3)
		for _, c := range triple {
			assert.Contains(t, WorkerCodeLetters, string(c), "blocked triple %s must be drawable to count", triple)
		}
	}
}

func TestGenerateWorkerCodeFormat(t *testing.T) {
	t.Parallel()
	for digits := MinWorkerCodeDigits; digits <= MaxWorkerCodeDigits; digits++ {
		re := regexp.MustCompile(`^[` + WorkerCodeLetters + `]{3}[` + WorkerCodeDigits + `]{` + strconv.Itoa(digits) + `}$`)
		for range 2000 {
			code, err := GenerateWorkerCode(digits)
			require.NoError(t, err)
			require.Regexp(t, re, code)
			assert.Equal(t, code, NormalizeWorkerCode(code), "a generated code is its own normalised form")
			assert.LessOrEqual(t, len(code), MaxWorkerCodeLength)
			assert.False(t, blockedWorkerCodeLetters[code[:3]], "blocked triple %s drawn", code[:3])
		}
	}
}

func TestGenerateWorkerCodeOutOfRangeUsesDefault(t *testing.T) {
	t.Parallel()
	for _, digits := range []int{0, 2, 7, -1} {
		code, err := GenerateWorkerCode(digits)
		require.NoError(t, err)
		assert.Len(t, code, 3+DefaultWorkerCodeDigits)
	}
}

func TestGenerateWorkerCodeUsesWholeAlphabet(t *testing.T) {
	t.Parallel()
	letters, digits := map[byte]bool{}, map[byte]bool{}
	for range 5000 {
		code, err := GenerateWorkerCode(3)
		require.NoError(t, err)
		for i := range 3 {
			letters[code[i]] = true
		}
		for i := 3; i < len(code); i++ {
			digits[code[i]] = true
		}
	}
	assert.Len(t, letters, len(WorkerCodeLetters))
	assert.Len(t, digits, len(WorkerCodeDigits))
}

func TestWorkerCodeSpace(t *testing.T) {
	t.Parallel()
	triples := int64(19*19*19 - len(blockedWorkerCodeLetters))
	assert.Equal(t, triples*512, WorkerCodeSpace(3))
	assert.Equal(t, triples*4096, WorkerCodeSpace(4))
	assert.Equal(t, triples*32768, WorkerCodeSpace(5))
	assert.Equal(t, triples*262144, WorkerCodeSpace(6))
}

// A generated code always holds letters, so no PIN (digits only) can equal it
// and the PIN policy's worker-code rule never refuses a PIN because of it.
func TestGeneratedCodePassesThePINPolicy(t *testing.T) {
	t.Parallel()
	for range 200 {
		code, err := GenerateWorkerCode(6)
		require.NoError(t, err)
		digitsOnly := code[3:]
		assert.NotContains(t, pin.Check(digitsOnly, code, 6), pin.IsWorkerCode)
		p, err := pin.Generate(6, code)
		require.NoError(t, err)
		assert.Empty(t, pin.Check(p, code, 6))
		assert.Contains(t, pin.Check(code, code, 6), pin.NotDigits, "a code is never a valid PIN")
	}
}

func TestPolicyWorkerCodeDigits(t *testing.T) {
	t.Parallel()
	p, err := PutPolicyCommand{ClientID: "clt_x"}.Build()
	require.NoError(t, err)
	assert.Equal(t, DefaultWorkerCodeDigits, p.WorkerCodeDigits, "omitted takes the default")
	assert.Equal(t, 3, DefaultPolicy("clt_x").WorkerCodeDigits)
	for n := 3; n <= 6; n++ {
		p, err := PutPolicyCommand{ClientID: "clt_x", WorkerCodeDigits: &n}.Build()
		require.NoError(t, err)
		assert.Equal(t, n, p.WorkerCodeDigits)
	}
	for _, n := range []int{0, 2, 7, 64} {
		_, err := PutPolicyCommand{ClientID: "clt_x", WorkerCodeDigits: &n}.Build()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "digits %d", n)
		assert.Equal(t, "POLICY_INVALID", ue.Code)
		assert.Equal(t, "workerCodeDigits", ue.Details["field"])
		assert.True(t, strings.Contains(ue.Message, "between 3 and 6"))
	}
}

func TestEnsureWorkerValidation(t *testing.T) {
	t.Parallel()
	op := (&Ops{}).ensureWorker()
	base := EnsureWorkerCommand{ClientID: "clt_x", Name: "Ann"}
	assert.NoError(t, op.Validate(t.Context(), base), "no code is valid: the platform generates one")
	blank := base
	blank.WorkerCode = "   "
	assert.NoError(t, op.Validate(t.Context(), blank), "a blank code is no code")

	both := base
	both.WorkerCode, both.IdempotencyKey = "W-1", "k-1"
	assert.Equal(t, "IDEMPOTENCY_KEY_WITH_WORKER_CODE", usecase.AsError(op.Validate(t.Context(), both)).Code)

	long := base
	long.IdempotencyKey = strings.Repeat("k", MaxIdempotencyKeyLength+1)
	assert.Equal(t, "INVALID_IDEMPOTENCY_KEY", usecase.AsError(op.Validate(t.Context(), long)).Code)
	long.IdempotencyKey = strings.Repeat("k", MaxIdempotencyKeyLength)
	assert.NoError(t, op.Validate(t.Context(), long))

	tooLong := base
	tooLong.WorkerCode = strings.Repeat("W", MaxWorkerCodeLength+1)
	assert.Equal(t, "WORKER_CODE_INVALID", usecase.AsError(op.Validate(t.Context(), tooLong)).Code)
}
