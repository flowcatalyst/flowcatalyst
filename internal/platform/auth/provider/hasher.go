package provider

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
)

// Argon2idHasher implements fosite.Hasher against the shared PHC-encoded
// argon2id scheme (internal/platform/auth/passwordhash). The same scheme
// is used by `principal/operations/create.go` (user passwords) and
// `operations/oauth_client.go::generateSecret` (client secrets). We
// hand this to fosite via Config.ClientSecretsHasher so client-secret
// verification uses our scheme instead of fosite's default bcrypt.
type Argon2idHasher struct{}

// Hash returns a PHC-encoded argon2id string of `data`. Stored as-is on
// the client row (in `oauth_clients.client_secret_ref`).
func (Argon2idHasher) Hash(_ context.Context, data []byte) ([]byte, error) {
	encoded, err := passwordhash.Hash(string(data))
	if err != nil {
		return nil, err
	}
	return []byte(encoded), nil
}

// Compare verifies `data` against a stored PHC envelope. Returns nil on
// match, an error otherwise.
func (Argon2idHasher) Compare(_ context.Context, hash, data []byte) error {
	err := passwordhash.Verify(string(data), string(hash))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, passwordhash.ErrMismatch):
		return errors.New("hash mismatch")
	default:
		return err
	}
}
