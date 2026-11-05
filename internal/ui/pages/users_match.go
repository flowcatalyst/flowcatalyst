package pages

import (
	"net/http"
	"strconv"
	"strings"

	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// Match emails: paste a list of addresses, see which are users. The page is
// POST /api/principals/match-emails run in-process (principalapi.State), so the
// permission gate, the parsing and the per-row visibility rule are the API's:
// a user the caller may not see is listed as not found.

const (
	usrMatchPath = usersPath + "/match-emails"
	usrMatchForm = "usr-match"
	usrMatchKey  = "emails"
)

// usrMatchFieldCodes maps the API's validation codes to the textarea.
var usrMatchFieldCodes = map[string]string{
	"EMAILS_REQUIRED": usrMatchKey,
	"TEXT_TOO_LONG":   usrMatchKey,
	"TOO_MANY_EMAILS": usrMatchKey,
}

// usrMatchView is the page: the pasted text kept, and the outcome of a post.
type usrMatchView struct {
	usrBase
	Text string
	// Done is whether the text was matched; the lists below are then its result.
	Done                   bool
	Rows                   []usrRow
	Unmatched, Invalid     []string
	Matched, NotFound, Bad int
	Perms                  usrPerms
}

func newUsrMatchView(ac *auth.AuthContext) usrMatchView {
	return usrMatchView{usrBase: newUsrBase(usrMatchForm, usrMatchKey), Perms: usrPermsFor(ac)}
}

// summary is the one-line count ("12 matched, 3 not found, 1 not an email").
func (v usrMatchView) summary() string {
	return strconv.Itoa(v.Matched) + " matched, " + strconv.Itoa(v.NotFound) + " not found, " + strconv.Itoa(v.Bad) + " not an email"
}

func (v usrMatchView) rowsText(list []string) string { return strings.Join(list, "\n") }

func (h *Users) matchPage(w http.ResponseWriter, r *http.Request, status int, v usrMatchView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Match emails", Path: usersPath}, usrMatchPage(v))
}

func (h *Users) matchForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/principals/match-emails's check (the list's).
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.matchPage(w, r, http.StatusOK, newUsrMatchView(ac))
}

func (h *Users) match(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := newUsrMatchView(ac)
	v.Text = r.PostFormValue(usrMatchKey)
	res, err := h.Users.MatchPrincipalEmails(ctx, principalapi.MatchEmailsRequest{Text: v.Text})
	if err != nil {
		v.Errs.apply(r, err, usrMatchFieldCodes)
		h.matchPage(w, r, v.Errs.Status, v)
		return
	}
	names, err := h.clientIndex(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Done = true
	for i := range res.Matches {
		v.Rows = append(v.Rows, toUsrRow(&res.Matches[i], names))
	}
	v.Unmatched, v.Invalid = res.Unmatched, res.Invalid
	v.Matched, v.NotFound, v.Bad = len(res.Matches), len(res.Unmatched), len(res.Invalid)
	h.matchPage(w, r, http.StatusOK, v)
}
