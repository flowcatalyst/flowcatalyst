package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ReplaceInstallLinkCommand names the link to give a new token.
type ReplaceInstallLinkCommand struct {
	LinkID string `json:"linkId"`
}

// ReplaceInstallLink gives a link a new token: the old one stops working when
// this commits (the public routes look the token up by its hash, which is
// gone), and every copy of the old address with it. The link keeps its id and
// its generation counts up.
func ReplaceInstallLink(repo *appupdate.LinkRepository) usecaseop.TxOperation[ReplaceInstallLinkCommand, InstallLinkResult] {
	return usecaseop.TxOperation[ReplaceInstallLinkCommand, InstallLinkResult]{
		Name: "ReplaceAppInstallLink",
		Authorize: func(ctx context.Context, cmd ReplaceInstallLinkCommand) error {
			_, err := ReachInstallLink(ctx, repo, cmd.LinkID)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ReplaceInstallLinkCommand, ec usecase.ExecutionContext) (InstallLinkResult, error) {
			var zero InstallLinkResult
			l, err := lockReachedLink(ctx, s, repo, cmd.LinkID)
			if err != nil {
				return zero, err
			}
			token, hash, err := appupdate.NewInstallToken()
			if err != nil {
				return zero, usecase.Internal("RANDOM", "token generation failed", err)
			}
			l.TokenHash, l.Generation = hash, l.Generation+1
			l.UpdatedBy, l.UpdatedAt = ec.PrincipalID, time.Now().UTC()
			event := InstallLinkReplaced{eventBase: linkBase(ec, InstallLinkReplacedType, l), InstallLinkData: linkData(l)}
			if r := usecasepgx.CommitScoped(ctx, s, l, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return InstallLinkResult{Link: l, Token: token}, nil
		},
	}
}

// lockReachedLink reads a link the caller reaches, locking its row until the
// transaction ends (404 otherwise).
func lockReachedLink(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *appupdate.LinkRepository, id string) (*appupdate.InstallLink, error) {
	if _, err := ReachInstallLink(ctx, repo, id); err != nil {
		return nil, err
	}
	var l *appupdate.InstallLink
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		l, err = repo.FindLinkTx(ctx, tx, id)
		return err
	}); err != nil {
		return nil, usecase.Internal("REPO", "read install link failed", err)
	}
	if l == nil {
		return nil, installLinkNotFound(id)
	}
	return l, nil
}
