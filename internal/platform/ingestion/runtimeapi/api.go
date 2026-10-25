// Package runtimeapi wires the routes an ingestion runtime calls
// (docs/ingestion-design.md section 7): poll, report, state, job creation and secrets, under
// /api/ingestion-runtime/v1, as the runtime's service account holding
// platform:ingestion:runtime:operate. The runtime is not an application SDK
// client; these operations are not in the application SDKs.
package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// State bundles the dependencies. Jobs, Resolver, Connections and Config back
// the job and secret calls only.
type State struct {
	Repo *ingestion.Repository
	// Jobs is where a record's dispatch job is created.
	Jobs *dispatchjob.Repository
	// Resolver resolves an ingestion's target (and its application) as the
	// save did.
	Resolver ingestion.Resolver
	// Connections finds the service account of the target's connection, which
	// signs the jobs.
	Connections *connection.Repository
	// Config reads the secrets an ingestion's live definition binds
	// (FetchBoundSecretForIngestionRuntime: not a reveal, not audited per
	// read).
	Config *platformconfig.Service
	// Audit records each refused secret fetch (SECRET_FETCH_REFUSED). A
	// successful fetch is security telemetry, not an audit row.
	Audit *audit.Repository
	// Lease is how long a lease lasts without being renewed; zero is
	// ingestion.DefaultLease.
	Lease time.Duration
	// Now is a clock for tests. Nil is the database's clock for everything a
	// lease decides (taking, renewing, holding, expiring), so replicas whose
	// own clocks disagree still agree on who holds what.
	Now func() time.Time
	// JobsMaxBytes and ReportMaxBytes bound the request bodies of the jobs and
	// report calls; zero reads FC_INGESTION_JOBS_MAX_BYTES and
	// FC_INGESTION_REPORT_MAX_BYTES (DefaultJobsMaxBytes, DefaultReportMaxBytes).
	JobsMaxBytes, ReportMaxBytes int64
}

// The request-body bounds of the two calls that carry payloads. A jobs
// request holds up to 1,000 records (MaxJobRecords), so the limit is per
// request, not per record: 10 MiB (owner ruling) is about 10 KB a record at
// the most. A report holds up to 500 failures with their excerpts and 2,000
// windows and route states: 4 MiB. Every other runtime call keeps huma's
// default of 1 MiB.
const (
	DefaultJobsMaxBytes   int64 = 10 << 20
	DefaultReportMaxBytes int64 = 4 << 20
)

func bodyLimit(set int64, env string, def int64) int64 {
	if set > 0 {
		return set
	}
	if n := envutil.Int(env, 0); n > 0 {
		return int64(n)
	}
	return def
}

const tag = "ingestion-runtime"

// The operations whose request bodies are relaxed all the way down
// (httpcompat.RelaxRequestBodiesDeep): a newer runtime may add a field to an
// entry of its poll or report, which is an additive change that does not bump
// the contract version, and an older platform must not refuse the whole call.
const (
	PollOperationID   = "ingestionRuntimePoll"
	ReportOperationID = "ingestionRuntimeReport"
)

// Register mounts the runtime endpoints. The server mounts them only when
// FC_INGESTION_ENABLED is on; the spec generator always registers them.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	huma.Register(api, huma.Operation{
		OperationID:   PollOperationID,
		Method:        http.MethodPost,
		Path:          "/api/ingestion-runtime/v1/poll",
		Summary:       "A runtime asks what it should run (ingestion runtime only)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusUpgradeRequired},
	}, s.poll)
	// The report and jobs calls carry payloads, so they take bigger bodies
	// than apiroute's default; huma.Register directly sets MaxBodyBytes.
	huma.Register(api, huma.Operation{
		OperationID:   ReportOperationID,
		Method:        http.MethodPost,
		Path:          "/api/ingestion-runtime/v1/report",
		Summary:       "A runtime reports route states, exchange windows, failures and test answers (ingestion runtime only)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
		MaxBodyBytes:  bodyLimit(s.ReportMaxBytes, "FC_INGESTION_REPORT_MAX_BYTES", DefaultReportMaxBytes),
	}, s.report)
	apiroute.Get(g, "ingestionRuntimeGetState", "/api/ingestion-runtime/v1/state/{ingestionId}/{key}", "Read a value a polled source keeps (lease holder only)", s.getState)
	huma.Register(api, huma.Operation{
		OperationID:   CreateJobsOperationID,
		Method:        http.MethodPost,
		Path:          "/api/ingestion-runtime/v1/ingestions/{id}/jobs",
		Summary:       "Create dispatch jobs from records, under the ingestion's own target (ingestion runtime only)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
		MaxBodyBytes:  bodyLimit(s.JobsMaxBytes, "FC_INGESTION_JOBS_MAX_BYTES", DefaultJobsMaxBytes),
	}, s.createJobs)
	apiroute.Get(g, GetSecretOperationID, "/api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}", "Fetch a secret the ingestion's live definition binds (ingestion runtime only; security telemetry, not an audited reveal)", s.getSecret)
	apiroute.Put(g, "ingestionRuntimePutState", "/api/ingestion-runtime/v1/state/{ingestionId}/{key}", "Store a value a polled source keeps (lease holder only)", http.StatusNoContent, s.putState)
}

// now is the time the repository decides as of: the test clock, or zero,
// which the repository reads as the database's clock.
func (s *State) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Time{}
}

func (s *State) lease() time.Duration {
	if s.Lease > 0 {
		return s.Lease
	}
	return ingestion.DefaultLease
}

// contractError is the 426 for a contract version the platform does not serve,
// with the range it does, in the platform's error envelope.
type contractError struct {
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func (e *contractError) Error() string  { return e.Message }
func (e *contractError) GetStatus() int { return http.StatusUpgradeRequired }

func unsupportedContract(got int) *contractError {
	return &contractError{
		Code: "CONTRACT_VERSION_UNSUPPORTED",
		Message: fmt.Sprintf("contract version %d is not supported; the platform serves %d to %d",
			got, ingestion.MinContractVersion, ingestion.ContractVersion),
		Details: map[string]any{"minContractVersion": ingestion.MinContractVersion, "maxContractVersion": ingestion.ContractVersion},
	}
}

type pollInput struct {
	Body PollRequest
}

func (s *State) poll(ctx context.Context, in *pollInput) (*apicommon.Out[PollResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireIngestionRuntime(ac); err != nil {
		return nil, err
	}
	b := in.Body
	if !ingestion.SupportedContract(b.ContractVersion) {
		return nil, unsupportedContract(b.ContractVersion)
	}
	req := ingestion.PollRequest{
		RuntimeID: b.RuntimeID, ContractVersion: b.ContractVersion, Kinds: b.Kinds, BuiltinKinds: knownBuiltins(b.BuiltinKinds),
		URL: b.URL, Now: s.now(), Lease: s.lease(),
	}
	req.Clients, req.Applications = ingestion.ListReach(runtimeReach(ac))
	res, err := s.Repo.Poll(ctx, req)
	if err != nil {
		return nil, usecase.Internal("REPO", "poll failed", err)
	}
	body := PollResponse{
		AsOf: jsontime.New(res.AsOf), LeaseFor: ingestion.Duration(s.lease()).String(),
		Ingestions: make([]RuntimeIngestion, 0, len(res.Ingestions)), Tests: make([]RuntimeTest, 0, len(res.Tests)),
	}
	for _, item := range res.Ingestions {
		ri, err := fromIngestion(item.Ingestion, item.ApplicationCode, item.ClientIdentifier)
		if err != nil {
			return nil, usecase.Internal("MARSHAL", "ingestion does not render", err)
		}
		body.Ingestions = append(body.Ingestions, ri)
	}
	for _, t := range res.Tests {
		ri, err := fromTest(t)
		if err != nil {
			return nil, usecase.Internal("MARSHAL", "test does not render", err)
		}
		body.Tests = append(body.Tests, RuntimeTest{ID: t.ID, ExpiresAt: jsontime.New(t.ExpiresAt), Ingestion: ri})
	}
	return &apicommon.Out[PollResponse]{Body: body}, nil
}

// knownBuiltins keeps the built-in kinds this platform knows from a poll's
// builtinKinds: a name it does not know (a newer runtime's kind) runs nothing
// here, so it is not stored. Nil (not declared) stays nil; a declared list
// stays a list, empty when nothing in it is known.
func knownBuiltins(declared []string) []string {
	if declared == nil {
		return nil
	}
	out := []string{}
	for _, k := range declared {
		if k != ingestion.KindCustom && ingestion.IsBuiltInKind(k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// fromIngestion renders the live definition in the contract's form.
func fromIngestion(i ingestion.Ingestion, applicationCode, clientIdentifier string) (RuntimeIngestion, error) {
	source, err := json.Marshal(i.Definition.Source)
	if err != nil {
		return RuntimeIngestion{}, err
	}
	split, err := json.Marshal(i.Definition.Split)
	if err != nil {
		return RuntimeIngestion{}, err
	}
	return RuntimeIngestion{
		ID: i.ID, Version: i.Version, ClientID: string(i.ClientID), ClientIdentifier: clientIdentifier,
		ApplicationCode: applicationCode, Code: i.Code, Name: i.Definition.Name,
		Source: source, Split: split, Target: i.Definition.Target, KeepFailedPayloads: i.Definition.KeepFailedPayloads,
	}, nil
}

// testIngestionID is the id of the ingestion in a test of a definition that is
// not saved.
const testIngestionID = "ing_test"

func fromTest(t ingestion.TestRequest) (RuntimeIngestion, error) {
	id, code := testIngestionID, "test"
	if t.IngestionID != nil {
		id, code = *t.IngestionID, t.IngestionCode
	}
	return fromIngestion(ingestion.Ingestion{
		ID: id, ClientID: t.ClientID, Code: code, Definition: t.Definition,
	}, t.ApplicationCode, t.ClientIdentifier)
}

type reportInput struct {
	Body ReportRequest
}

func (s *State) report(ctx context.Context, in *reportInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireIngestionRuntime(ac); err != nil {
		return nil, err
	}
	b := in.Body
	rep := ingestion.Report{RuntimeID: b.RuntimeID, Caller: runtimeReach(ac), Now: s.now()}
	for _, r := range b.Routes {
		rep.Routes = append(rep.Routes, ingestion.RouteReport{IngestionID: r.IngestionID, Version: r.Version, Status: r.Status, Error: r.Error})
	}
	for _, w := range b.Metrics {
		from, to := w.From.Underlying(), w.To.Underlying()
		if to.Before(from) || to.Sub(from) > 24*time.Hour {
			return nil, usecase.Validation("REPORT_WINDOW_INVALID", "a window ends after it starts and lasts at most a day: "+w.IngestionID)
		}
		rep.Metrics = append(rep.Metrics, ingestion.WindowReport{
			IngestionID: w.IngestionID, From: from, To: to,
			Completed: w.Completed, Failed: w.Failed, Records: w.Records, Dispatch: w.DispatchJobs,
		})
	}
	for _, f := range b.Failures {
		category := f.Category
		if category == "" {
			category = ingestion.FailurePayload
		}
		if !ingestion.ValidFailureCategory(category) {
			// The schema's enum says so too; this holds when the relaxed body
			// check lets an unknown value through.
			return nil, usecase.Validation("REPORT_FAILURE_CATEGORY_INVALID",
				"a failure's category is payload or rejected: "+f.IngestionID)
		}
		rep.Failures = append(rep.Failures, ingestion.FailureReport{
			IngestionID: f.IngestionID, Version: f.Version, At: f.At.Underlying(), Kind: f.Kind, Category: category,
			Message: f.Message, AfterRecords: f.AfterRecords, Excerpt: f.Excerpt,
		})
	}
	for _, t := range b.Tests {
		for _, step := range t.Steps {
			if utf8.RuneCountInString(step) > maxStepLength {
				return nil, usecase.Validation("REPORT_STEP_TOO_LONG",
					fmt.Sprintf("a step of test %s is longer than %d characters", t.ID, maxStepLength))
			}
		}
		steps := t.Steps
		if steps == nil {
			steps = []string{}
		}
		rep.Tests = append(rep.Tests, ingestion.TestAnswer{
			TestID: t.ID, Result: ingestion.TestResult{Outcome: t.Outcome, Steps: steps, Records: t.Records, Excerpt: t.Excerpt},
		})
	}
	if _, err := s.Repo.Record(ctx, rep); err != nil {
		return nil, usecase.Internal("REPO", "record report failed", err)
	}
	return &apicommon.Empty{}, nil
}

// maxStepLength bounds one step of a test answer, in characters.
const maxStepLength = 500

type getStateInput struct {
	IngestionID string `path:"ingestionId"`
	Key         string `path:"key" pattern:"^[A-Za-z0-9._:-]{1,100}$"`
	RuntimeID   string `query:"runtimeId" required:"true" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The asking runtime, which must hold the ingestion's lease"`
}

type putStateInput struct {
	IngestionID string `path:"ingestionId"`
	Key         string `path:"key" pattern:"^[A-Za-z0-9._:-]{1,100}$"`
	RuntimeID   string `query:"runtimeId" required:"true" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The writing runtime, which must hold the ingestion's lease"`
	Body        StateValue
}

// runtimeReach is the reach of a caller that passed auth.RequireIngestionRuntime:
// every client and every application. A runtime is infrastructure that serves the
// whole platform, and one fleet runs every application's ingestions (the fair share
// spreads them over its instances). The caller is already known to be an anchor-tier
// service account bound to no application (owner ruling 2026-10-25: a client-tier or
// application-bound account is refused before this, never silently widened), so this
// only spells out the reach it has: an account bound to no application holds no
// application list to confine it by.
func runtimeReach(ac *auth.AuthContext) *auth.AuthContext {
	r := *ac
	r.Scope = auth.ScopeAnchor
	r.AllApplications = true
	return &r
}

// requireIngestion checks the caller may operate the runtime API and reaches
// the ingestion, and returns it.
func (s *State) requireIngestion(ctx context.Context, id string) (*ingestion.Ingestion, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireIngestionRuntime(ac); err != nil {
		return nil, err
	}
	i, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return nil, operations.NotFound(id)
	}
	return i, ingestion.CheckReach(runtimeReach(ac), i)
}

func (s *State) getState(ctx context.Context, in *getStateInput) (*apicommon.Out[StateValue], error) {
	if _, err := s.requireIngestion(ctx, in.IngestionID); err != nil {
		return nil, err
	}
	value, found, err := s.Repo.GetState(ctx, in.IngestionID, in.RuntimeID, in.Key, s.now())
	if err != nil {
		return nil, stateError(err)
	}
	if !found {
		return nil, usecase.NotFound("STATE_NOT_FOUND", "nothing is stored under that key")
	}
	return &apicommon.Out[StateValue]{Body: StateValue{Value: value}}, nil
}

func (s *State) putState(ctx context.Context, in *putStateInput) (*apicommon.Empty, error) {
	if _, err := s.requireIngestion(ctx, in.IngestionID); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(in.Body.Value) > ingestion.MaxStateValue {
		return nil, usecase.Validation("STATE_VALUE_TOO_LONG", fmt.Sprintf("a value is at most %d characters", ingestion.MaxStateValue))
	}
	if err := s.Repo.PutState(ctx, in.IngestionID, in.RuntimeID, in.Key, in.Body.Value, s.now()); err != nil {
		return nil, stateError(err)
	}
	return &apicommon.Empty{}, nil
}

// stateError passes the lease refusal through and makes anything else a 500.
func stateError(err error) error {
	if usecase.AsError(err) != nil {
		return err
	}
	return usecase.Internal("REPO", "state failed", err)
}
