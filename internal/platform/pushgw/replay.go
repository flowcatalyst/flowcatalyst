package pushgw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// replayMargin is added to a proof's acceptance window when its jti is remembered, so a proof
// is never forgotten while a gateway task with a slightly fast clock would still accept it.
const replayMargin = time.Minute

// ReplayGuard remembers the DPoP proofs a gateway has accepted so that a proof presented twice is
// refused (RFC 9449 section 11.1; spec section 5). The field plane accepts a proof whose iat is
// within a day of the server's clock, because devices keep bad clocks, and defends its own
// endpoints with single-use nonces; the gateway's subscribe has no nonce, so the proof's jti is
// the one-time value.
type ReplayGuard interface {
	// Seen records the proof (the key thumbprint and the jti it carries) until the given instant
	// and reports whether it had already been recorded and was still remembered. now is the
	// caller's clock.
	Seen(ctx context.Context, jkt, jti string, until, now time.Time) (replayed bool, err error)
}

// PGReplayGuard is a [ReplayGuard] on the platform database (push_dpop_proofs), so the memory is
// shared by every gateway task: a proof used on one task is refused on the others. A row lives for
// the proof's acceptance window (a day and a minute), so the table holds one row per subscribe of
// that period, about 100 bytes each.
type PGReplayGuard struct{ Pool *pgxpool.Pool }

var _ ReplayGuard = PGReplayGuard{}

// Seen implements ReplayGuard with one statement. A row past its expiry that has not been pruned
// yet is taken over, not counted as a replay.
func (g PGReplayGuard) Seen(ctx context.Context, jkt, jti string, until, now time.Time) (bool, error) {
	sum := sha256.Sum256([]byte(jkt + "\x00" + jti))
	tag, err := g.Pool.Exec(ctx, `
		INSERT INTO push_dpop_proofs (proof_hash, expires_at) VALUES ($1, $2)
		ON CONFLICT (proof_hash) DO UPDATE SET expires_at = EXCLUDED.expires_at
		 WHERE push_dpop_proofs.expires_at <= $3`,
		hex.EncodeToString(sum[:]), until, now)
	if err != nil {
		return false, fmt.Errorf("record the DPoP proof: %w", err)
	}
	return tag.RowsAffected() == 0, nil
}

// Prune deletes the proofs past their window and returns how many.
func (g PGReplayGuard) Prune(ctx context.Context, now time.Time) (int64, error) {
	tag, err := g.Pool.Exec(ctx, `DELETE FROM push_dpop_proofs WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
