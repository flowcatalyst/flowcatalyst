package oidc

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/auth/jwt"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyUsed   = errors.New("already used")
	ErrExpired       = errors.New("expired")
	ErrRevoked       = errors.New("token revoked")
	ErrDuplicateCode = errors.New("duplicate authorization code")
)

// Repository provides access to OAuth/OIDC data
type Repository struct {
	oauthClients  *mongo.Collection
	authCodes     *mongo.Collection
	refreshTokens *mongo.Collection
	loginStates   *mongo.Collection
}

// NewRepository creates a new OIDC repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		oauthClients:  db.Collection("oauth_clients"),
		authCodes:     db.Collection("authorization_codes"),
		refreshTokens: db.Collection("refresh_tokens"),
		loginStates:   db.Collection("oidc_login_state"),
	}
}

// === OAuth Client operations ===

// FindClientByID finds an OAuth client by its ID
func (r *Repository) FindClientByID(ctx context.Context, id string) (*OAuthClient, error) {
	var client OAuthClient
	err := r.oauthClients.FindOne(ctx, bson.M{"_id": id}).Decode(&client)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &client, nil
}

// FindClientByClientID finds an OAuth client by its client ID (public identifier)
func (r *Repository) FindClientByClientID(ctx context.Context, clientID string) (*OAuthClient, error) {
	var client OAuthClient
	err := r.oauthClients.FindOne(ctx, bson.M{"clientId": clientID}).Decode(&client)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &client, nil
}

// FindAllClients returns all OAuth clients
func (r *Repository) FindAllClients(ctx context.Context) ([]*OAuthClient, error) {
	cursor, err := r.oauthClients.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clients []*OAuthClient
	if err := cursor.All(ctx, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// InsertClient creates a new OAuth client
func (r *Repository) InsertClient(ctx context.Context, client *OAuthClient) error {
	if client.ID == "" {
		client.ID = tsid.Generate()
	}
	now := time.Now()
	client.CreatedAt = now
	client.UpdatedAt = now

	_, err := r.oauthClients.InsertOne(ctx, client)
	return err
}

// UpdateClient updates an existing OAuth client
func (r *Repository) UpdateClient(ctx context.Context, client *OAuthClient) error {
	client.UpdatedAt = time.Now()

	result, err := r.oauthClients.ReplaceOne(ctx, bson.M{"_id": client.ID}, client)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteClient removes an OAuth client
func (r *Repository) DeleteClient(ctx context.Context, id string) error {
	result, err := r.oauthClients.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// === Authorization Code operations ===

// SaveAuthorizationCode stores an authorization code
func (r *Repository) SaveAuthorizationCode(ctx context.Context, code *AuthorizationCode) error {
	code.CreatedAt = time.Now()
	if code.ExpiresAt.IsZero() {
		code.ExpiresAt = code.CreatedAt.Add(10 * time.Minute)
	}

	_, err := r.authCodes.InsertOne(ctx, code)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateCode
	}
	return err
}

// FindAuthorizationCode finds an authorization code
func (r *Repository) FindAuthorizationCode(ctx context.Context, code string) (*AuthorizationCode, error) {
	var authCode AuthorizationCode
	err := r.authCodes.FindOne(ctx, bson.M{"_id": code}).Decode(&authCode)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &authCode, nil
}

// UseAuthorizationCode marks an authorization code as used (single-use)
func (r *Repository) UseAuthorizationCode(ctx context.Context, code string) (*AuthorizationCode, error) {
	var authCode AuthorizationCode
	err := r.authCodes.FindOneAndUpdate(
		ctx,
		bson.M{"_id": code, "used": false},
		bson.M{"$set": bson.M{"used": true}},
	).Decode(&authCode)

	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Check if it exists but was already used
			existing, findErr := r.FindAuthorizationCode(ctx, code)
			if findErr == nil && existing.Used {
				return nil, ErrAlreadyUsed
			}
			return nil, ErrNotFound
		}
		return nil, err
	}

	if authCode.IsExpired() {
		return nil, ErrExpired
	}

	return &authCode, nil
}

// DeleteAuthorizationCode removes an authorization code
func (r *Repository) DeleteAuthorizationCode(ctx context.Context, code string) error {
	_, err := r.authCodes.DeleteOne(ctx, bson.M{"_id": code})
	return err
}

// === Refresh Token operations ===

// SaveRefreshToken stores a refresh token
func (r *Repository) SaveRefreshToken(ctx context.Context, token *RefreshToken) error {
	token.CreatedAt = time.Now()

	_, err := r.refreshTokens.InsertOne(ctx, token)
	return err
}

// FindRefreshToken finds a refresh token by its hash
func (r *Repository) FindRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var token RefreshToken
	err := r.refreshTokens.FindOne(ctx, bson.M{"_id": tokenHash}).Decode(&token)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// FindRefreshTokenByRaw finds a refresh token by the raw token value
func (r *Repository) FindRefreshTokenByRaw(ctx context.Context, rawToken string) (*RefreshToken, error) {
	tokenHash := jwt.HashToken(rawToken)
	return r.FindRefreshToken(ctx, tokenHash)
}

// RevokeRefreshToken revokes a refresh token
func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string, replacedBy string) error {
	now := time.Now()
	result, err := r.refreshTokens.UpdateOne(
		ctx,
		bson.M{"_id": tokenHash, "revoked": false},
		bson.M{"$set": bson.M{
			"revoked":    true,
			"revokedAt":  now,
			"replacedBy": replacedBy,
		}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeTokenFamily revokes all tokens in a token family (for rotation reuse detection)
func (r *Repository) RevokeTokenFamily(ctx context.Context, tokenFamily string) error {
	now := time.Now()
	_, err := r.refreshTokens.UpdateMany(
		ctx,
		bson.M{"tokenFamily": tokenFamily, "revoked": false},
		bson.M{"$set": bson.M{
			"revoked":   true,
			"revokedAt": now,
		}},
	)
	return err
}

// RevokeTokensByPrincipal revokes all refresh tokens for a principal
func (r *Repository) RevokeTokensByPrincipal(ctx context.Context, principalID string) error {
	now := time.Now()
	_, err := r.refreshTokens.UpdateMany(
		ctx,
		bson.M{"principalId": principalID, "revoked": false},
		bson.M{"$set": bson.M{
			"revoked":   true,
			"revokedAt": now,
		}},
	)
	return err
}

// === OIDC Login State operations ===

// SaveLoginState stores OIDC login state
func (r *Repository) SaveLoginState(ctx context.Context, state *OIDCLoginState) error {
	state.CreatedAt = time.Now()
	if state.ExpiresAt.IsZero() {
		state.ExpiresAt = state.CreatedAt.Add(10 * time.Minute)
	}

	_, err := r.loginStates.InsertOne(ctx, state)
	return err
}

// FindLoginState finds OIDC login state by state parameter
func (r *Repository) FindLoginState(ctx context.Context, state string) (*OIDCLoginState, error) {
	var loginState OIDCLoginState
	err := r.loginStates.FindOne(ctx, bson.M{"_id": state}).Decode(&loginState)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &loginState, nil
}

// DeleteLoginState removes OIDC login state (single-use)
func (r *Repository) DeleteLoginState(ctx context.Context, state string) error {
	_, err := r.loginStates.DeleteOne(ctx, bson.M{"_id": state})
	return err
}

// ConsumeLoginState finds and deletes login state in one operation
func (r *Repository) ConsumeLoginState(ctx context.Context, state string) (*OIDCLoginState, error) {
	var loginState OIDCLoginState
	err := r.loginStates.FindOneAndDelete(ctx, bson.M{"_id": state}).Decode(&loginState)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if loginState.IsExpired() {
		return nil, ErrExpired
	}

	return &loginState, nil
}
