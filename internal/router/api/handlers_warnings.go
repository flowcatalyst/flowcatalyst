package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

const tagWarnings = "warnings"

func registerWarnings(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "listWarnings", Method: http.MethodGet, Path: "/warnings",
		Summary: "List warnings", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.listWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "clearAllWarnings", Method: http.MethodDelete, Path: "/warnings",
		Summary: "Clear all warnings", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.clearAllWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "acknowledgeWarning", Method: http.MethodPost, Path: "/warnings/{id}/acknowledge",
		Summary: "Acknowledge a warning", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.acknowledgeWarning)
	huma.Register(api, huma.Operation{
		OperationID: "acknowledgeAllWarnings", Method: http.MethodPost, Path: "/warnings/acknowledge-all",
		Summary: "Acknowledge every unacknowledged warning", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.acknowledgeAllWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "criticalWarnings", Method: http.MethodGet, Path: "/warnings/critical",
		Summary: "List critical warnings", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.criticalWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "unacknowledgedWarnings", Method: http.MethodGet, Path: "/warnings/unacknowledged",
		Summary: "List unacknowledged warnings", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.unacknowledgedWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "warningsBySeverity", Method: http.MethodGet, Path: "/warnings/severity/{severity}",
		Summary: "Filter warnings by severity", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.warningsBySeverity)
	huma.Register(api, huma.Operation{
		OperationID: "clearOldWarnings", Method: http.MethodDelete, Path: "/warnings/old",
		Summary: "Purge warnings older than ?hours", Tags: []string{tagWarnings}, DefaultStatus: http.StatusOK,
	}, s.clearOldWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringUnacknowledged", Method: http.MethodGet, Path: "/monitoring/warnings/unacknowledged",
		Summary: "Unacknowledged warnings (dashboard alias)", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.unacknowledgedWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringWarningsBySeverity", Method: http.MethodGet, Path: "/monitoring/warnings/severity/{severity}",
		Summary: "Filter warnings by severity (dashboard alias)", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.warningsBySeverity)
}

type listWarningsInput struct {
	Severity     string `query:"severity"`
	Category     string `query:"category"`
	Acknowledged string `query:"acknowledged"`
}

type listWarningsOutput struct {
	Body []WireWarning
}

func (s *State) listWarnings(_ context.Context, in *listWarningsInput) (*listWarningsOutput, error) {
	var warnings []router.Warning
	if in.Acknowledged == "false" {
		warnings = s.Warnings.Unacknowledged()
	} else {
		warnings = s.Warnings.All()
	}
	if sev := strings.ToUpper(in.Severity); sev != "" {
		warnings = filterWarnings(warnings, func(w router.Warning) bool {
			return matchesSeverity(w.Severity, sev)
		})
	}
	if cat := strings.ToUpper(in.Category); cat != "" {
		warnings = filterWarnings(warnings, func(w router.Warning) bool {
			return strings.ToUpper(string(w.Category)) == cat
		})
	}
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].CreatedAt.After(warnings[j].CreatedAt)
	})
	return &listWarningsOutput{Body: fromWarnings(warnings)}, nil
}

func matchesSeverity(have router.WarningSeverity, want string) bool {
	if want == "WARN" {
		return have == router.WarningWarning
	}
	return strings.EqualFold(string(have), want)
}

func filterWarnings(in []router.Warning, ok func(router.Warning) bool) []router.Warning {
	out := in[:0]
	for _, w := range in {
		if ok(w) {
			out = append(out, w)
		}
	}
	return out
}

type clearAllOutput struct {
	Body CountResponse
}

func (s *State) clearAllWarnings(_ context.Context, _ *emptyInput) (*clearAllOutput, error) {
	ids := make([]string, 0)
	for _, w := range s.Warnings.All() {
		ids = append(ids, w.ID)
	}
	for _, id := range ids {
		s.Warnings.Remove(id)
	}
	return &clearAllOutput{Body: CountResponse{Cleared: uint64(len(ids))}}, nil
}

type acknowledgeInput struct {
	ID string `path:"id"`
}

type acknowledgeOutput struct {
	Body AcknowledgedResponse
}

func (s *State) acknowledgeWarning(_ context.Context, in *acknowledgeInput) (*acknowledgeOutput, error) {
	if s.Warnings.Acknowledge(in.ID) {
		return &acknowledgeOutput{Body: AcknowledgedResponse{Acknowledged: true}}, nil
	}
	return nil, huma.Error404NotFound("Warning not found: " + in.ID)
}

type acknowledgeAllOutput struct {
	Body AcknowledgedCountResponse
}

func (s *State) acknowledgeAllWarnings(_ context.Context, _ *emptyInput) (*acknowledgeAllOutput, error) {
	n := s.Warnings.AcknowledgeMatching(func(router.Warning) bool { return true })
	return &acknowledgeAllOutput{Body: AcknowledgedCountResponse{Acknowledged: uint64(n)}}, nil
}

type warningsListOutput struct {
	Body []WireWarning
}

func (s *State) criticalWarnings(_ context.Context, _ *emptyInput) (*warningsListOutput, error) {
	return &warningsListOutput{Body: fromWarnings(s.Warnings.Critical())}, nil
}

func (s *State) unacknowledgedWarnings(_ context.Context, _ *emptyInput) (*warningsListOutput, error) {
	return &warningsListOutput{Body: fromWarnings(s.Warnings.Unacknowledged())}, nil
}

type warningsBySeverityInput struct {
	Severity string `path:"severity"`
}

func (s *State) warningsBySeverity(_ context.Context, in *warningsBySeverityInput) (*warningsListOutput, error) {
	want := strings.ToUpper(in.Severity)
	all := s.Warnings.All()
	filtered := make([]router.Warning, 0, len(all))
	for _, w := range all {
		if matchesSeverity(w.Severity, want) {
			filtered = append(filtered, w)
		}
	}
	return &warningsListOutput{Body: fromWarnings(filtered)}, nil
}

type clearOldInput struct {
	Hours int `query:"hours"`
}

func (s *State) clearOldWarnings(_ context.Context, in *clearOldInput) (*clearAllOutput, error) {
	hours := in.Hours
	if hours <= 0 {
		hours = 8
	}
	n := s.Warnings.ClearOlderThan(time.Duration(hours) * time.Hour)
	return &clearAllOutput{Body: CountResponse{Cleared: uint64(n)}}, nil
}
