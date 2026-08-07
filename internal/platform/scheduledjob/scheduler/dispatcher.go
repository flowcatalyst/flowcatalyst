package scheduler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

// Signature headers on delivered firings. Byte-format matches the router's
// webhook signing (internal/router/mediator.go signWebhook) and the SDK's
// WebhookValidator: HMAC-SHA256 over `timestamp + body`, millisecond-precision
// ISO8601 UTC timestamp.
const (
	signatureHeader = "X-FlowCatalyst-Signature"
	timestampHeader = "X-FlowCatalyst-Timestamp"
)

// DeliveryCredsResolver returns the delivery credentials (bearer token +
// HMAC signing secret) for an application's service account (zero-value when
// the application has none). The server wires this to the service-account
// repository with a short TTL cache, so credential rotation takes effect
// without a restart or job re-sync.
type DeliveryCredsResolver func(ctx context.Context, applicationID string) (serviceaccount.OutboundCreds, error)

// signFiring computes the delivery signature. Must byte-match the SDK
// verifier (WebhookValidator: hash_hmac over timestamp . rawBody).
func signFiring(payload []byte, secret string) (sigHex, ts string) {
	ts = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), ts
}

// dispatcher claims QUEUED instances and POSTs the firing webhook, treating
// any 2xx response as delivered. Mirrors the Rust dispatcher's structure but
// deliberately widened from Rust's 202-only contract — many consumer
// endpoints return a plain 200, and there is no meaningful difference between
// 2xx codes for this purpose.
type dispatcher struct {
	cfg       Config
	jobs      *scheduledjob.Repository
	instances *scheduledjob.InstanceRepository
	http      *http.Client
	isLeader  func() bool
	// creds resolves the application's SA delivery credentials; nil (or
	// zero-value resolved creds) delivers bare — the SDK's fail-closed
	// fc-signature middleware then rejects with a descriptive 401 that lands
	// in delivery_error, making the missing pairing diagnosable.
	creds DeliveryCredsResolver
}

// webhookEnvelope is the POST body delivered to a job's target URL. Field
// names are camelCase to match the Rust dispatcher's WebhookEnvelope and the
// SDK runner's ScheduledJobEnvelope (both #[serde(rename_all="camelCase")]) —
// this is an external SDK contract: the receiver deserializes these exact keys
// and rejects (HTTP 400) on a missing required field, so snake_case would fail
// every firing.
type webhookEnvelope struct {
	JobID            string           `json:"jobId"`
	JobCode          string           `json:"jobCode"`
	InstanceID       string           `json:"instanceId"`
	ScheduledFor     *time.Time       `json:"scheduledFor,omitempty"`
	FiredAt          time.Time        `json:"firedAt"`
	TriggerKind      string           `json:"triggerKind"`
	CorrelationID    *string          `json:"correlationId,omitempty"`
	Payload          *json.RawMessage `json:"payload,omitempty"`
	TracksCompletion bool             `json:"tracksCompletion"`
	TimeoutSeconds   *int32           `json:"timeoutSeconds,omitempty"`
	// Concurrent echoes the job definition's `concurrent` attribute so SDK
	// runners can enforce their overlap lock exactly when the definition asks
	// for it (concurrent=false ⇒ lock), instead of needing a separate runner
	// option. Additive — older runners ignore it.
	Concurrent bool `json:"concurrent"`
}

func (d *dispatcher) run(ctx context.Context) {
	t := time.NewTicker(d.cfg.DispatchInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduled-job dispatcher stopped")
			return
		case <-t.C:
			if !d.isLeader() {
				continue
			}
			if err := d.tick(ctx); err != nil {
				slog.Warn("scheduled-job dispatcher tick error", "err", err)
			}
		}
	}
}

func (d *dispatcher) tick(ctx context.Context) error {
	queued := scheduledjob.InstanceStatusQueued
	limit := d.cfg.DispatchBatchSize
	instances, err := d.instances.List(ctx, scheduledjob.InstanceListFilters{
		Status: &queued,
		Limit:  &limit,
	})
	if err != nil {
		return err
	}
	// Batch-load jobs to avoid N+1 lookups within a tick.
	jobCache := make(map[string]*scheduledjob.ScheduledJob)
	for i := range instances {
		inst := &instances[i]
		job, cached := jobCache[inst.ScheduledJobID]
		if !cached {
			job, err = d.jobs.FindByID(ctx, inst.ScheduledJobID)
			if err != nil {
				slog.Warn("scheduled-job dispatcher: load job failed", "job_id", inst.ScheduledJobID, "err", err)
				continue
			}
			jobCache[inst.ScheduledJobID] = job
		}
		if job == nil {
			// Job was deleted while the instance sat QUEUED. Mark the
			// instance terminally DELIVERY_FAILED so it doesn't stay QUEUED
			// forever (mirrors the Rust tick's orphan handling; no FK/CASCADE
			// — instances are firing history).
			slog.Warn("scheduled-job dispatcher: orphan instance (job gone); marking DELIVERY_FAILED",
				"instance_id", inst.ID, "job_id", inst.ScheduledJobID)
			if err := d.instances.MarkDeliveryFailed(ctx, inst.ID, "ScheduledJob no longer exists", true); err != nil {
				slog.Warn("scheduled-job dispatcher: mark_delivery_failed failed", "instance_id", inst.ID, "err", err)
			}
			continue
		}
		d.dispatchOne(ctx, job, inst)
	}
	return nil
}

// dispatchOne marks the instance IN_FLIGHT, POSTs the webhook, and accepts
// any 2xx response as delivered. Mirrors the Rust dispatch_one's structure
// (see the widened-from-202 note on the dispatcher type above).
func (d *dispatcher) dispatchOne(ctx context.Context, job *scheduledjob.ScheduledJob, inst *scheduledjob.ScheduledJobInstance) {
	if err := d.instances.MarkInFlight(ctx, inst.ID); err != nil {
		slog.Warn("scheduled-job dispatcher: mark_in_flight failed", "instance_id", inst.ID, "err", err)
		return
	}
	attemptsAfter := inst.DeliveryAttempts + 1 // MarkInFlight bumped the row

	if job.TargetURL == nil || *job.TargetURL == "" {
		d.handleFailure(ctx, job, inst, attemptsAfter, "No target URL configured for job")
		return
	}

	var payload *json.RawMessage
	if len(job.Payload) > 0 {
		p := job.Payload
		payload = &p
	}
	envelope := webhookEnvelope{
		JobID:            job.ID,
		JobCode:          job.Code,
		InstanceID:       inst.ID,
		ScheduledFor:     inst.ScheduledFor,
		FiredAt:          inst.FiredAt,
		TriggerKind:      string(inst.TriggerKind),
		CorrelationID:    inst.CorrelationID,
		Payload:          payload,
		TracksCompletion: job.TracksCompletion,
		TimeoutSeconds:   job.TimeoutSeconds,
		Concurrent:       job.Concurrent,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		d.handleFailure(ctx, job, inst, attemptsAfter, "marshal webhook envelope: "+err.Error())
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *job.TargetURL, bytes.NewReader(body))
	if err != nil {
		d.handleFailure(ctx, job, inst, attemptsAfter, "build request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if d.creds != nil && job.ApplicationID != nil && *job.ApplicationID != "" {
		creds, serr := d.creds(ctx, *job.ApplicationID)
		switch {
		case serr != nil:
			slog.Warn("scheduled-job dispatcher: delivery-creds lookup failed; delivering unsigned",
				"job_code", job.Code, "application_id", *job.ApplicationID, "err", serr)
		case creds.SigningSecret == "" && creds.BearerToken == "":
			slog.Warn("scheduled-job dispatcher: application SA has no webhook credentials; delivering unsigned",
				"job_code", job.Code, "application_id", *job.ApplicationID)
		default:
			// Same header set as router-mediated webhooks: bearer (static
			// convenience credential) + HMAC signature (the real boundary).
			if creds.BearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+creds.BearerToken)
			}
			if creds.SigningSecret != "" {
				sig, ts := signFiring(body, creds.SigningSecret)
				req.Header.Set(signatureHeader, sig)
				req.Header.Set(timestampHeader, ts)
			}
		}
	}

	resp, err := d.http.Do(req)
	if err != nil {
		d.handleFailure(ctx, job, inst, attemptsAfter, "Network/HTTP error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 { // any 2xx = accepted/ack
		if err := d.instances.MarkDelivered(ctx, inst.ID); err != nil {
			slog.Warn("scheduled-job dispatcher: mark_delivered failed", "instance_id", inst.ID, "err", err)
		}
		return
	}

	// Non-2xx: read up to 500 chars of the body for the error message.
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	d.handleFailure(ctx, job, inst, attemptsAfter,
		fmt.Sprintf("HTTP %d (expected 2xx): %s", resp.StatusCode, string(snippet)))
}

// handleFailure records a failed attempt: terminal (DELIVERY_FAILED) when max
// attempts reached, else back to QUEUED for the next dispatch tick. Mirrors
// the Rust handle_failure.
func (d *dispatcher) handleFailure(ctx context.Context, job *scheduledjob.ScheduledJob, inst *scheduledjob.ScheduledJobInstance, attemptsAfter int32, errMsg string) {
	terminal := attemptsAfter >= job.DeliveryMaxAttempts
	if err := d.instances.MarkDeliveryFailed(ctx, inst.ID, errMsg, terminal); err != nil {
		slog.Warn("scheduled-job dispatcher: mark_delivery_failed failed", "instance_id", inst.ID, "err", err)
		return
	}
	if terminal {
		slog.Warn("scheduled-job delivery exhausted retries",
			"instance_id", inst.ID, "job_id", job.ID, "attempts", attemptsAfter, "err", errMsg)
	}
}
