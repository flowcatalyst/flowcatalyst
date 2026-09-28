package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type desiredInput struct {
	Pool        string `query:"pool" required:"true" doc:"The runner pool to fetch desired state for"`
	Wait        int    `query:"wait" doc:"Seconds to hold the request open for a change, capped at control.MaxWait"`
	IfNoneMatch string `header:"If-None-Match"`
}

type desiredOutput struct {
	Status int
	ETag   string `header:"ETag"`
	// CacheControl is always "no-store": the document holds secrets
	// (docs/function-runner-plan.md §8.3) and must never be cached anywhere
	// between the platform and the runner.
	CacheControl string `header:"Cache-Control"`
	Body         *fncontrol.Desired
}

// desired serves GET /control/functions/desired (plan §8.3): a long-poll
// over a pool's desired document.
//
// The ETag is a hash of the RENDERED document, not just the pool revision:
// a rotated service-account secret or a settings change must reach runners
// even when nothing bumped the revision (BumpPoolRevision is only called by
// promote/alias/settings writes, and a service-account secret rotation is
// none of those). Holding the request: re-render at most every
// ReRenderInterval, waking early on NOTIFY fng_desired for this pool via
// s.Listener; answer 200 with the new document as soon as the ETag
// differs, else 304 once wait elapses.
func (s *State) desired(ctx context.Context, in *desiredInput) (*desiredOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	if in.Pool == "" {
		return nil, usecase.Validation("POOL_REQUIRED", "pool is required")
	}

	wait := min(time.Duration(in.Wait)*time.Second, fncontrol.MaxWait)
	deadline := time.Now().Add(wait)
	reRender := s.reRenderInterval()

	for {
		doc, err := s.buildDesired(ctx, in.Pool)
		if err != nil {
			return nil, err
		}
		etag, err := etagOf(doc)
		if err != nil {
			return nil, usecase.Internal("DESIRED_ETAG", "rendering the desired document failed", err)
		}
		if in.IfNoneMatch == "" || in.IfNoneMatch != etag {
			return &desiredOutput{Status: http.StatusOK, ETag: etag, CacheControl: "no-store", Body: doc}, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return &desiredOutput{Status: http.StatusNotModified, ETag: etag, CacheControl: "no-store"}, nil
		}
		wakeIn := min(reRender, remaining)
		var wake <-chan struct{}
		if s.Listener != nil {
			wake = s.Listener.Wait(in.Pool)
		}
		timer := time.NewTimer(wakeIn)
		select {
		case <-ctx.Done(): // caller gone (timeout, disconnect, or server shutdown)
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C: // re-render interval elapsed; try again
		case <-wake: // NOTIFY fng_desired for this pool
			timer.Stop()
		}
	}
}

// etagOf hashes the rendered document.
func etagOf(doc *fncontrol.Desired) (string, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:]) + `"`, nil
}
