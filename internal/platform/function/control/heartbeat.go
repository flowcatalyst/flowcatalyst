package control

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

type heartbeatInput struct {
	Body fncontrol.Heartbeat
}

// pendingVersionUpdate is one version's PUBLISHED→READY or
// PUBLISHED→FAILED transition, applied in heartbeat's single transaction.
type pendingVersionUpdate struct {
	versionID string
	ready     bool
	failure   []byte
}

// heartbeat serves POST /control/functions/heartbeat (plan §6.5, §8.3):
// upserts the runner's report and, for each reported version, applies a
// PUBLISHED→READY (once) or PUBLISHED→FAILED transition.
//
// "Once": a version already READY or FAILED is never touched again here —
// gated on the version's CURRENT status being PUBLISHED before any write.
// In particular a READY (possibly live) version reported FAILED by one
// runner is recorded only in that runner's own report (hb persisted via
// UpsertRunnerHeartbeat, unconditionally, below) — it is never demoted.
//
// A FAILED report is fenced by the version's requeued_at_revision: it counts
// only from a runner whose applied Revision is at least the revision at which
// the version was last re-queued by a publish (see ResetFailedVersionTx).
//
// Every version transition in this heartbeat is applied in one transaction
// together with a single pool-revision bump, so the candidate→READY change
// (or a failure) reaches other runners as soon as this call returns.
func (s *State) heartbeat(ctx context.Context, in *heartbeatInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	hb := in.Body
	if strings.TrimSpace(hb.RunnerID) == "" {
		return nil, usecase.Validation("RUNNER_ID_REQUIRED", "runnerId is required")
	}
	if strings.TrimSpace(hb.Pool) == "" {
		return nil, usecase.Validation("POOL_REQUIRED", "pool is required")
	}

	if c := hb.Capabilities; c != nil {
		if len(c.Runtimes) == 0 {
			return nil, usecase.Validation("CAPABILITIES_INVALID", "capabilities.runtimes must name at least one runtime")
		}
		for _, r := range c.Runtimes {
			if !runtimes.ValidTag(r) {
				return nil, usecase.Validation("CAPABILITIES_INVALID", "capabilities.runtimes: "+strconv.Quote(r)+" is not a valid runtime tag")
			}
		}
	}

	report, err := json.Marshal(hb)
	if err != nil {
		return nil, usecase.Internal("HEARTBEAT_MARSHAL", "marshalling the heartbeat report failed", err)
	}
	if err := s.Repo.UpsertRunnerHeartbeat(ctx, hb.RunnerID, hb.Pool, report); err != nil {
		return nil, usecase.Internal("REPO", "upsert_runner_heartbeat failed", err)
	}

	pending, err := s.pendingVersionUpdates(ctx, hb.Versions)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return &apicommon.Empty{}, nil
	}

	now := time.Now().UTC()
	err = s.Repo.WithPoolTx(ctx, func(tx pgx.Tx) error {
		applied := 0
		for _, p := range pending {
			// The writes are conditional on the row still being PUBLISHED
			// (and, for FAILED, on the runner having applied the version's
			// last re-queue revision): pendingVersionUpdates read the status
			// outside this transaction, and a publish-time reset can land in
			// between. A stale report must never resurrect a FAILED status.
			var ok bool
			var err error
			if p.ready {
				ok, err = s.Repo.SetVersionReadyIfPublishedTx(ctx, tx, p.versionID, now)
			} else {
				ok, err = s.Repo.SetVersionFailureIfPublishedTx(ctx, tx, p.versionID, p.failure, hb.Revision)
			}
			if err != nil {
				return err
			}
			if ok {
				applied++
			}
		}
		if applied == 0 {
			return nil
		}
		_, err := s.Repo.BumpPoolRevisionTx(ctx, tx, hb.Pool)
		return err
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "apply heartbeat version transitions failed", err)
	}
	return &apicommon.Empty{}, nil
}

// pendingVersionUpdates reads each reported version's current status
// (outside the write transaction — a cheap read that decides whether there
// is anything to do) and returns the PUBLISHED-only subset that needs a
// write.
func (s *State) pendingVersionUpdates(ctx context.Context, reports []fncontrol.VersionReport) ([]pendingVersionUpdate, error) {
	var pending []pendingVersionUpdate
	for _, vr := range reports {
		if strings.TrimSpace(vr.FunctionID) == "" {
			continue
		}
		switch vr.State {
		case fncontrol.StateLoaded, fncontrol.StateFailed:
		default:
			continue // COMPILED (and any future state) carries no version transition here
		}
		v, err := s.Repo.GetVersionByNumber(ctx, vr.FunctionID, int32(vr.Number))
		if err != nil {
			return nil, usecase.Internal("REPO", "get_version_by_number failed", err)
		}
		if v == nil || v.Status != function.VersionPublished {
			continue // unknown to this platform, or already past PUBLISHED: never re-touched
		}
		switch vr.State {
		case fncontrol.StateLoaded:
			pending = append(pending, pendingVersionUpdate{versionID: v.ID, ready: true})
		case fncontrol.StateFailed:
			failure, err := json.Marshal(struct {
				Reason string `json:"reason"`
			}{Reason: vr.Reason})
			if err != nil {
				return nil, usecase.Internal("HEARTBEAT_MARSHAL", "marshalling the failure reason failed", err)
			}
			pending = append(pending, pendingVersionUpdate{versionID: v.ID, failure: failure})
		}
	}
	return pending, nil
}
