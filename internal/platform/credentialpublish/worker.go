package credentialpublish

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// ValueSource reads the current plaintext of a credential the platform can
// recover (everything but client-secret, whose plaintext travels in
// pending_value).
type ValueSource interface {
	// Value is the credential's current plaintext; present is false when the
	// owner or the credential no longer exists.
	Value(ctx context.Context, owner Owner, credential string) (value string, present bool, err error)
}

const (
	defaultInterval = 5 * time.Second
	// lease is how long a claimed item is held before another pass may take it
	// again: longer than any AWS call, short enough to recover from a crash.
	lease = 2 * time.Minute
	// maxPerPass bounds one RunOnce, so Run re-checks leadership regularly.
	maxPerPass = 200
)

// Worker writes queued credentials to the store and deletes removed ones
// (spec §6). Run it on the leader only.
type Worker struct {
	Repo     *Repository
	Store    Store
	Values   ValueSource
	Enc      *encryption.Service
	IsLeader func() bool
	Interval time.Duration
	Now      func() time.Time
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Run works until ctx is done, a pass every Interval while this process is the
// leader, and straight on to the next while there is more due.
func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if w.IsLeader == nil || w.IsLeader() {
			if n, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("credential publish: pass failed", "err", err)
			} else if n >= maxPerPass {
				continue
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce claims each due item in turn and processes it, returning how many it
// handled. A failed item is recorded and counts as handled; an error is a
// failure to reach the database.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	n := 0
	for n < maxPerPass && ctx.Err() == nil {
		c, err := w.Repo.claim(ctx, w.now(), lease)
		if err != nil {
			return n, err
		}
		if c == nil {
			break
		}
		n++
		if err := w.process(ctx, c); err != nil {
			return n, err
		}
	}
	if p, err := w.Repo.pending(ctx); err == nil {
		pendingGauge.Set(float64(p))
	}
	return n, nil
}

func (w *Worker) process(ctx context.Context, c *claimed) error {
	log := slog.With("owner", string(c.owner.Type)+"/"+c.owner.ID, "credential", c.credential, "path", c.path)
	tag := TagValue(c.owner)

	if c.state == StatePendingDelete {
		err := w.Store.Delete(ctx, c.path, tag)
		switch {
		case err == nil:
			log.Info("credential publish: deleted")
		case errors.Is(err, ErrNotManaged):
			log.Warn("credential publish: a parameter FlowCatalyst does not manage is left in place")
		default:
			return w.fail(ctx, c, log, err)
		}
		deletesTotal.Inc()
		return w.Repo.recordDeleted(ctx, c)
	}

	value, present, err := w.value(ctx, c)
	if err != nil {
		return w.fail(ctx, c, log, err)
	}
	if !present {
		// The credential went away between the queueing and now.
		return w.Repo.recordGone(ctx, c, w.now())
	}
	if err := w.Store.Put(ctx, c.path, value, tag); err != nil {
		return w.fail(ctx, c, log, err)
	}
	log.Info("credential publish: put")
	putsTotal.Inc()
	return w.Repo.recordPut(ctx, c, w.now())
}

var errUndecryptable = errors.New("the stored value cannot be decrypted")

// value is what to publish: the encrypted pending value when there is one,
// else the owner's current credential. Never an empty one.
func (w *Worker) value(ctx context.Context, c *claimed) (string, bool, error) {
	if c.pendingValue != nil {
		if w.Enc == nil {
			return "", false, errUndecryptable
		}
		v, err := w.Enc.Decrypt(*c.pendingValue)
		if err != nil || v == "" {
			return "", false, errUndecryptable
		}
		return v, true, nil
	}
	v, present, err := w.Values.Value(ctx, c.owner, c.credential)
	if err != nil {
		return "", false, err
	}
	return v, present && v != "", nil
}

// fail records the failure and the next attempt. It returns an error only when
// the record itself could not be written.
func (w *Worker) fail(ctx context.Context, c *claimed, log *slog.Logger, err error) error {
	code := ErrorCode(err)
	switch {
	case errors.Is(err, ErrNotManaged):
		code = "NotManaged"
	case errors.Is(err, errUndecryptable):
		code = "Undecryptable"
	case code == "":
		code = "error"
	}
	text := errorText(err, c.path)
	if errors.Is(err, errUndecryptable) {
		text = errUndecryptable.Error()
	}
	failuresTotal.WithLabelValues(code).Inc()
	log.Warn("credential publish: failed", "attempts", c.attempts+1, "error", text)
	return w.Repo.recordFailure(ctx, c, text, w.now())
}
