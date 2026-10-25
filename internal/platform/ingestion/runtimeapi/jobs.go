package runtimeapi

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
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
func (s *State) requireRuntimeIngestion(ctx context.Context, id, runtimeID string, withLease bool) (*ingestion.Ingestion, error) {
	i, err := s.requireIngestion(ctx, id)
	if err != nil {
		return nil, err
	}
	if !i.Live() {
		return nil, usecase.Conflict("INGESTION_NOT_LIVE", "ingestion "+id+" is paused or has no live version")
	}
	if withLease {
		err = s.Repo.CheckRuntime(ctx, i, runtimeID, s.now())
	} else {
		err = s.Repo.CheckRuntimeKind(ctx, i, runtimeID)
	}
	if err != nil {
		if usecase.AsError(err) != nil {
			return nil, err
		}
		return nil, usecase.Internal("REPO", "runtime check failed", err)
	}
	if withLease {
		c, err := s.Resolver.Client(ctx, string(i.ClientID))
		if err != nil {
			return nil, err
		}
		if err := ingestion.ClientActive(string(c.Status), c.ID); err != nil {
			return nil, err
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
	ingestion      *ingestion.Ingestion
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
	rt, err := s.Resolver.Target(ctx, app.Code, i.ClientID, i.Definition.Target)
	if err != nil {
		if e := usecase.AsError(err); e != nil && e.Kind == usecase.KindValidation {
			return nil, usecase.Conflict(e.Code, e.Message)
		}
		return nil, err
	}
	t := &jobTarget{
		ingestion: i, source: "ingestion:" + app.Code + ":" + i.Code,
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
	target, clientID := t.ingestion.Definition.Target, string(t.ingestion.ClientID)
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

type getSecretInput struct {
	ID        string `path:"id"`
	Name      string `path:"name" pattern:"^[A-Za-z0-9._:-]{1,201}$" doc:"A secret the ingestion's live definition names"`
	RuntimeID string `query:"runtimeId" required:"true" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The asking runtime, which must carry the ingestion's kind and for a polled source hold its lease"`
}

type secretOutput struct {
	// CacheControl keeps the plaintext out of every shared or browser cache.
	CacheControl string `header:"Cache-Control"`
	Body         SecretValue
}

// getSecret reveals a secret the ingestion names, from the ingestion's
// application, section "secrets" and client with no global fallback, through
// the audited reveal of platform config. The runtime holds no reveal
// permission: it may read what the ingestion's own definition names, and
// nothing else.
func (s *State) getSecret(ctx context.Context, in *getSecretInput) (*secretOutput, error) {
	i, err := s.requireRuntimeIngestion(ctx, in.ID, in.RuntimeID, true)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(ingestion.SecretNames(i.Definition.Source), in.Name) {
		return nil, usecase.NotFound("SECRET_NOT_FOUND", "the ingestion names no secret "+in.Name)
	}
	app, err := s.Resolver.ApplicationByID(ctx, string(i.ApplicationID))
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, usecase.Internal("REPO", "ingestion's application not found", nil)
	}
	r, err := s.Config.RevealAuthorised(ctx, auth.FromContext(ctx), app.Code, "secrets", in.Name, string(i.ClientID),
		map[string]string{"ingestionId": i.ID, "ingestionCode": i.Code, "runtimeId": in.RuntimeID})
	if err != nil {
		return nil, err
	}
	return &secretOutput{CacheControl: "no-store", Body: SecretValue{Value: r.Value, UpdatedAt: jsontime.New(r.Config.UpdatedAt)}}, nil
}
