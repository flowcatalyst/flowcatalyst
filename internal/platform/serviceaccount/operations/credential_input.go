package operations

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// validWebhookCredentialInput refuses webhook credentials a caller supplies
// that carry a stored ciphertext ("encrypted:...") as the auth token or the
// signing secret. The repository keeps an "encrypted:" value as it is, so a
// row loaded and saved again is not encrypted twice, and decrypts it when the
// platform signs or authenticates a delivery: a ciphertext copied from a
// database dump and pasted here would be decrypted and sent to the
// subscriber. Create and update (the only operations that take credentials
// from a caller) run it in Validate, so the API and the console are both
// covered; the credentials the platform generates (create-with-credentials,
// regenerate) never pass through it.
func validWebhookCredentialInput(c *serviceaccount.WebhookCredentials) error {
	if c == nil {
		return nil
	}
	if err := encryption.RefuseStoredCiphertext(c.Token); err != nil {
		return err
	}
	return encryption.RefuseStoredCiphertext(c.SigningSecret)
}
