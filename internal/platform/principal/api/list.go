package api

import (
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// listInput carries the filter / sort / pagination query params for
// GET /api/principals. Every field is optional; an absent param means
// "no filter" (or the documented default). The SPA's UserListPage drives all
// of these.
type listInput struct {
	Type      string `query:"type" doc:"Filter by principal type (USER or SERVICE)"`
	ClientID  string `query:"clientId" doc:"Filter to principals homed at, or granted access to, this client"`
	Active    string `query:"active" doc:"Filter by active status (true/false); absent = both"`
	Q         string `query:"q" doc:"Case-insensitive substring search across name and email"`
	Roles     string `query:"roles" doc:"CSV of role names; matches principals holding any of them"`
	Page      int    `query:"page" doc:"0-based page index (default 0)"`
	PageSize  int    `query:"pageSize" doc:"Page size; <=0 returns all matches (default: all)"`
	SortField string `query:"sortField" doc:"Sort key: name | email | createdAt (default createdAt)"`
	SortOrder string `query:"sortOrder" doc:"Sort direction: asc | desc (default asc)"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[PrincipalListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}

	// Normalise the filter inputs once, outside the row loop.
	q := strings.ToLower(strings.TrimSpace(in.Q))
	wantType := strings.ToUpper(strings.TrimSpace(in.Type))
	wantClient := strings.TrimSpace(in.ClientID)
	wantRoles := splitCSV(in.Roles)

	filtered := make([]*principal.Principal, 0, len(rows))
	for i := range rows {
		p := &rows[i]
		if !PrincipalVisible(ac, p) {
			continue
		}
		if wantType != "" && string(p.Type) != wantType {
			continue
		}
		if in.Active == "true" && !p.Active {
			continue
		}
		if in.Active == "false" && p.Active {
			continue
		}
		if wantClient != "" && !principalMatchesClient(p, wantClient) {
			continue
		}
		if q != "" && !principalMatchesQuery(p, q) {
			continue
		}
		if len(wantRoles) > 0 && !principalHasAnyRole(p, wantRoles) {
			continue
		}
		filtered = append(filtered, p)
	}

	sortPrincipals(filtered, in.SortField, in.SortOrder)

	// Total is the full filtered count (pre-pagination) so the SPA paginator
	// can compute page counts correctly.
	total := len(filtered)
	filtered = paginate(filtered, in.Page, in.PageSize)

	out := make([]PrincipalResponse, 0, len(filtered))
	for _, p := range filtered {
		out = append(out, forCaller(ac, fromEntity(p)))
	}
	return &apicommon.Out[PrincipalListResponse]{Body: PrincipalListResponse{Principals: out, Total: total}}, nil
}

// matchEmails resolves a pasted email list against the users the caller may
// see. It filters FindAll in memory, exactly as list does: the same visibility
// code path, and the platform user table is small enough that a per-email
// lookup would only add a second copy of the access rule.
func (s *State) matchEmails(ctx context.Context, in *apicommon.In[MatchEmailsRequest]) (*apicommon.Out[MatchEmailsResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	if len(in.Body.Text) > maxMatchEmailsTextBytes {
		return nil, usecase.Validation("TEXT_TOO_LONG", "text is limited to 200000 bytes")
	}

	var emails, invalid []string
	seenEmail := map[string]struct{}{}
	seenInvalid := map[string]struct{}{}
	collect := func(text string) {
		es, is := principal.ParseEmailList(text)
		for _, e := range es {
			if _, ok := seenEmail[e]; !ok {
				seenEmail[e] = struct{}{}
				emails = append(emails, e)
			}
		}
		for _, i := range is {
			if _, ok := seenInvalid[i]; !ok {
				seenInvalid[i] = struct{}{}
				invalid = append(invalid, i)
			}
		}
	}
	for _, e := range in.Body.Emails {
		collect(e)
	}
	collect(in.Body.Text)

	if len(emails) == 0 && len(invalid) == 0 {
		return nil, usecase.Validation("EMAILS_REQUIRED", "no email addresses supplied")
	}
	if len(emails) > maxMatchEmails {
		return nil, usecase.Validation("TOO_MANY_EMAILS", "matching is limited to 1000 distinct email addresses")
	}

	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	byEmail := make(map[string]*principal.Principal, len(rows))
	for i := range rows {
		p := &rows[i]
		if p.Type != principal.TypeUser || p.UserIdentity == nil || !PrincipalVisible(ac, p) {
			continue
		}
		byEmail[strings.ToLower(p.UserIdentity.Email)] = p
	}

	out := MatchEmailsResponse{Matches: []PrincipalResponse{}, Unmatched: []string{}, Invalid: invalid}
	if out.Invalid == nil {
		out.Invalid = []string{}
	}
	for _, e := range emails {
		if p, ok := byEmail[e]; ok {
			out.Matches = append(out.Matches, forCaller(ac, fromEntity(p)))
		} else {
			out.Unmatched = append(out.Unmatched, e)
		}
	}
	return &apicommon.Out[MatchEmailsResponse]{Body: out}, nil
}

// --- list filtering / sorting / pagination helpers ---

func principalEmail(p *principal.Principal) string {
	if p.UserIdentity != nil {
		return p.UserIdentity.Email
	}
	return ""
}

func principalMatchesQuery(p *principal.Principal, qLower string) bool {
	return strings.Contains(strings.ToLower(p.Name), qLower) ||
		strings.Contains(strings.ToLower(principalEmail(p)), qLower)
}

// principalMatchesClient matches a principal's home client OR any granted
// client, so the Client filter surfaces both client-homed and partner users
// who can reach that client.
func principalMatchesClient(p *principal.Principal, clientID string) bool {
	if p.ClientID != nil && string(*p.ClientID) == clientID {
		return true
	}
	return slices.Contains(p.AssignedClients, ids.ClientID(clientID))
}

// principalHasAnyRole reports whether the principal holds at least one of the
// requested role names (OR semantics, matching the multi-select filter UX).
func principalHasAnyRole(p *principal.Principal, want []string) bool {
	for _, r := range p.Roles {
		if slices.Contains(want, r.Role.String()) {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	out := make([]string, 0)
	for part := range strings.SplitSeq(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// sortPrincipals orders in place. Unknown/empty field falls back to createdAt;
// any order other than "desc" is treated as ascending.
func sortPrincipals(ps []*principal.Principal, field, order string) {
	var cmpFn func(a, b *principal.Principal) int
	switch field {
	case "name":
		cmpFn = func(a, b *principal.Principal) int {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
	case "email":
		cmpFn = func(a, b *principal.Principal) int {
			return strings.Compare(strings.ToLower(principalEmail(a)), strings.ToLower(principalEmail(b)))
		}
	default: // "createdAt" and any unknown key
		cmpFn = func(a, b *principal.Principal) int { return a.CreatedAt.Compare(b.CreatedAt) }
	}
	slices.SortStableFunc(ps, cmpFn)
	if strings.EqualFold(order, "desc") {
		slices.Reverse(ps)
	}
}

// paginate returns the 0-based page slice. pageSize <= 0 returns everything.
func paginate(ps []*principal.Principal, page, pageSize int) []*principal.Principal {
	if pageSize <= 0 {
		return ps
	}
	if page < 0 {
		page = 0
	}
	start := min(page*pageSize, len(ps))
	end := min(start+pageSize, len(ps))
	return ps[start:end]
}
