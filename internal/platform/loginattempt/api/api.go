// Package api wires the HTTP routes for the login-attempt subdomain via huma.
package api

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *loginattempt.Repository
}

const tag = "login-attempts"

// Register mounts the login-attempt endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listLoginAttempts", "/api/login-attempts", "List login attempts (cursor-paginated)", s.list)
}

// LoginAttemptResponse is the wire shape for a single attempt.
type LoginAttemptResponse struct {
	ID            string          `json:"id"`
	AttemptType   string          `json:"attemptType"`
	Outcome       string          `json:"outcome"`
	FailureReason *string         `json:"failureReason"`
	Identifier    string          `json:"identifier"`
	PrincipalID   *string         `json:"principalId"`
	IPAddress     *string         `json:"ipAddress"`
	UserAgent     *string         `json:"userAgent"`
	AttemptedAt   httpcompat.Time `json:"attemptedAt"`
}

// LoginAttemptListResponse is the cursor-paginated list envelope.
type LoginAttemptListResponse struct {
	Items      []LoginAttemptResponse `json:"items"`
	HasMore    bool                   `json:"hasMore"`
	NextCursor *string                `json:"nextCursor,omitempty"`
}

func attemptFromEntity(a *loginattempt.LoginAttempt) LoginAttemptResponse {
	identifier := ""
	if a.Identifier != nil {
		identifier = *a.Identifier
	}
	return LoginAttemptResponse{
		ID:            a.ID,
		AttemptType:   string(a.AttemptType),
		Outcome:       string(a.Outcome),
		FailureReason: a.FailureReason,
		Identifier:    identifier,
		PrincipalID:   a.PrincipalID,
		IPAddress:     a.IPAddress,
		UserAgent:     a.UserAgent,
		AttemptedAt:   jsontime.New(a.AttemptedAt),
	}
}

type listInput struct {
	AttemptType string `query:"attemptType"`
	Outcome     string `query:"outcome"`
	Identifier  string `query:"identifier"`
	PrincipalID string `query:"principalId"`
	DateFrom    string `query:"dateFrom"`
	DateTo      string `query:"dateTo"`
	After       string `query:"after"`
	PageSize    int    `query:"pageSize"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[LoginAttemptListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}

	size := in.PageSize
	if size <= 0 || size > 200 {
		size = 50
	}
	params := loginattempt.ListParams{
		Limit:       size + 1,
		AttemptType: apicommon.OptStr(in.AttemptType),
		Outcome:     apicommon.OptStr(in.Outcome),
		Identifier:  apicommon.OptStr(in.Identifier),
		PrincipalID: apicommon.OptStr(in.PrincipalID),
	}
	if t, ok := parseTime(in.DateFrom); ok {
		params.DateFrom = &t
	}
	if t, ok := parseTime(in.DateTo); ok {
		params.DateTo = &t
	}
	if at, id, ok := decodeCursor(in.After); ok {
		params.AfterTime = &at
		params.AfterID = &id
	}

	rows, err := s.Repo.FindPage(ctx, params)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_page failed", err)
	}
	hasMore := len(rows) > size
	if hasMore {
		rows = rows[:size]
	}
	items := apicommon.MapSlice(rows, attemptFromEntity)
	var next *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		c := encodeCursor(last.AttemptedAt, last.ID)
		next = &c
	}
	return &apicommon.Out[LoginAttemptListResponse]{Body: LoginAttemptListResponse{Items: items, HasMore: hasMore, NextCursor: next}}, nil
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(s string) (time.Time, string, bool) {
	if s == "" {
		return time.Time{}, "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", false
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", false
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", false
	}
	return t, parts[1], true
}
