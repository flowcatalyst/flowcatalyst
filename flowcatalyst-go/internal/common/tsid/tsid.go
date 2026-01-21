package tsid

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

const (
	// TSID epoch: 2020-01-01T00:00:00Z
	tsidEpoch = 1577836800000

	// Bit lengths
	timestampBits = 42
	randomBits    = 22

	// Crockford Base32 alphabet (lowercase for compatibility)
	alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var (
	generator     *Generator
	generatorOnce sync.Once
)

// Generator generates TSIDs
type Generator struct {
	mu       sync.Mutex
	lastTime int64
	counter  uint32
}

// NewGenerator creates a new TSID generator
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate generates a new TSID as a Crockford Base32 string
func Generate() string {
	generatorOnce.Do(func() {
		generator = NewGenerator()
	})
	return generator.Generate()
}

// Generate generates a new TSID as a Crockford Base32 string
func (g *Generator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Get current timestamp in milliseconds since TSID epoch
	now := time.Now().UnixMilli() - tsidEpoch

	// Generate random component
	var randomBytes [4]byte
	rand.Read(randomBytes[:])
	random := binary.BigEndian.Uint32(randomBytes[:]) & ((1 << randomBits) - 1)

	// If same millisecond, increment counter; otherwise reset
	if now == g.lastTime {
		g.counter++
		// Use counter as part of random to ensure uniqueness
		random = (random &^ 0xFFFF) | (g.counter & 0xFFFF)
	} else {
		g.lastTime = now
		g.counter = 0
	}

	// Combine timestamp and random into 64-bit TSID
	tsid := (uint64(now) << randomBits) | uint64(random)

	// Encode to Crockford Base32 (13 characters)
	return encodeCrockford(tsid)
}

// encodeCrockford encodes a uint64 to a 13-character Crockford Base32 string
func encodeCrockford(value uint64) string {
	// 13 characters for 64 bits (13 * 5 = 65 bits, but we only use 64)
	result := make([]byte, 13)

	for i := 12; i >= 0; i-- {
		result[i] = alphabet[value&0x1F]
		value >>= 5
	}

	return string(result)
}

// decodeCrockford decodes a Crockford Base32 string to a uint64
func decodeCrockford(s string) (uint64, error) {
	var result uint64

	for _, c := range s {
		result <<= 5
		idx := crockfordIndex(byte(c))
		if idx < 0 {
			return 0, ErrInvalidCharacter
		}
		result |= uint64(idx)
	}

	return result, nil
}

// crockfordIndex returns the numeric value of a Crockford Base32 character
func crockfordIndex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'H':
		return int(c - 'A' + 10)
	case c >= 'a' && c <= 'h':
		return int(c - 'a' + 10)
	case c == 'I' || c == 'i' || c == 'L' || c == 'l':
		return 1 // I and L map to 1
	case c >= 'J' && c <= 'K':
		return int(c - 'J' + 18)
	case c >= 'j' && c <= 'k':
		return int(c - 'j' + 18)
	case c >= 'M' && c <= 'N':
		return int(c - 'M' + 20)
	case c >= 'm' && c <= 'n':
		return int(c - 'm' + 20)
	case c == 'O' || c == 'o':
		return 0 // O maps to 0
	case c >= 'P' && c <= 'T':
		return int(c - 'P' + 22)
	case c >= 'p' && c <= 't':
		return int(c - 'p' + 22)
	case c == 'U' || c == 'u':
		return 27 // U is valid in Crockford
	case c >= 'V' && c <= 'Z':
		return int(c - 'V' + 27)
	case c >= 'v' && c <= 'z':
		return int(c - 'v' + 27)
	default:
		return -1
	}
}

// ToLong converts a TSID string to its int64 representation
func ToLong(tsid string) (int64, error) {
	value, err := decodeCrockford(tsid)
	if err != nil {
		return 0, err
	}
	return int64(value), nil
}

// ToString converts an int64 TSID to its Crockford Base32 string representation
func ToString(value int64) string {
	return encodeCrockford(uint64(value))
}

// GetTimestamp extracts the timestamp from a TSID string
func GetTimestamp(tsid string) (time.Time, error) {
	value, err := decodeCrockford(tsid)
	if err != nil {
		return time.Time{}, err
	}

	// Extract timestamp (upper 42 bits)
	timestamp := int64(value >> randomBits)

	// Convert back to Unix milliseconds
	unixMilli := timestamp + tsidEpoch

	return time.UnixMilli(unixMilli), nil
}

// ErrInvalidCharacter is returned when an invalid character is encountered
type ErrInvalidCharacterType struct{}

func (e ErrInvalidCharacterType) Error() string {
	return "invalid character in TSID"
}

var ErrInvalidCharacter = ErrInvalidCharacterType{}
