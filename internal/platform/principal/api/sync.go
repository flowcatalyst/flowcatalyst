package api

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// SyncUserInput is one entry in the platform-level user-sync body. It mirrors
// the per-application sync entry but is NOT scoped to any application — users
// are global, matched by email.
type SyncUserInput struct {
	Email        string   `json:"email" doc:"User's email address (unique identifier for matching)"`
	Name         string   `json:"name" doc:"Display name"`
	Roles        []string `json:"roles,omitempty" doc:"Role names to assign (SDK_SYNC source; replaces this source's prior set)"`
	Active       *bool    `json:"active,omitempty" doc:"Whether the user is active (default true)"`
	PasswordHash *string  `json:"passwordHash,omitempty" doc:"Pre-hashed password (bcrypt/argon2i/argon2id), stored verbatim on a user the sync creates; migrated on first login. Never applied to an existing user (reported in passwordHashIgnored)."`
}

// SyncUsersRequest is the body for POST /api/principals/sync.
type SyncUsersRequest struct {
	Principals []SyncUserInput `json:"principals"`
}

// SyncUsersResponse is the rollup result of a user sync.
type SyncUsersResponse struct {
	Created      uint32   `json:"created"`
	Updated      uint32   `json:"updated"`
	Deleted      uint32   `json:"deleted"`
	SyncedEmails []string `json:"syncedEmails"`
	// PasswordHashIgnored names the entries whose passwordHash was not applied
	// because the user already existed (F13; Java's and Rust's field). Omitted
	// when empty.
	PasswordHashIgnored []string `json:"passwordHashIgnored,omitempty" doc:"Emails whose passwordHash was ignored because the user already exists (omitted when none)"`
}

// syncUsers handles POST /api/principals/sync — a declarative, application-less
// user upsert keyed on email. Unlike the SDK's per-application
// /api/applications/{appCode}/principals/sync, it needs no application: users
// are global, so "just syncing users" (optionally carrying a migrated password
// hash) doesn't require wrapping the call in an application. Pure upsert — it
// never strips roles from unlisted users.
func (s *State) syncUsers(ctx context.Context, in *apicommon.In[SyncUsersRequest]) (*apicommon.Out[SyncUsersResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncPrincipals(ac); err != nil {
		return nil, err
	}

	inputs := make([]operations.SyncPrincipalInput, 0, len(in.Body.Principals))
	for _, p := range in.Body.Principals {
		active := true
		if p.Active != nil {
			active = *p.Active
		}
		inputs = append(inputs, operations.SyncPrincipalInput{
			Email:        p.Email,
			Name:         p.Name,
			Roles:        p.Roles,
			Active:       active,
			PasswordHash: p.PasswordHash,
		})
	}

	// Read before the sync runs: the existing users whose hash it will not apply.
	ignored, err := operations.PasswordHashesIgnored(ctx, s.Repo, inputs)
	if err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	ev, err := usecaseop.Run(ctx, s.UoW, operations.SyncPrincipals(s.Repo, s.Roles, s.ClientConfigs),
		operations.SyncPrincipalsCommand{Principals: inputs}, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[SyncUsersResponse]{Body: SyncUsersResponse{
		Created:             ev.Created,
		Updated:             ev.Updated,
		Deleted:             ev.Deactivated,
		SyncedEmails:        ev.SyncedEmails,
		PasswordHashIgnored: ignored,
	}}, nil
}
