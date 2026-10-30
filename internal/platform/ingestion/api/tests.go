package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// registerTests mounts the "test connection" routes. The test is pull-based
// (docs/ingestion-design.md section 7): the request is queued here, a runtime
// that carries the kind receives it in its next poll and posts the result
// through its report, and the page reads the result from the GET.
func registerTests(g apiroute.Group, s *State) {
	apiroute.Post(g, "testIngestionConnection", "/api/ingestion-tests", "Queue a test of an ingestion's source", http.StatusAccepted, s.queueTest)
	apiroute.Get(g, "getIngestionTest", "/api/ingestion-tests/{id}", "Get a queued test and, once a runtime has answered, its result", s.getTest)
}

func (s *State) queueTest(ctx context.Context, in *apicommon.In[TestIngestionRequest]) (*apicommon.Out[IngestionTestResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanTestIngestions(ac); err != nil {
		return nil, err
	}
	var (
		ingestionID   *string
		applicationID ids.ApplicationID
		clientID      ids.ClientID
		def           ingestion.Definition
	)
	switch b := in.Body; {
	case b.IngestionID != nil && b.Definition == nil && b.Application == "" && b.Client == "":
		i, err := s.find(ctx, *b.IngestionID)
		if err != nil {
			return nil, err
		}
		ingestionID, applicationID, clientID, def = &i.ID, i.ApplicationID, i.ClientID, i.Definition
		// The runtime is given the effective definition: the settings of the
		// config entry the source names, when it names one.
		app, err := s.Repo.ApplicationCode(ctx, i.ApplicationID)
		if err != nil {
			return nil, usecase.Internal("REPO", "application lookup failed", err)
		}
		if def, err = s.testable(ctx, app, i.ClientID, i.Code, def); err != nil {
			return nil, err
		}
		// A definition saved before the outbound guard existed is held to it
		// here: a test sends the runtime where the source points.
		if err := ingestion.CheckSourceAddresses(def.Source).Err(); err != nil {
			return nil, err
		}
	case b.IngestionID == nil && b.Definition != nil:
		var err error
		if def, err = parseDefinition(*b.Definition); err != nil {
			return nil, err
		}
		if err := def.Validate().Err(); err != nil {
			return nil, err
		}
		// The client is resolved only after the permission check, as for create.
		if clientID, err = s.resolveClient(ctx, b.Client); err != nil {
			return nil, err
		}
		if err := auth.CheckScopeAccess(ac, new(string(clientID))); err != nil {
			return nil, err
		}
		app, err := s.Resolver.Application(ctx, strings.TrimSpace(b.Application))
		if err != nil {
			return nil, err
		}
		if err := ingestion.CheckApplicationReach(ac, string(app.ID)); err != nil {
			return nil, err
		}
		applicationID = app.ID
		// The entry a source names is read now, and the runtime is given the
		// effective definition; an entry that is not there stops the test.
		if def, err = s.testable(ctx, app.Code, clientID, strings.TrimSpace(b.Code), def); err != nil {
			return nil, err
		}
		// Trying an unsaved definition runs it on a runtime, so it needs what
		// creating it would: the kind's permission, the runtime identity, the
		// secrets it points (the outbound guard is in Validate, above), judged
		// on the effective definition.
		if err := s.Repo.CheckAuthor(ctx, ac, ingestion.Change{Next: def}); err != nil {
			return nil, err
		}
	default:
		return nil, usecase.Validation("INGESTION_TEST_INVALID",
			"name either ingestionId, or application, client and definition")
	}
	by := ac.PrincipalID
	t, err := s.Repo.QueueTest(ctx, ingestionID, applicationID, clientID, def, &by, time.Now().UTC())
	if err != nil {
		return nil, usecase.Internal("REPO", "queue_test failed", err)
	}
	return s.testResponse(ctx, t)
}

// testable is the definition a runtime is given to test: def with the settings
// of its config entry. A test needs them to try anything, so an entry that is
// missing or invalid is a 400 (INGESTION_CONFIG_MISSING, INGESTION_CONFIG_INVALID)
// rather than a queued test no runtime could run. code is the ingestion's, the
// entry's key for `config: true`; an unsaved definition tested without one
// cannot use it.
func (s *State) testable(ctx context.Context, applicationCode string, clientID ids.ClientID, code string, def ingestion.Definition) (ingestion.Definition, error) {
	c, byConfig := ingestion.ConfigOf(def)
	if !byConfig {
		return def, nil
	}
	if c.Config.Own && code == "" {
		return def, usecase.Validation("INGESTION_CONFIG_KEY_UNKNOWN",
			"the source says config: true, the ingestion's code, which this test was not given; send code, or name the key")
	}
	res, err := s.Repo.Resolve(ctx, applicationCode, clientID, code, def)
	if err != nil {
		return def, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	switch res.Status {
	case ingestion.ConfigMissing:
		return def, usecase.Validation("INGESTION_CONFIG_MISSING",
			"the config entry "+res.Key+" is not set for this client; set it (PUT /api/ingestions/{id}/config) before testing")
	case ingestion.ConfigInvalid:
		return def, operations.EntryProblems(res).Err()
	case ingestion.ConfigInline, ingestion.ConfigSet:
	}
	return res.Definition, nil
}

func (s *State) getTest(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionTestResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanTestIngestions(ac); err != nil {
		return nil, err
	}
	t, err := s.Repo.FindTest(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_test failed", err)
	}
	if t == nil {
		return nil, usecase.NotFound("INGESTION_TEST_NOT_FOUND", "Ingestion test not found: "+in.ID)
	}
	if err := auth.CheckScopeAccess(ac, new(string(t.ClientID))); err != nil {
		return nil, err
	}
	if err := ingestion.CheckApplicationReach(ac, string(t.ApplicationID)); err != nil {
		return nil, err
	}
	return s.testResponse(ctx, t)
}

// testResponse renders a request as it reads now, with the notice that no
// runtime carries its kind while that is so (the request then expires
// unanswered, which is not a failure of the source).
func (s *State) testResponse(ctx context.Context, t *ingestion.TestRequest) (*apicommon.Out[IngestionTestResponse], error) {
	carries, err := s.Repo.AnyLiveRuntimeCarries(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "live runtimes failed", err)
	}
	body := IngestionTestResponse{
		ID: t.ID, IngestionID: t.IngestionID, Kind: t.Kind, Status: t.EffectiveStatus(time.Now().UTC()),
		RuntimeID: t.ClaimedBy, CreatedAt: jsontime.New(t.CreatedAt), ExpiresAt: jsontime.New(t.ExpiresAt),
	}
	if t.CompletedAt != nil {
		body.CompletedAt = new(jsontime.New(*t.CompletedAt))
	}
	if t.Result != nil {
		body.Outcome, body.Steps, body.Records, body.Excerpt = &t.Result.Outcome, t.Result.Steps, t.Result.Records, t.Result.Excerpt
	}
	if body.Status == ingestion.TestPending && !carries(t.Kind) {
		body.Notice = new(noRuntimeNotice)
	}
	return &apicommon.Out[IngestionTestResponse]{Body: body}, nil
}

// noRuntimeNotice is shown where an ingestion or a test waits for a runtime
// that carries its kind: nothing has failed, nobody is there to run it yet.
const noRuntimeNotice = "no runtime carries this kind"

// runtimeNotices returns, for the ingestions of a response, the notice that no
// runtime carries the kind of a running one: not a failure, nobody is there to
// run it yet. nil for everything else.
func (s *State) runtimeNotices(ctx context.Context) (func(*ingestion.Ingestion) *string, error) {
	carries, err := s.Repo.AnyLiveRuntimeCarries(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "live runtimes failed", err)
	}
	return func(i *ingestion.Ingestion) *string {
		if i.Live() && !carries(ingestion.RunKind(i.Definition.Source)) {
			return new(noRuntimeNotice)
		}
		return nil
	}, nil
}
