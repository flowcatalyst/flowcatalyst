package runtimeapi

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	CreateJobsOperationID = "ingestionRuntimeCreateJobs"
	GetSecretOperationID  = "ingestionRuntimeGetSecret"
)

type createJobsInput struct {
	ID        string `path:"id"`
	RuntimeID string `query:"runtimeId" required:"true" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The sending runtime, which must carry the ingestion's kind and for a polled source hold its lease"`
	Body      CreateJobsRequest
}

// requireRuntimeIngestion is what both calls need beyond the runtime
// permission: the ingestion is the caller's to reach, is live, and the
// runtime carries its kind. withLease also checks, for a polled source, that
// the runtime holds the lease (ingestion.CheckRuntime) and that the client is
// active: the secrets call; the jobs call checks both again inside the
// transaction that makes the jobs, where they cannot change underneath it.
// On a refusal the ingestion is still returned once it was found (the
// secrets call records whose fetch was refused); a caller checks err first.
func (s *State) requireRuntimeIngestion(ctx context.Context, id, runtimeID string, withLease bool) (*ingestion.Ingestion, error) {
	i, err := s.requireIngestion(ctx, id)
	if err != nil {
		return i, err
	}
	if !i.Live() {
		return i, usecase.Conflict("INGESTION_NOT_LIVE", "ingestion "+id+" is paused or has no live version")
	}
	if withLease {
		err = s.Repo.CheckRuntime(ctx, i, runtimeID, s.now())
	} else {
		err = s.Repo.CheckRuntimeKind(ctx, i, runtimeID)
	}
	if err != nil {
		if usecase.AsError(err) != nil {
			return i, err
		}
		return i, usecase.Internal("REPO", "runtime check failed", err)
	}
	if withLease {
		c, err := s.Resolver.Client(ctx, string(i.ClientID))
		if err != nil {
			return i, err
		}
		if err := ingestion.ClientActive(string(c.Status), c.ID); err != nil {
			return i, err
		}
	}
	return i, nil
}

// createJobs makes a dispatch job of each record, from the ingestion's own
// target. The runtime supplies the records; the job's code, URL, pool,
// connection, signer, mode and client are the ingestion's, resolved as at
// save, so a runtime can neither choose nor alter them.
func (s *State) createJobs(ctx context.Context, in *createJobsInput) (*apicommon.Out[CreateJobsResponse], error) {
	i, err := s.requireRuntimeIngestion(ctx, in.ID, in.RuntimeID, false)
	if err != nil {
		return nil, err
	}
	target, err := s.resolveTarget(ctx, i)
	if err != nil {
		return nil, err
	}

	results := make([]JobResult, len(in.Body.Records))
	var jobs []dispatchjob.DispatchJob
	var at []int // the record each job in jobs came from
	for n, rec := range in.Body.Records {
		results[n].Index = n
		item, problem := target.item(rec, in.RuntimeID)
		if problem == "" {
			var j dispatchjob.DispatchJob
			if j, err = sdk.NewJobFromItem(item); err != nil {
				problem = err.Error()
			} else {
				j.ConnectionID = target.connectionID
				jobs, at = append(jobs, j), append(at, n)
			}
		}
		if problem != "" {
			results[n].Error = &problem
		}
	}

	var keys []ingestion.JobKey
	for k, j := range jobs {
		if j.ExternalID != nil {
			keys = append(keys, ingestion.JobKey{ExternalID: *j.ExternalID, JobID: jobs[k].ID})
		}
	}
	created := 0
	// The lease (a polled source) and the client's status are checked in the
	// transaction that makes the jobs, with the lease row held until it commits.
	err = s.Repo.WithJobKeys(ctx, i, in.RuntimeID, keys, s.now(), func(ctx context.Context, tx pgx.Tx, existing map[string]string) error {
		fresh := make([]dispatchjob.DispatchJob, 0, len(jobs))
		for k, j := range jobs {
			if id, seen := existing[j.ID]; seen {
				results[at[k]].ID, results[at[k]].Existing = id, true
				continue
			}
			results[at[k]].ID = j.ID
			fresh = append(fresh, j)
		}
		created = len(fresh)
		if len(fresh) == 0 {
			return nil
		}
		_, err := s.Jobs.Lifecycle().In(tx).CreateBatch(ctx, fresh)
		return err
	})
	if err != nil {
		if usecase.AsError(err) != nil {
			return nil, err // LEASE_NOT_HELD, CLIENT_NOT_ACTIVE
		}
		return nil, usecase.Internal("REPO", "create jobs failed", err)
	}
	return &apicommon.Out[CreateJobsResponse]{Body: CreateJobsResponse{Created: created, Results: results}}, nil
}

// jobTarget is an ingestion's target resolved to what its jobs carry.
type jobTarget struct {
	ingestion *ingestion.Ingestion
	// target is the effective one: its delivery URL may be the config entry's.
	target         ingestion.Target
	source         string
	poolID         string
	connectionID   *string
	serviceAccount *string
}

// resolveTarget resolves the ingestion's target as the save did. A target
// that no longer resolves (a pool paused, a connection deleted since) is a 409
// on the ingestion, not a fault in the runtime's request.
func (s *State) resolveTarget(ctx context.Context, i *ingestion.Ingestion) (*jobTarget, error) {
	app, err := s.Resolver.ApplicationByID(ctx, string(i.ApplicationID))
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, usecase.Internal("REPO", "ingestion's application not found", nil)
	}
	effective, err := s.effective(ctx, app.Code, i)
	if err != nil {
		return nil, err
	}
	rt, err := s.Resolver.Target(ctx, app.Code, i.ClientID, effective.Definition.Target)
	if err != nil {
		if e := usecase.AsError(err); e != nil && e.Kind == usecase.KindValidation {
			return nil, usecase.Conflict(e.Code, e.Message)
		}
		return nil, err
	}
	t := &jobTarget{
		ingestion: i, target: effective.Definition.Target, source: "ingestion:" + app.Code + ":" + i.Code,
		poolID: rt.PoolID, connectionID: rt.ConnectionID, serviceAccount: rt.ServiceAccountID,
	}
	if rt.ConnectionID != nil {
		conn, err := s.Connections.FindByID(ctx, *rt.ConnectionID)
		if err != nil {
			return nil, usecase.Internal("REPO", "connection lookup failed", err)
		}
		if conn == nil {
			return nil, usecase.Conflict("INGESTION_TARGET_INVALID", "target.connection: the connection no longer exists")
		}
		t.serviceAccount = &conn.ServiceAccountID
	}
	return t, nil
}

// Lengths of the columns a record's fields land in.
const (
	maxContentType = 100
	maxGroup       = 200
	maxExternalID  = 100
	maxCorrelation = 100
	maxSubject     = 500
)

// item is the batch item a record becomes under the target, or why it cannot.
func (t *jobTarget) item(rec JobRecord, runtimeID string) (sdk.BatchItem, string) {
	switch {
	case rec.Payload == "":
		return sdk.BatchItem{}, "payload is required"
	case utf8.RuneCountInString(rec.PayloadContentType) > maxContentType:
		return sdk.BatchItem{}, tooLong("payloadContentType", maxContentType)
	case tooLongPtr(rec.MessageGroup, maxGroup):
		return sdk.BatchItem{}, tooLong("messageGroup", maxGroup)
	case rec.ExternalID != nil && strings.TrimSpace(*rec.ExternalID) == "":
		return sdk.BatchItem{}, "externalId may not be blank"
	case tooLongPtr(rec.ExternalID, maxExternalID):
		return sdk.BatchItem{}, tooLong("externalId", maxExternalID)
	case tooLongPtr(rec.CorrelationID, maxCorrelation):
		return sdk.BatchItem{}, tooLong("correlationId", maxCorrelation)
	case tooLongPtr(rec.Subject, maxSubject):
		return sdk.BatchItem{}, tooLong("subject", maxSubject)
	}
	target, clientID := t.target, string(t.ingestion.ClientID)
	return sdk.BatchItem{
		ExternalID: rec.ExternalID, Code: target.Code, Source: &t.source, Subject: rec.Subject,
		TargetURL: target.TargetURL, Payload: &rec.Payload, PayloadContentType: rec.PayloadContentType,
		CorrelationID: rec.CorrelationID, ClientID: &clientID, ServiceAccountID: t.serviceAccount,
		DispatchPoolID: &t.poolID, MessageGroup: rec.MessageGroup, Mode: target.Mode,
		Metadata: []dispatchjob.Metadata{{Key: "ingestionId", Value: t.ingestion.ID}, {Key: "runtimeId", Value: runtimeID}},
	}, ""
}

func tooLong(field string, max int) string {
	return fmt.Sprintf("%s is longer than %d characters", field, max)
}

func tooLongPtr(s *string, max int) bool { return s != nil && utf8.RuneCountInString(*s) > max }

// effective is the ingestion's live definition with its config entry's
// settings, when its source names one. An entry that is missing or invalid is
// a 409: the ingestion is not listed to a runtime meanwhile, and one that asks
// anyway (it polled before the entry went) is told so.
func (s *State) effective(ctx context.Context, applicationCode string, i *ingestion.Ingestion) (ingestion.Resolved, error) {
	res, err := s.Repo.Resolve(ctx, applicationCode, i.ClientID, i.Code, i.Definition)
	if err != nil {
		return res, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	if !res.Runnable() {
		return res, usecase.Conflict("INGESTION_CONFIG_NOT_READY",
			"the ingestion's config entry "+res.Key+" is "+strings.ToLower(string(res.Status))+"; it runs once the entry is valid")
	}
	return res, nil
}
