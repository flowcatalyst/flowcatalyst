package pushgw

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/push"
)

// NotifyPrincipalEnd tells every gateway task to end the push streams of a platform principal
// (a sign-out, push contract 7, spec 5). It can be called from any platform process: it is a
// NOTIFY on the platform database, which the gateway tasks turn into Pushes.EndPrincipal, so the
// revoking process needs neither the gateway nor the push log's driver. Field login families
// need no call: the trigger of migration 105 notifies when their last session ends.
func NotifyPrincipalEnd(ctx context.Context, pool *pgxpool.Pool, principal string) error {
	if principal == "" {
		return nil
	}
	if _, err := pool.Exec(ctx, `SELECT pg_notify($1, $2)`, ChannelPrincipalEnd, principal); err != nil {
		return fmt.Errorf("push gateway: notify the end of a principal's streams: %w", err)
	}
	return nil
}

// NotifyConfigChanged tells every gateway task to reload its caches. A sync or a settings edit
// calls it after its transaction commits.
func NotifyConfigChanged(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `SELECT pg_notify($1, '')`, ChannelConfig)
	return err
}

// runEnds ends streams on the NOTIFYs of ChannelSessionEnd and ChannelPrincipalEnd until ctx
// ends. EndStreams ends this task's streams and publishes the end through the push log (A1
// pg_notify('fw_push_end'), A2 pub/sub), so every task's streams of the holder end whichever
// task received the platform's notification. Each task receives it, so a holder is published
// once per task; ending an already ended stream is a no-op.
func runEnds(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, pushes *push.Pushes) {
	end := func(h push.Holder) {
		if err := pushes.EndStreams(ctx, h); err != nil && ctx.Err() == nil {
			log.Warn("push gateway: ending streams failed", "holder", h, "err", err)
		}
	}
	listenLoop(ctx, log, "ends", pool, []string{ChannelSessionEnd, ChannelPrincipalEnd},
		func(channel, payload string) {
			if payload == "" {
				return
			}
			switch channel {
			case ChannelSessionEnd:
				end(push.Holder{Session: payload})
			case ChannelPrincipalEnd:
				end(push.Holder{Principal: payload})
			}
		}, nil)
}
