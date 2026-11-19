package fieldidentity

import (
	"crypto/rand"
	"math/big"
)

// Generated worker codes: three letters then WorkerCodeDigits digits
// (KTM482). The letters leave out the look-alikes I, O and L and the vowels A,
// E, U and Y, so a code never spells a word; the digits leave out 0 and 1. The
// code is drawn at random (crypto/rand, as the platform's other generated
// identifiers), so knowing one code does not give its neighbours. A code is
// not a secret: the worker's PIN is.
const (
	WorkerCodeLetters = "BCDFGHJKMNPQRSTVWXZ"
	WorkerCodeDigits  = "23456789"

	// WorkerCodeLetterCount is the fixed number of letters.
	WorkerCodeLetterCount = 3
	// MinWorkerCodeDigits, MaxWorkerCodeDigits and DefaultWorkerCodeDigits
	// bound the field policy's workerCodeDigits.
	MinWorkerCodeDigits     = 3
	MaxWorkerCodeDigits     = 6
	DefaultWorkerCodeDigits = 3

	// MaxWorkerCodeLength is the longest worker code a person may type.
	MaxWorkerCodeLength = 64
	// MaxIdempotencyKeyLength is the longest idempotency key of a creation
	// without a code.
	MaxIdempotencyKeyLength = 100

	// workerCodeDraws is how many codes one creation tries before it reports
	// the client's code space exhausted.
	workerCodeDraws = 10
)

// blockedWorkerCodeLetters are letter triples that read as an offensive
// abbreviation even without vowels; a draw that starts with one is redrawn.
var blockedWorkerCodeLetters = map[string]bool{
	"KKK": true, "FCK": true, "FKN": true, "FFS": true, "WTF": true, "STF": true, "SHT": true,
	"CNT": true, "DCK": true, "CCK": true, "PNS": true, "FGT": true, "NGR": true, "NGG": true,
	"XXX": true, "SXX": true, "KMS": true, "PRN": true,
}

// ValidWorkerCodeDigits reports whether n is an acceptable workerCodeDigits
// policy value.
func ValidWorkerCodeDigits(n int) bool { return n >= MinWorkerCodeDigits && n <= MaxWorkerCodeDigits }

// WorkerCodeSpace is the number of distinct codes [GenerateWorkerCode] can
// draw with digits digits: the unblocked letter triples times the digit
// strings.
func WorkerCodeSpace(digits int) int64 {
	l, d := int64(len(WorkerCodeLetters)), int64(len(WorkerCodeDigits))
	triples := l*l*l - int64(len(blockedWorkerCodeLetters))
	n := triples
	for range digits {
		n *= d
	}
	return n
}

// GenerateWorkerCode draws a random worker code with digits digits (outside
// the allowed range the default is used). It is upper case, so it is its own
// normalised form.
func GenerateWorkerCode(digits int) (string, error) {
	if !ValidWorkerCodeDigits(digits) {
		digits = DefaultWorkerCodeDigits
	}
	for {
		b := make([]byte, 0, WorkerCodeLetterCount+digits)
		var err error
		if b, err = appendRandom(b, WorkerCodeLetters, WorkerCodeLetterCount); err != nil {
			return "", err
		}
		if blockedWorkerCodeLetters[string(b)] {
			continue
		}
		if b, err = appendRandom(b, WorkerCodeDigits, digits); err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func appendRandom(b []byte, alphabet string, n int) ([]byte, error) {
	size := big.NewInt(int64(len(alphabet)))
	for range n {
		d, err := rand.Int(rand.Reader, size)
		if err != nil {
			return nil, err
		}
		b = append(b, alphabet[d.Int64()])
	}
	return b, nil
}
