package local

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidPassword  = errors.New("invalid password")
	ErrPasswordMismatch = errors.New("password mismatch")
	ErrPasswordTooWeak  = errors.New("password does not meet requirements")
)

const (
	// DefaultBcryptCost is the default bcrypt cost factor
	DefaultBcryptCost = 10

	// MinPasswordLength is the minimum password length
	MinPasswordLength = 8
)

// PasswordService handles password hashing and validation
type PasswordService struct {
	bcryptCost int
}

// NewPasswordService creates a new password service
func NewPasswordService() *PasswordService {
	return &PasswordService{
		bcryptCost: DefaultBcryptCost,
	}
}

// NewPasswordServiceWithCost creates a password service with a custom bcrypt cost
func NewPasswordServiceWithCost(cost int) *PasswordService {
	if cost < bcrypt.MinCost {
		cost = DefaultBcryptCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	return &PasswordService{
		bcryptCost: cost,
	}
}

// HashPassword hashes a password using bcrypt
// For passwords longer than 72 bytes (bcrypt's limit), we pre-hash with SHA-256
func (s *PasswordService) HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrInvalidPassword
	}

	// Pre-hash long passwords to handle bcrypt's 72-byte limit
	passwordBytes := s.preparePassword(password)

	hash, err := bcrypt.GenerateFromPassword(passwordBytes, s.bcryptCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// preparePassword handles passwords longer than bcrypt's 72-byte limit
// by pre-hashing them with SHA-256
func (s *PasswordService) preparePassword(password string) []byte {
	passwordBytes := []byte(password)
	if len(passwordBytes) <= 72 {
		return passwordBytes
	}
	// Pre-hash with SHA-256 and encode as base64 (44 bytes, fits in 72)
	hash := sha256.Sum256(passwordBytes)
	encoded := base64.StdEncoding.EncodeToString(hash[:])
	return []byte(encoded)
}

// VerifyPassword verifies a password against a hash
func (s *PasswordService) VerifyPassword(password, hash string) error {
	if password == "" || hash == "" {
		return ErrPasswordMismatch
	}

	// Use the same preparation for long passwords
	passwordBytes := s.preparePassword(password)

	err := bcrypt.CompareHashAndPassword([]byte(hash), passwordBytes)
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordMismatch
		}
		return err
	}

	return nil
}

// ValidatePasswordStrength checks if a password meets strength requirements
func (s *PasswordService) ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooWeak
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	// Require at least 3 of 4 character classes
	count := 0
	if hasUpper {
		count++
	}
	if hasLower {
		count++
	}
	if hasNumber {
		count++
	}
	if hasSpecial {
		count++
	}

	if count < 3 {
		return ErrPasswordTooWeak
	}

	return nil
}

// ExtractEmailDomain extracts the domain from an email address
func ExtractEmailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

// NormalizeEmail normalizes an email address to lowercase
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
