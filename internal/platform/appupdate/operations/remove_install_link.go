package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// RemoveInstallLinkCommand names the link to remove.
type RemoveInstallLinkCommand struct {
	LinkID string `json:"linkId"`
}

// RemoveInstallLink removes a link: its token stops working when this
// commits. The channel may get a new link later (a new address).
func RemoveInstallLink(repo *appupdate.LinkRepository) usecaseop.TxOperation[RemoveInstallLinkCommand, *appupdate.InstallLink] {
	return usecaseop.TxOperation[RemoveInstallLinkCommand, *appupdate.InstallLink]{
		Name: "RemoveAppInstallLink",
		Authorize: func(ctx context.Context, cmd RemoveInstallLinkCommand) error {
			_, err := ReachInstallLink(ctx, repo, cmd.LinkID)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RemoveInstallLinkCommand, ec usecase.ExecutionContext) (*appupdate.InstallLink, error) {
			l, err := lockReachedLink(ctx, s, repo, cmd.LinkID)
			if err != nil {
				return nil, err
			}
			event := InstallLinkRemoved{eventBase: linkBase(ec, InstallLinkRemovedType, l), InstallLinkData: linkData(l)}
			if r := usecasepgx.CommitDeleteScoped(ctx, s, l, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return nil, e
			}
			return l, nil
		},
	}
}
