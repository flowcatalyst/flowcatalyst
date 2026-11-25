package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	cpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Publishing credentials to Parameter Store in the console
// (docs/credential-publishing.md section 8): the create dialogs' checkbox,
// the "Also published to" line under a revealed value, and the drawer's
// "Parameter Store" section. Everything is hidden while publishing is not
// configured (a nil CredentialPublishing, or one whose Config is not Enabled).

// CredentialPublishing is what the service account, OAuth client and
// application pages need of credential publishing. CredentialPublishOps is the
// production implementation.
type CredentialPublishing interface {
	Config() credentialpublish.Config
	Status(ctx context.Context, owner credentialpublish.Owner) (*credentialpublish.Publication, []credentialpublish.Item, error)
	// LinkedServiceAccount is the service account row id an OAuth client
	// publishes with, "" when it is stand-alone.
	LinkedServiceAccount(ctx context.Context, oauthClientRowID string) (string, error)
	Enable(ctx context.Context, owner credentialpublish.Owner, basePath *string) error
	Disable(ctx context.Context, owner credentialpublish.Owner) error
	Retry(ctx context.Context, owner credentialpublish.Owner) error
	// RotateClientSecret is the OAuth client secret rotation (with its default
	// overlap): "Rotate and publish" runs it for a service account's client.
	RotateClientSecret(ctx context.Context, oauthClientRowID string) (RotatedOAuthSecret, error)
}

// publishOn reports whether publishing is configured.
func publishOn(p CredentialPublishing) bool { return p != nil && p.Config().Enabled() }

// CredentialPublishOps runs the credential publishing use cases through the
// unit of work, exactly as the service account and OAuth client APIs do.
type CredentialPublishOps struct {
	Cfg          credentialpublish.Config
	Repo         *credentialpublish.Repository
	Accounts     *serviceaccount.Repository
	Principals   *principal.Repository
	OAuthClients *platformauth.OAuthClientRepo
	Apps         *application.Repository
	// OAuth runs the secret rotation behind "Rotate and publish".
	OAuth OAuthClientCommands
	UoW   *usecasepgx.UnitOfWork
}

// Config is the deployment's publishing setting.
func (o CredentialPublishOps) Config() credentialpublish.Config { return o.Cfg }

// Status reads the owner's publication and items.
func (o CredentialPublishOps) Status(ctx context.Context, owner credentialpublish.Owner) (*credentialpublish.Publication, []credentialpublish.Item, error) {
	return o.Repo.Status(ctx, owner)
}

// LinkedServiceAccount reads the account an OAuth client belongs to.
func (o CredentialPublishOps) LinkedServiceAccount(ctx context.Context, id string) (string, error) {
	return o.Repo.LinkedServiceAccount(ctx, id)
}

// Enable runs EnableCredentialPublishing.
func (o CredentialPublishOps) Enable(ctx context.Context, owner credentialpublish.Owner, basePath *string) error {
	_, err := usecaseop.Run(ctx, o.UoW, cpops.EnableCredentialPublishing(o.Cfg, o.Accounts, o.Principals, o.OAuthClients, o.Apps),
		cpops.EnableCommand{OwnerType: owner.Type, OwnerID: owner.ID, BasePath: basePath}, auth.NewExecutionContext(ctx))
	return err
}

// Disable runs DisableCredentialPublishing.
func (o CredentialPublishOps) Disable(ctx context.Context, owner credentialpublish.Owner) error {
	_, err := usecaseop.Run(ctx, o.UoW, cpops.DisableCredentialPublishing(o.Accounts, o.OAuthClients, o.Repo),
		cpops.DisableCommand{OwnerType: owner.Type, OwnerID: owner.ID}, auth.NewExecutionContext(ctx))
	return err
}

// Retry runs RetryCredentialPublishing.
func (o CredentialPublishOps) Retry(ctx context.Context, owner credentialpublish.Owner) error {
	return cpops.RetryCredentialPublishing(ctx, o.Repo, owner)
}

// RotateClientSecret runs the OAuth client secret rotation with its default
// overlap.
func (o CredentialPublishOps) RotateClientSecret(ctx context.Context, id string) (RotatedOAuthSecret, error) {
	return o.OAuth.Rotate(ctx, id, nil)
}

// Flash keys of the publishing actions, shared by the pages.
const (
	flashPubEnabled  = "credential-publishing-enabled"
	flashPubDisabled = "credential-publishing-disabled"
	flashPubRetry    = "credential-publishing-retry"
)

func init() {
	maps.Copy(FlashMessages, map[string]kit.FlashMessageProps{
		flashPubEnabled:  success("Credential publishing switched on"),
		flashPubDisabled: success("Credential publishing switched off. The parameters are being removed"),
		flashPubRetry:    success("Retrying the waiting parameters now"),
	})
}

// pubPlaceholder stands in for a value the browser (or the page, later) fills
// into a rendered template: a valid path segment, so BasePath accepts it.
const pubPlaceholder = "fcph"

// pubFieldCodes are the refusals that belong to the create dialogs' fields.
var pubFieldCodes = map[string]string{
	credentialpublish.CodePathInvalid:     "publishBasePath",
	credentialpublish.CodeUnavailable:     "publish",
	credentialpublish.CodeNotConfidential: "publish",
	credentialpublish.CodeLinked:          "publish",
}

// applyPublishErr records a use-case error on a form: a publishing refusal on
// the checkbox or the path field, anything else as the form applies it. The
// path message loses the operation's "publishBasePath: " prefix, which names
// the wire field rather than the control.
func applyPublishErr(e *formErrors, r *http.Request, err error, other map[string]string) {
	if ue, ok := errors.AsType[*usecase.Error](err); ok {
		if key, mapped := pubFieldCodes[ue.Code]; mapped {
			e.field(key, sentence(strings.TrimPrefix(ue.Message, "publishBasePath: ")))
			if s := ue.HTTPStatus(); s >= 400 && s < 500 {
				e.Status = s
			}
			return
		}
	}
	e.apply(r, err, other)
}

// ── the create dialogs' block ──────────────────────────────────────────

// pubBlock is the "Publish credentials to Parameter Store" control of a
// create dialog: the checkbox, the path the parameters will get and a custom
// path behind a disclosure.
type pubBlock struct {
	// Show is false while publishing is not configured: nothing renders.
	Show bool
	// ID is the checkbox's id; the block, the path input and the errors
	// derive from it.
	ID string
	// On is the checkbox, Path the custom path typed.
	On   bool
	Path string
	// Err and PathErr are the checkbox's and the path field's errors.
	Err, PathErr string
	// Prefix is what every path must lie under (the custom path's help).
	Prefix string
	// Preview is the path the parameters get.
	Preview pubPreview
	// PathSignal binds the custom path input, for a preview that follows it;
	// CodeSignal is the signal holding the service account code the live
	// preview substitutes. Both are empty for a static preview.
	PathSignal, CodeSignal string
	// ShowWhen is a Datastar expression that shows the block (an OAuth
	// client's type); Visible is its state when the block is drawn.
	ShowWhen string
	Visible  bool
}

// pubPreview is a rendered path template: Tmpl holds pubPlaceholder where the
// owner's name goes, Static the text for a preview that does not follow the
// code, Err why there is no default path.
type pubPreview struct {
	Tmpl, Static, Err string
}

func (b pubBlock) live() bool { return b.PathSignal != "" && b.CodeSignal != "" }

func (b pubBlock) hidden() string { return hiddenUnless(b.ShowWhen == "" || b.Visible) }

func (b pubBlock) pathID() string { return b.ID + "-path" }

func (b pubBlock) pathDescribedBy() string {
	if b.PathErr != "" {
		return b.pathID() + "-err"
	}
	return ""
}

// pathAttrs are the custom path input's attributes: bound to its signal when
// the preview follows it.
func (b pubBlock) pathAttrs() templ.Attributes {
	a := templ.Attributes{"autocomplete": "off", "autocapitalize": "off", "spellcheck": "false"}
	if b.PathSignal != "" {
		a["data-bind:"+kebab(b.PathSignal)] = true
	}
	return a
}

// focusID is the control an error lands on.
func (b pubBlock) focusID() string {
	if b.Err == "" && b.PathErr != "" {
		return b.pathID()
	}
	return b.ID
}

func (b pubBlock) signals() string {
	if b.PathSignal == "" {
		return "{}"
	}
	j, _ := json.Marshal(map[string]any{b.PathSignal: b.Path})
	return string(j)
}

// previewExpr is the live preview: the custom path typed, else the template
// with the code typed so far. Every value in it is a constant of the page (the
// template holds only path characters) or a signal.
func (b pubBlock) previewExpr() string {
	code := "($" + b.CodeSignal + " || '<code>').replaceAll(':', '.')"
	return "$" + b.PathSignal + ".trim() || '" + b.Preview.Tmpl + "'.split('" + pubPlaceholder + "').join(" + code + ")"
}

// pubPreviewFor renders the template of the owner type with the placeholder
// where the owner's name goes. display is what a static preview shows there.
func pubPreviewFor(cfg credentialpublish.Config, t credentialpublish.OwnerType, v credentialpublish.Vars, display string) pubPreview {
	base, err := cfg.BasePath(t, v)
	if err != nil {
		return pubPreview{Err: sentence(err.Error())}
	}
	return pubPreview{Tmpl: base, Static: strings.ReplaceAll(base, pubPlaceholder, display)}
}

// newPubBlock is the block of a create dialog: nothing unless publishing is
// configured; the checkbox starts as the deployment's default says.
func newPubBlock(p CredentialPublishing, id string) pubBlock {
	if !publishOn(p) {
		return pubBlock{}
	}
	cfg := p.Config()
	return pubBlock{Show: true, ID: id, On: cfg.DefaultOn, Prefix: cfg.Prefix, Visible: true}
}

// posted reads what the dialog posted into the block.
func (b *pubBlock) posted(r *http.Request) {
	b.On = r.PostFormValue("publishCredentials") == "true"
	b.Path = strings.TrimSpace(r.PostFormValue("publishBasePath"))
}

// command returns the create command's two fields from what was posted: the
// checkbox, and the custom path when one was typed. Call it only when
// publishing is configured: without the control, the use case's default
// stands.
func (b pubBlock) command() (publish *bool, basePath *string) {
	on := b.On
	return &on, optionalText(b.Path)
}

// ── the drawer's section ───────────────────────────────────────────────

// pubOrder is the order credentials are listed in.
var pubOrder = []string{credentialpublish.CredClientID, credentialpublish.CredClientSecret, credentialpublish.CredAuthToken, credentialpublish.CredSigningSecret}

func pubLabel(cred string) string {
	switch cred {
	case credentialpublish.CredClientID:
		return "Client ID"
	case credentialpublish.CredClientSecret:
		return "Client secret"
	case credentialpublish.CredAuthToken:
		return "Auth token"
	case credentialpublish.CredSigningSecret:
		return "Signing secret"
	}
	return cred
}

// pubRow kinds.
const (
	pubPublished     = "published"
	pubPublishing    = "publishing"
	pubFailing       = "failing"
	pubRemoving      = "removing"
	pubUnpublishable = "unpublishable"
)

// pubRow is one credential's line in the section.
type pubRow struct {
	Cred, Label, Path, Reference string
	Kind                         string
	// At is when it was published, or when the next attempt is.
	At  time.Time
	Err string
}

// pubSection is the drawer's "Parameter Store" section.
type pubSection struct {
	// Show is false while publishing is not configured (and for a client that
	// has nothing to publish): the section does not render.
	Show bool
	// Err says the state could not be read.
	Err string
	// LinkedName and LinkedHref: a client published with its service account.
	LinkedName, LinkedHref string
	On                     bool
	BasePath               string
	Rows                   []pubRow
	// CanChange: the caller may switch publishing on and off and retry.
	// CanRotate: and may rotate the client secret ("Rotate and publish").
	CanChange, CanRotate bool
	// Base is the owner's action base (the page's URL of the record);
	// RotateDialog and RotateAction run the rotation behind "Rotate and
	// publish" (an empty RotateAction offers none).
	Base                       string
	RotateDialog, RotateAction string
	// DrawerSig is the page's drawer signal (posted so the drawer refreshes).
	DrawerSig string
	Failing   bool
	StopPaths []string
}

func (s pubSection) stopDialogID() string { return "pub-stop" }

// pubRows turns the items into rows, in display order, and reports whether any
// carries an error and the paths that Stop publishing deletes (those the
// platform wrote or is writing: the disable operation's own set).
func pubRows(items []credentialpublish.Item) (rows []pubRow, failing bool, stop []string) {
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b credentialpublish.Item) int {
		return cmp.Compare(slices.Index(pubOrder, a.Credential), slices.Index(pubOrder, b.Credential))
	})
	for _, it := range sorted {
		row := pubRow{Cred: it.Credential, Label: pubLabel(it.Credential), Path: it.Path, Reference: credentialpublish.Reference(it.Path), Err: it.LastError}
		isFailing := it.Attempts > 0 && it.LastError != ""
		switch {
		case it.State == credentialpublish.StateNotPublishable:
			row.Kind = pubUnpublishable
		case isFailing && (it.State == credentialpublish.StatePendingPut || it.State == credentialpublish.StatePendingDelete):
			row.Kind = pubFailing
			if it.NextAttemptAt != nil {
				row.At = *it.NextAttemptAt
			}
		case it.State == credentialpublish.StatePendingDelete:
			row.Kind = pubRemoving
		case it.State == credentialpublish.StatePendingPut:
			row.Kind = pubPublishing
		default:
			row.Kind = pubPublished
			if it.PublishedAt != nil {
				row.At = *it.PublishedAt
			}
		}
		if it.LastError != "" {
			failing = true
		}
		if it.State != credentialpublish.StateNotPublishable {
			stop = append(stop, it.Path)
		}
		rows = append(rows, row)
	}
	return rows, failing, stop
}

// readSection fills a section from the owner's publication.
func readSection(ctx context.Context, p CredentialPublishing, owner credentialpublish.Owner, s pubSection) pubSection {
	if !publishOn(p) {
		return pubSection{}
	}
	s.Show = true
	pub, items, err := p.Status(ctx, owner)
	if err != nil {
		slog.WarnContext(ctx, "ui: credential publishing status unavailable", "owner", owner.ID, "err", err)
		s.Err = "The publishing state could not be read."
		return s
	}
	if pub != nil {
		s.On, s.BasePath = true, pub.BasePath
	}
	s.Rows, s.Failing, s.StopPaths = pubRows(items)
	return s
}

// pubPaths is the paths that the owner's credentials were queued to, by
// credential: the items waiting to be written or written. Empty while
// publishing is not configured or the read fails (the reveal then simply does
// not say it).
func pubPaths(ctx context.Context, p CredentialPublishing, owner credentialpublish.Owner) map[string]string {
	if !publishOn(p) {
		return nil
	}
	_, items, err := p.Status(ctx, owner)
	if err != nil {
		slog.WarnContext(ctx, "ui: credential publishing status unavailable", "owner", owner.ID, "err", err)
		return nil
	}
	out := map[string]string{}
	for _, it := range items {
		if it.State == credentialpublish.StatePendingPut || it.State == credentialpublish.StatePublished {
			out[it.Credential] = it.Path
		}
	}
	return out
}

// ── the enable dialog ──────────────────────────────────────────────────

const (
	pubFormDialog = "pub-form"
	pubFormSignal = "pubForm"
)

// pubEnableForm is the "Publish credentials" dialog of a record's drawer: the
// default path and an optional custom one.
type pubEnableForm struct {
	Action, DrawerSig string
	Subtitle          string
	// Default is the path the template gives; DefaultErr why there is none.
	Default, DefaultErr string
	Prefix              string
	Path                string
	Errs                formErrors
	Open                bool
}

func (f pubEnableForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: pubFormDialog, Signal: pubFormSignal, Width: kit.WidthWide, Action: f.Action, Open: f.Open, Error: f.Errs.Form,
		Title: "Publish credentials", Description: f.Subtitle, SubmitLabel: "Publish credentials",
		Hidden: []kit.HiddenField{drawerField(f.DrawerSig)},
	}
}

func (f pubEnableForm) fieldProps() kit.FieldProps {
	return kit.FieldProps{
		Label: "Custom path", For: pubFormDialog + "-path", Error: f.Errs.Fields["publishBasePath"],
		Description: "Optional. Replaces the default path. It must start and end with '/' and lie under " + f.Prefix,
		Attrs:       templ.Attributes{"id": pubFormDialog + "-path-field"},
	}
}

func (f pubEnableForm) describedBy() string {
	if f.Errs.Fields["publishBasePath"] != "" {
		return pubFormDialog + "-path-err"
	}
	return ""
}

// pubCopyExpr copies the text of the clicked button's data-copy attribute (a
// plain attribute, so a path never enters the expression).
const pubCopyExpr = "navigator.clipboard.writeText(el.dataset.copy)"
