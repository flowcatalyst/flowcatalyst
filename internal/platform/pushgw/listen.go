package pushgw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channels the gateway uses on the platform database, besides the framework's own
// (fw_push_log, fw_push_end).
const (
	// ChannelConfig is NOTIFYed when an application's rules, ticket keys or limits change
	// (spec 8, 10); every gateway task reloads its caches at once. A 60 s poll is the fallback.
	ChannelConfig = "fw_push_config"
	// ChannelSessionEnd carries a field login family id whose last session ended (the trigger of
	// migration 105): the streams of that family end on every gateway task.
	ChannelSessionEnd = "fc_push_session_end"
	// ChannelPrincipalEnd carries a platform principal id that signed out (NotifyPrincipalEnd):
	// that principal's streams end on every gateway task.
	ChannelPrincipalEnd = "fc_push_principal_end"
)

// listenOnce holds a connection of its own (taken out of the pool) LISTENing on channels, calls
// ready once the LISTENs are in place and note for every notification, until ctx ends (nil) or
// the connection fails (the error).
func listenOnce(ctx context.Context, pool *pgxpool.Pool, channels []string, note func(channel, payload string), ready func()) error {
	pooled, err := pool.Acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("take a connection to listen: %w", err)
	}
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	for _, ch := range channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{ch}.Sanitize()); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("listen on %s: %w", ch, err)
		}
	}
	if ready != nil {
		ready()
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("wait for a notification: %w", err)
		}
		note(n.Channel, n.Payload)
	}
}

// listenLoop is listenOnce with reconnection (a pause of one second, doubling to thirty) until ctx
// ends. ready runs each time the LISTENs are (re)established, so a caller can catch up on what it
// missed meanwhile.
func listenLoop(ctx context.Context, log *slog.Logger, name string, pool *pgxpool.Pool, channels []string, note func(channel, payload string), ready func()) {
	pause := time.Second
	for ctx.Err() == nil {
		err := listenOnce(ctx, pool, channels, note, func() {
			pause = time.Second
			if ready != nil {
				ready()
			}
		})
		if ctx.Err() != nil {
			return
		}
		log.Warn("push gateway: listener lost; reconnecting", "listener", name, "err", err, "in", pause)
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
		pause = min(pause*2, 30*time.Second)
	}
}
