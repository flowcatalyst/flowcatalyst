package jwt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token expired")
	ErrInvalidIssuer    = errors.New("invalid issuer")
	ErrInvalidTokenType = errors.New("invalid token type")
)

// TokenType identifies the type of token
type TokenType string

const (
	TokenTypeUser    TokenType = "USER"
	TokenTypeService TokenType = "SERVICE"
)

// SessionClaims represents claims in a session token (for users)
type SessionClaims struct {
	jwt.RegisteredClaims
	Email        string   `json:"email,omitempty"`
	Type         string   `json:"type"`
	Clients      []string `json:"clients"`
	Groups       []string `json:"groups,omitempty"`
	Applications []string `json:"applications,omitempty"`
}

// AccessClaims represents claims in an access token (for service accounts)
type AccessClaims struct {
	jwt.RegisteredClaims
	ClientID string   `json:"client_id,omitempty"`
	Type     string   `json:"type"`
	Groups   []string `json:"groups,omitempty"`
}

// IDTokenClaims represents claims in an OIDC ID token
type IDTokenClaims struct {
	jwt.RegisteredClaims
	Email   string   `json:"email,omitempty"`
	Name    string   `json:"name,omitempty"`
	Nonce   string   `json:"nonce,omitempty"`
	Clients []string `json:"clients,omitempty"`
}

// TokenService handles JWT token generation and validation
type TokenService struct {
	keyManager           *KeyManager
	issuer               string
	accessTokenExpiry    time.Duration
	sessionTokenExpiry   time.Duration
	refreshTokenExpiry   time.Duration
	authCodeExpiry       time.Duration
}

// TokenServiceConfig holds configuration for the token service
type TokenServiceConfig struct {
	Issuer               string
	AccessTokenExpiry    time.Duration
	SessionTokenExpiry   time.Duration
	RefreshTokenExpiry   time.Duration
	AuthCodeExpiry       time.Duration
}

// NewTokenService creates a new token service
func NewTokenService(keyManager *KeyManager, cfg TokenServiceConfig) *TokenService {
	return &TokenService{
		keyManager:           keyManager,
		issuer:               cfg.Issuer,
		accessTokenExpiry:    cfg.AccessTokenExpiry,
		sessionTokenExpiry:   cfg.SessionTokenExpiry,
		refreshTokenExpiry:   cfg.RefreshTokenExpiry,
		authCodeExpiry:       cfg.AuthCodeExpiry,
	}
}

// IssueSessionToken creates a session token for a user
func (s *TokenService) IssueSessionToken(principalID, email string, roles, clients, applications []string) (string, error) {
	now := time.Now()
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   principalID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.sessionTokenExpiry)),
		},
		Email:        email,
		Type:         string(TokenTypeUser),
		Clients:      clients,
		Groups:       roles,
		Applications: applications,
	}

	return s.signToken(claims)
}

// IssueAccessToken creates an access token for a service account
func (s *TokenService) IssueAccessToken(principalID, clientID string, roles []string) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   principalID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenExpiry)),
		},
		ClientID: clientID,
		Type:     string(TokenTypeService),
		Groups:   roles,
	}

	return s.signToken(claims)
}

// IssueIDToken creates an OIDC ID token
func (s *TokenService) IssueIDToken(principalID, email, name, audience, nonce string, clients []string) (string, error) {
	now := time.Now()
	claims := IDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   principalID,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.sessionTokenExpiry)),
		},
		Email:   email,
		Name:    name,
		Nonce:   nonce,
		Clients: clients,
	}

	return s.signToken(claims)
}

// signToken signs claims with the RSA private key
func (s *TokenService) signToken(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.keyManager.KeyID()

	return token.SignedString(s.keyManager.PrivateKey())
}

// ValidateToken validates a token and returns the claims
func (s *TokenService) ValidateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidToken
		}
		return s.keyManager.PublicKey(), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Verify issuer
	if iss, ok := claims["iss"].(string); !ok || iss != s.issuer {
		return nil, ErrInvalidIssuer
	}

	return claims, nil
}

// ValidateSessionToken validates a session token and returns the principal ID
func (s *TokenService) ValidateSessionToken(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// Check token type
	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != string(TokenTypeUser) {
		return "", ErrInvalidTokenType
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", ErrInvalidToken
	}

	return sub, nil
}

// ValidateAccessToken validates an access token and returns the principal ID
func (s *TokenService) ValidateAccessToken(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", ErrInvalidToken
	}

	return sub, nil
}

// GetSessionClaims extracts session claims from a token
func (s *TokenService) GetSessionClaims(tokenString string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidToken
		}
		return s.keyManager.PublicKey(), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*SessionClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// HashToken creates a SHA-256 hash of a token for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// GenerateRefreshToken generates a random refresh token
func GenerateRefreshToken() (string, error) {
	return generateRandomString(48)
}

// GenerateAuthorizationCode generates a random authorization code
func GenerateAuthorizationCode() (string, error) {
	return generateRandomString(32)
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
