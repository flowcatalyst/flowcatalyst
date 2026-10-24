// Package runtimeapi wires the routes an ingestion runtime calls
// (docs/ingestion-design.md section 7): poll, report and state, under
// /api/ingestion-runtime/v1, as the runtime's service account holding
// platform:ingestion:runtime:operate. The runtime is not an application SDK
// client; these operations are not in the application SDKs.
package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// State bundles the dependencies.
type State struct {
	Repo *ingestion.Repository
	// Lease is how long a lease lasts without being renewed; zero is
	// ingestion.DefaultLease.
	Lease time.Duration
	// Now is the clock; nil is time.Now in UTC.
	Now func() time.Time
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
	apiroute.Post(g, ReportOperationID, "/api/ingestion-runtime/v1/report", "A runtime reports route states, exchange windows, failures and test answers (ingestion runtime only)", http.StatusNoContent, s.report)
	apiroute.Get(g, "ingestionRuntimeGetState", "/api/ingestion-runtime/v1/state/{ingestionId}/{key}", "Read a value a polled source keeps (lease holder only)", s.getState)
	apiroute.Put(g, "ingestionRuntimePutState", "/api/ingestion-runtime/v1/state/{ingestionId}/{key}", "Store a value a polled source keeps (lease holder only)", http.StatusNoContent, s.putState)
}

func (s *State) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
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
	if err := auth.CanOperateIngestionRuntime(ac); err != nil {
		return nil, err
	}
	b := in.Body
	if !ingestion.SupportedContract(b.ContractVersion) {
		return nil, unsupportedContract(b.ContractVersion)
	}
	now := s.now()
	req := ingestion.PollRequest{
		RuntimeID: b.RuntimeID, ContractVersion: b.ContractVersion, Kinds: b.Kinds, URL: b.URL, Now: now, Lease: s.lease(),
	}
	req.Clients, req.Applications = ingestion.ListReach(ac)
	res, err := s.Repo.Poll(ctx, req)
	if err != nil {
		return nil, usecase.Internal("REPO", "poll failed", err)
	}
	body := PollResponse{
		AsOf: jsontime.New(now), LeaseFor: ingestion.Duration(s.lease()).String(),
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
	if err := auth.CanOperateIngestionRuntime(ac); err != nil {
		return nil, err
	}
	b := in.Body
	rep := ingestion.Report{RuntimeID: b.RuntimeID, Caller: ac, Now: s.now()}
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
		rep.Failures = append(rep.Failures, ingestion.FailureReport{
			IngestionID: f.IngestionID, Version: f.Version, At: f.At.Underlying(), Kind: f.Kind, Message: f.Message,
			AfterRecords: f.AfterRecords, Excerpt: f.Excerpt,
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

// requireIngestion checks the caller may operate the runtime API and reaches
// the ingestion the state belongs to.
func (s *State) requireIngestion(ctx context.Context, id string) error {
	ac := auth.FromContext(ctx)
	if err := auth.CanOperateIngestionRuntime(ac); err != nil {
		return err
	}
	i, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return operations.NotFound(id)
	}
	return ingestion.CheckReach(ac, i)
}

func (s *State) getState(ctx context.Context, in *getStateInput) (*apicommon.Out[StateValue], error) {
	if err := s.requireIngestion(ctx, in.IngestionID); err != nil {
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
	if err := s.requireIngestion(ctx, in.IngestionID); err != nil {
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
