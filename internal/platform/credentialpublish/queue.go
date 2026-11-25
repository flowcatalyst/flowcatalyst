package credentialpublish

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The item states.
const (
	StatePendingPut     = "PENDING_PUT"
	StatePublished      = "PUBLISHED"
	StatePendingDelete  = "PENDING_DELETE"
	StateNotPublishable = "NOT_PUBLISHABLE"
	CodeAlreadyEnabled  = "CREDENTIAL_PUBLISHING_ALREADY_ENABLED"
	CodeNotEnabled      = "CREDENTIAL_PUBLISHING_NOT_ENABLED"
	CodeRemovalPending  = "CREDENTIAL_PUBLISHING_REMOVAL_PENDING"
)

// Change is one credential's change, carried by the aggregate to its
// repository's Persist (spec §5 "Same transaction").
type Change struct {
	Credential string
	// Remove: the credential no longer exists; its parameter is deleted.
	Remove bool
	// Plaintext is the value to publish, only for client-secret (the platform
	// cannot read that one back): stored encrypted in pending_value until the
	// put. Every other credential is read from its owner when it is published,
	// so two quick regenerates publish only the latest.
	Plaintext *string
	// Unavailable: the client secret is not known (publishing switched on for
	// an existing owner): recorded as NOT_PUBLISHABLE so the page can say so.
	Unavailable bool
}

// EnableRequest switches publishing on for the owner.
type EnableRequest struct {
	BasePath, EnabledBy string
	// Store is the store, "aws-ps" when empty.
	Store string
}

// Pending is what an aggregate's Persist hands to QueueInTx.
type Pending struct {
	Enable  *EnableRequest
	Changes []Change
}

// Empty reports that there is nothing to queue.
func (p Pending) Empty() bool { return p.Enable == nil && len(p.Changes) == 0 }

// QueueInTx writes the publishing work of one change into its transaction, so
// a credential is never changed without its publish being queued, and a
// publish is never queued for a change that rolled back. It talks to the
// database only; the worker talks to AWS.
//
// Enable first (REMOVAL_PENDING while an earlier publication is still being
// deleted, ALREADY_ENABLED when there is one). An OAuth client linked to a
// service account (the client made with the account) publishes with the
// account, so its changes go to the account's row. An owner without a
// publication ignores the changes.
func QueueInTx(ctx context.Context, tx pgx.Tx, enc *encryption.Service, owner Owner, p Pending) error {
	if p.Enable != nil {
		if err := enableInTx(ctx, tx, owner, *p.Enable); err != nil {
			return err
		}
	}
	if len(p.Changes) == 0 {
		return nil
	}
	target, err := publishingOwner(ctx, tx, owner)
	if err != nil {
		return err
	}
	var base string
	err = tx.QueryRow(ctx,
		`SELECT base_path FROM iam_credential_publications WHERE owner_type = $1 AND owner_id = $2`,
		string(target.Type), target.ID).Scan(&base)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("credential publication: %w", err)
	}
	for _, c := range p.Changes {
		if err := queueChange(ctx, tx, enc, target, base, c); err != nil {
			return err
		}
	}
	return nil
}

func enableInTx(ctx context.Context, tx pgx.Tx, owner Owner, req EnableRequest) error {
	var removing bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM iam_credential_publication_items
		                 WHERE owner_type = $1 AND owner_id = $2 AND state = $3)`,
		string(owner.Type), owner.ID, StatePendingDelete).Scan(&removing); err != nil {
		return fmt.Errorf("credential publication: %w", err)
	}
	if removing {
		return usecase.BusinessRule(CodeRemovalPending,
			"the earlier published credentials are still being removed; try again in a moment")
	}
	store := req.Store
	if store == "" {
		store = StoreAWSParameterStore
	}
	var by *string
	if req.EnabledBy != "" {
		by = &req.EnabledBy
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO iam_credential_publications (owner_type, owner_id, store, base_path, enabled_at, enabled_by)
		 VALUES ($1, $2, $3, $4, now(), $5)
		 ON CONFLICT (owner_type, owner_id) DO NOTHING`,
		string(owner.Type), owner.ID, store, req.BasePath, by)
	if err != nil {
		return fmt.Errorf("credential publication: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.Conflict(CodeAlreadyEnabled, "credential publishing is already switched on")
	}
	return nil
}

// publishingOwner is the owner whose publication a change belongs to.
func publishingOwner(ctx context.Context, q pgx.Tx, owner Owner) (Owner, error) {
	if owner.Type != OwnerOAuthClient {
		return owner, nil
	}
	sa, err := linkedServiceAccount(ctx, q, owner.ID)
	if err != nil {
		return Owner{}, err
	}
	if sa == "" {
		return owner, nil
	}
	return Owner{Type: OwnerServiceAccount, ID: sa}, nil
}

// querier is what the read helpers need, a pool or a transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// linkedServiceAccount is the service account an OAuth client is the client
// of: through the client's principal (oauth_clients.service_account_principal_id),
// which carries the account (iam_principals.service_account_id). "" when it is
// stand-alone.
func linkedServiceAccount(ctx context.Context, q querier, oauthClientID string) (string, error) {
	var sa *string
	err := q.QueryRow(ctx,
		`SELECT p.service_account_id
		   FROM oauth_clients oc
		   JOIN iam_principals p ON p.id = oc.service_account_principal_id
		  WHERE oc.id = $1`, oauthClientID).Scan(&sa)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && sa == nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("credential publication: %w", err)
	}
	return *sa, nil
}

func queueChange(ctx context.Context, tx pgx.Tx, enc *encryption.Service, owner Owner, base string, c Change) error {
	if !validCredential(c.Credential) {
		return fmt.Errorf("credential publication: unknown credential %q", c.Credential)
	}
	path := base + c.Credential
	switch {
	case c.Remove:
		// Never published (a client secret that was not known) or never queued:
		// nothing exists at the store. Anything else may, so it is deleted
		// (the delete tolerates a parameter that is not there).
		if _, err := tx.Exec(ctx,
			`DELETE FROM iam_credential_publication_items
			  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3 AND state = $4`,
			string(owner.Type), owner.ID, c.Credential, StateNotPublishable); err != nil {
			return fmt.Errorf("credential publication: %w", err)
		}
		_, err := tx.Exec(ctx, markDeleteSQL+` AND credential = $3`,
			string(owner.Type), owner.ID, c.Credential)
		return wrap(err)
	case c.Unavailable:
		// A secret already published, or on its way, is not replaced by "not known".
		_, err := tx.Exec(ctx,
			`INSERT INTO iam_credential_publication_items
			        (owner_type, owner_id, credential, path, state, updated_at)
			 VALUES ($1, $2, $3, $4, $5, now())
			 ON CONFLICT (owner_type, owner_id, credential) DO NOTHING`,
			string(owner.Type), owner.ID, c.Credential, path, StateNotPublishable)
		return wrap(err)
	}
	var pending *string
	if c.Plaintext != nil {
		if c.Credential != CredClientSecret {
			return fmt.Errorf("credential publication: only %s is carried with its value", CredClientSecret)
		}
		if enc == nil {
			return errors.New("credential publication: no encryption key is configured to hold the client secret")
		}
		ct, err := enc.Encrypt(*c.Plaintext)
		if err != nil {
			return fmt.Errorf("credential publication: %w", err)
		}
		pending = &ct
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO iam_credential_publication_items
		        (owner_type, owner_id, credential, path, state, pending_value,
		         attempts, next_attempt_at, last_error, generation, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 0, now(), NULL, 1, now())
		 ON CONFLICT (owner_type, owner_id, credential) DO UPDATE
		    SET path = EXCLUDED.path, state = EXCLUDED.state, pending_value = EXCLUDED.pending_value,
		        attempts = 0, next_attempt_at = now(), last_error = NULL,
		        generation = iam_credential_publication_items.generation + 1, updated_at = now()`,
		string(owner.Type), owner.ID, c.Credential, path, StatePendingPut, pending)
	return wrap(err)
}

// markDeleteSQL turns an owner's items, except the ones that never existed at
// the store, into deletions that are due now. generation moves so a publish in
// flight does not record its outcome over it.
const markDeleteSQL = `UPDATE iam_credential_publication_items
	SET state = 'PENDING_DELETE', pending_value = NULL, attempts = 0,
	    next_attempt_at = now(), last_error = NULL,
	    generation = generation + 1, updated_at = now()
	WHERE owner_type = $1 AND owner_id = $2 AND state <> 'NOT_PUBLISHABLE'`

func wrap(err error) error {
	if err != nil {
		return fmt.Errorf("credential publication: %w", err)
	}
	return nil
}

// DisableInTx switches publishing off: the publication goes, and every
// published or pending parameter is queued for deletion (the worker removes
// the rows once the parameters are gone). CREDENTIAL_PUBLISHING_NOT_ENABLED
// when it was not on.
func DisableInTx(ctx context.Context, tx pgx.Tx, owner Owner) error {
	tag, err := tx.Exec(ctx,
		`DELETE FROM iam_credential_publications WHERE owner_type = $1 AND owner_id = $2`,
		string(owner.Type), owner.ID)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.BusinessRule(CodeNotEnabled, "credential publishing is not switched on")
	}
	return removeAllInTx(ctx, tx, owner)
}

// OwnerDeletedInTx is DisableInTx for an owner being deleted: the same,
// without an error when publishing was never on. An OAuth client linked to a
// service account does nothing: its credentials are the account's, and the
// account's own delete removes them.
func OwnerDeletedInTx(ctx context.Context, tx pgx.Tx, owner Owner) error {
	if owner.Type == OwnerOAuthClient {
		sa, err := linkedServiceAccount(ctx, tx, owner.ID)
		if err != nil {
			return err
		}
		if sa != "" {
			return nil
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM iam_credential_publications WHERE owner_type = $1 AND owner_id = $2`,
		string(owner.Type), owner.ID); err != nil {
		return wrap(err)
	}
	return removeAllInTx(ctx, tx, owner)
}

func removeAllInTx(ctx context.Context, tx pgx.Tx, owner Owner) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM iam_credential_publication_items
		  WHERE owner_type = $1 AND owner_id = $2 AND state = $3`,
		string(owner.Type), owner.ID, StateNotPublishable); err != nil {
		return wrap(err)
	}
	_, err := tx.Exec(ctx, markDeleteSQL, string(owner.Type), owner.ID)
	return wrap(err)
}
