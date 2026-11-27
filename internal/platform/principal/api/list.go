package api

import (
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// listInput carries the filter / sort / pagination query params for
// GET /api/principals. Every field is optional; an absent param means
// "no filter" (or the documented default).
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

	rows, total, err := s.Repo.FindFiltered(ctx, toListFilter(ac, in))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_filtered failed", err)
	}

	out := make([]PrincipalResponse, 0, len(rows))
	for i := range rows {
		p := &rows[i]
		// Belt-and-suspenders check: SQL visibility already enforces this rule.
		if !PrincipalVisible(ac, p) {
			continue
		}
		out = append(out, forCaller(ac, fromEntity(p)))
	}
	return &apicommon.Out[PrincipalListResponse]{Body: PrincipalListResponse{Principals: out, Total: total}}, nil
}

// matchEmails resolves a pasted email list against the users the caller may
// see, pushing both the email lookup and the visibility predicate down to SQL.
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

	byEmail := map[string]*principal.Principal{}
	if len(emails) > 0 {
		rows, err := s.Repo.FindUsersByEmails(ctx, emails, visibilityFor(ac))
		if err != nil {
			return nil, usecase.Internal("REPO", "find_users_by_emails failed", err)
		}
		byEmail = make(map[string]*principal.Principal, len(rows))
		for i := range rows {
			p := &rows[i]
			if p.Type != principal.TypeUser || p.UserIdentity == nil || !PrincipalVisible(ac, p) {
				continue
			}
			byEmail[strings.ToLower(p.UserIdentity.Email)] = p
		}
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

func visibilityFor(ac *auth.AuthContext) principal.VisibilityScope {
	if ac == nil {
		return principal.VisibilityScope{}
	}
	if ac.IsAnchor() {
		return principal.VisibilityScope{Anchor: true}
	}
	return principal.VisibilityScope{
		Anchor:            false,
		AccessibleClients: slices.Clone(ac.Clients),
	}
}

func toListFilter(ac *auth.AuthContext, in *listInput) principal.ListFilter {
	var active *bool
	switch in.Active {
	case "true":
		v := true
		active = &v
	case "false":
		v := false
		active = &v
	}
	return principal.ListFilter{
		Visibility: visibilityFor(ac),
		Type:       strings.ToUpper(strings.TrimSpace(in.Type)),
		Active:     active,
		ClientID:   strings.TrimSpace(in.ClientID),
		Query:      strings.ToLower(strings.TrimSpace(in.Q)),
		Roles:      splitCSV(in.Roles),
		SortField:  strings.TrimSpace(in.SortField),
		SortDesc:   strings.EqualFold(strings.TrimSpace(in.SortOrder), "desc"),
		Page:       in.Page,
		PageSize:   in.PageSize,
	}
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
