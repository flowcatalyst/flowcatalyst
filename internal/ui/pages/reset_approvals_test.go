package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	resetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// fakeResetService is the API State seen from the page: it records the
// decisions and fails them with err.
type fakeResetService struct {
	pending []resetapi.RequestDTO
	actions []string
	err     error
}

func (f *fakeResetService) Pending(context.Context) ([]resetapi.RequestDTO, error) {
	return append([]resetapi.RequestDTO(nil), f.pending...), f.err
}

func (f *fakeResetService) decide(name, id string) (string, error) {
	f.actions = append(f.actions, name+":"+id)
	if f.err != nil {
		return "", f.err
	}
	f.pending = append(f.pending[:0:0], slicesDeleteByID(f.pending, id)...)
	return "ok", nil
}

func slicesDeleteByID(in []resetapi.RequestDTO, id string) []resetapi.RequestDTO {
	var out []resetapi.RequestDTO
	for _, r := range in {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeResetService) Approve(_ context.Context, id string) (string, error) {
	return f.decide("approve", id)
}

func (f *fakeResetService) Deny(_ context.Context, id string) (string, error) {
	return f.decide("deny", id)
}

func newResetService() *fakeResetService {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return &fakeResetService{pending: []resetapi.RequestDTO{
		{ID: "rar_1", PrincipalID: "prn_1", Email: "ada@example.com", Name: "Ada <Admin>", CreatedAt: jsontime.New(at), ExpiresAt: jsontime.New(at.Add(24 * time.Hour))},
		{ID: "rar_2", PrincipalID: "prn_2", Email: "bob@example.com", CreatedAt: jsontime.New(at), ExpiresAt: jsontime.New(at.Add(24 * time.Hour))},
	}}
}

func serveResets(t *testing.T, svc *fakeResetService, q req) (int, string, http.Header) {
	t.Helper()
	rec := serveMounted(t, anchorAdmin(), q, &ResetApprovals{Service: svc, Shell: shellForTest()})
	return rec.Code, rec.Body.String(), rec.Header()
}

func TestResetApprovalsList(t *testing.T) {
	svc := newResetService()
	code, body, _ := serveResets(t, svc, req{target: "/ui/reset-approvals"})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, `<title>Password reset approvals · FlowCatalyst</title>`, "Approving emails them a reset link and clears their 2FA so they re-enrol.",
		"Ada &lt;Admin&gt;", "ada@example.com", `datetime="2026-10-01T09:00:00Z"`, `datetime="2026-10-02T09:00:00Z"`,
		`action="/ui/reset-approvals/rar_1/approve"`, `action="/ui/reset-approvals/rar_1/deny"`, "Approve", "Deny", "Refresh",
		`href="/ui/reset-approvals" class="fc-nav-item" title="Reset Approvals" aria-current="page"`)
	// The SPA shows the email as the name when there is none.
	mustContain(t, body, `<span class="block font-medium">bob@example.com</span>`)

	_, refresh, _ := serveResets(t, svc, req{target: "/ui/reset-approvals", datastar: true})
	mustContain(t, refresh, `id="ra-region"`, "ada@example.com")
	mustNotContain(t, refresh, "<title>")

	empty := &fakeResetService{}
	_, none, _ := serveResets(t, empty, req{target: "/ui/reset-approvals"})
	mustContain(t, none, "No pending reset requests.")
	mustNotContain(t, none, "Approve")
}

func TestResetApprovalsRefusal(t *testing.T) {
	// The API's check (user:update, the client of the request) is the
	// service's: its refusal is the answer, for the queue and for a decision.
	svc := newResetService()
	svc.err = usecase.Authorization("PERMISSION_DENIED", "permission required: platform:iam:user:update")
	code, body, _ := serveResets(t, svc, req{target: "/ui/reset-approvals"})
	if code != http.StatusForbidden {
		t.Errorf("list: status %d", code)
	}
	mustNotContain(t, body, "ada@example.com")
	_, flash, _ := serveResets(t, svc, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_1/approve", datastar: true, form: url.Values{}})
	mustContain(t, flash, "fc-flash-error", "permission required")
	mustNotContain(t, flash, "Approved")
	if code, _, _ := serveResets(t, svc, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_1/deny", form: url.Values{}}); code != http.StatusForbidden {
		t.Errorf("native deny: status %d", code)
	}
	// A request that was decided meanwhile is told so.
	svc.err = usecase.Validation("ALREADY_DECIDED", "request is no longer pending")
	_, gone, _ := serveResets(t, svc, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_1/approve", datastar: true, form: url.Values{}})
	mustContain(t, gone, "request is no longer pending")
}

func TestResetApprovalsDecisions(t *testing.T) {
	svc := newResetService()
	_, body, _ := serveResets(t, svc, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_1/approve", datastar: true, form: url.Values{}})
	if strings.Join(svc.actions, ",") != "approve:rar_1" {
		t.Fatalf("actions %v", svc.actions)
	}
	// The queue as it now stands, and the SPA's toast.
	mustContain(t, body, `id="ra-region"`, "bob@example.com", "Approved", "The user has been emailed a reset link.")
	mustNotContain(t, body, "ada@example.com")

	_, denied, _ := serveResets(t, svc, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_2/deny", datastar: true, form: url.Values{}})
	mustContain(t, denied, "Denied", "Request denied.", "No pending reset requests.")

	svc2 := newResetService()
	code, _, hdr := serveResets(t, svc2, req{method: http.MethodPost, target: "/ui/reset-approvals/rar_1/approve", form: url.Values{}})
	if code != http.StatusSeeOther || hdr.Get("Location") != "/ui/reset-approvals" {
		t.Errorf("native approve: %d %v", code, hdr)
	}
	// An id that could not be a request never reaches the service.
	svc3 := newResetService()
	if code, _, _ := serveResets(t, svc3, req{method: http.MethodPost, target: "/ui/reset-approvals/a'b/approve", form: url.Values{}}); code != http.StatusNotFound || len(svc3.actions) != 0 {
		t.Errorf("malformed id: %d %v", code, svc3.actions)
	}
	// Decisions are POST only.
	if code, _, _ := serveResets(t, svc3, req{target: "/ui/reset-approvals/rar_1/approve"}); code != http.StatusMethodNotAllowed && code != http.StatusNotFound {
		t.Errorf("GET approve: %d", code)
	}
}
