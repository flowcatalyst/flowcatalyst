package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// CreateInstallLinkCommand makes the install link of a phone app's channel.
type CreateInstallLinkCommand struct {
	ApplicationCode string `json:"applicationCode"`
	AppKey          string `json:"appKey"`
	Channel         string `json:"channel"`
}

// InstallLinkResult is a link and, after a create or a replace, its token:
// the only time the token exists outside the person's hands (the platform
// keeps its hash). It is in no event and no audit row.
type InstallLinkResult struct {
	Link  *appupdate.InstallLink
	Token string
}

// CreateInstallLink makes the link of a phone app's channel with a new token
// (409 INSTALL_LINK_EXISTS when the channel has one: replace it instead). The
// link works at once, and gives the newest binary of each variant the channel
// has, or will have.
func CreateInstallLink(repo *appupdate.LinkRepository, apps AppLookup) usecaseop.TxOperation[CreateInstallLinkCommand, InstallLinkResult] {
	return usecaseop.TxOperation[CreateInstallLinkCommand, InstallLinkResult]{
		Name: "CreateAppInstallLink",
		Validate: func(_ context.Context, cmd CreateInstallLinkCommand) error {
			switch {
			case !appupdate.ValidApplicationCode(cmd.ApplicationCode):
				return usecase.Validation("INVALID_APPLICATION_CODE", "applicationCode is not an application code")
			case !appupdate.ValidAppKey(cmd.AppKey):
				return usecase.Validation("INVALID_APP_KEY", "appKey must be 1-40 lower-case letters, digits and hyphens, starting with a letter or digit")
			case !appupdate.ValidChannel(cmd.Channel):
				return usecase.Validation("INVALID_CHANNEL", "channel must be 1-64 lower-case letters, digits and . _ -, starting with a letter or digit")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd CreateInstallLinkCommand) error {
			_, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CreateInstallLinkCommand, ec usecase.ExecutionContext) (InstallLinkResult, error) {
			var zero InstallLinkResult
			app, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			if err != nil {
				return zero, err
			}
			var existing *appupdate.InstallLink
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if err := repo.LockChannel(ctx, tx, string(app.ID), cmd.AppKey, cmd.Channel); err != nil {
					return err
				}
				existing, err = repo.FindLinkByChannelTx(ctx, tx, string(app.ID), cmd.AppKey, cmd.Channel)
				return err
			}); err != nil {
				return zero, usecase.Internal("REPO", "read install link failed", err)
			}
			if existing != nil {
				return zero, usecase.Conflict("INSTALL_LINK_EXISTS",
					"this channel has an install link ("+existing.ID+"); replace it to get a new address")
			}
			token, hash, err := appupdate.NewInstallToken()
			if err != nil {
				return zero, usecase.Internal("RANDOM", "token generation failed", err)
			}
			now := time.Now().UTC()
			l := &appupdate.InstallLink{
				ID: tsid.Generate(tsid.AppInstallLink), ApplicationID: string(app.ID), ApplicationCode: app.Code,
				AppKey: cmd.AppKey, Channel: cmd.Channel, TokenHash: hash, Generation: 1,
				CreatedBy: ec.PrincipalID, CreatedAt: now, UpdatedBy: ec.PrincipalID, UpdatedAt: now,
			}
			event := InstallLinkCreated{eventBase: linkBase(ec, InstallLinkCreatedType, l), InstallLinkData: linkData(l)}
			if r := usecasepgx.CommitScoped(ctx, s, l, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return InstallLinkResult{Link: l, Token: token}, nil
		},
	}
}

func linkBase(ec usecase.ExecutionContext, eventType string, l *appupdate.InstallLink) eventBase {
	return baseFor(ec, eventType, installLinkSubjectPrefix+l.ID, l.ApplicationID, l.AppKey)
}

func linkData(l *appupdate.InstallLink) InstallLinkData {
	return InstallLinkData{
		LinkID: l.ID, ApplicationID: l.ApplicationID, ApplicationCode: l.ApplicationCode,
		AppKey: l.AppKey, Channel: l.Channel, Generation: l.Generation,
	}
}
