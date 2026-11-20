package fieldidentity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Error codes of the photo operations (docs/field-worker-photo-spec.md sections 4 and 5).
const (
	// CodePhotosDisabledForClient: the client's policy has photosEnabled false
	// (409).
	CodePhotosDisabledForClient = "PHOTOS_DISABLED_FOR_CLIENT"
	// CodeOwnPhotoNotAllowed: a worker acting for themselves, and the client's
	// policy has workerMaySetOwnPhoto false (403).
	CodeOwnPhotoNotAllowed = "OWN_PHOTO_NOT_ALLOWED"
)

// errPhotosOff is the locked re-check's answer inside the set transaction.
var errPhotosOff = errors.New("field photos are off for the client")

// PhotoActorOf is who a caller is, for the record of a photo: a service
// account's token is SERVICE_ACCOUNT, any other platform caller ADMIN.
func PhotoActorOf(ac *auth.AuthContext) PhotoActor {
	if ac != nil && ac.PrincipalType == auth.PrincipalTypeService {
		return PhotoByServiceAccount
	}
	return PhotoByAdmin
}

// SetPhotoCommand sets or replaces a worker's photo with renditions already
// made by [imageproc]. Image carries the bytes and is never serialised: the
// command is what the audit row records, and an audit row holds no image data.
type SetPhotoCommand struct {
	WorkerRef
	// Actor is who sets it; ActorID the principal id (or, for the worker
	// themselves, the worker id).
	Actor   PhotoActor `json:"actor"`
	ActorID string     `json:"actorId"`
	// Version, Source and the sizes describe Image, for the audit row.
	Version     string `json:"version"`
	Source      string `json:"source"`
	FullWidth   int    `json:"fullWidth"`
	FullHeight  int    `json:"fullHeight"`
	ThumbWidth  int    `json:"thumbWidth"`
	ThumbHeight int    `json:"thumbHeight"`

	Image *imageproc.Photo `json:"-"`
}

// NewSetPhotoCommand builds the command for a processed photo.
func NewSetPhotoCommand(ref WorkerRef, actor PhotoActor, actorID string, p *imageproc.Photo) SetPhotoCommand {
	return SetPhotoCommand{
		WorkerRef: ref, Actor: actor, ActorID: actorID, Version: p.Version, Source: string(p.Source),
		FullWidth: p.Full.Width, FullHeight: p.Full.Height, ThumbWidth: p.Thumb.Width, ThumbHeight: p.Thumb.Height,
		Image: p,
	}
}

// SetWorkerPhoto stores the worker's photo, replacing any earlier one. It is
// refused with 409 PHOTOS_DISABLED_FOR_CLIENT when the client has photos off. It
// is the administrator's and the service account's; the worker's own is
// SetOwnPhoto.
func (o *Ops) SetWorkerPhoto() usecaseop.TxOperation[SetPhotoCommand, Event] {
	return usecaseop.TxOperation[SetPhotoCommand, Event]{
		Name: "SetFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd SetPhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if !cmd.Actor.Valid() || cmd.Actor == PhotoByWorker {
				return usecase.Validation("PHOTO_ACTOR_INVALID", "the actor of an administrator's photo change is ADMIN or SERVICE_ACCOUNT; a worker's own is SetOwnPhoto")
			}
			if cmd.Image == nil || len(cmd.Image.Full.JPEG) == 0 || len(cmd.Image.Thumb.JPEG) == 0 {
				return usecase.Validation("PHOTO_REQUIRED", "a processed photo is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[SetPhotoCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd SetPhotoCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			pol, err := o.R.Policies.Effective(ctx, ident.ClientID)
			if err != nil {
				return Event{}, repoErr("find_field_policy", err)
			}
			if !pol.PhotosEnabled {
				return Event{}, usecase.Conflict(CodePhotosDisabledForClient, "this client has switched field-worker photos off")
			}
			return storePhoto(ctx, s, ec, ident, cmd.Image, cmd.Actor, cmd.ActorID, cmd)
		},
	}
}

// RemovePhotoCommand removes a worker's photo.
type RemovePhotoCommand struct {
	WorkerRef
	Actor   PhotoActor `json:"actor"`
	ActorID string     `json:"actorId"`
}

// RemoveWorkerPhoto deletes the worker's photo. A worker with none is a no-op
// (no event), and an administrator may remove a hidden photo of a client that
// has photos off. The worker's own is RemoveOwnPhoto.
func (o *Ops) RemoveWorkerPhoto() usecaseop.TxOperation[RemovePhotoCommand, Event] {
	return usecaseop.TxOperation[RemovePhotoCommand, Event]{
		Name: "RemoveFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd RemovePhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if !cmd.Actor.Valid() || cmd.Actor == PhotoByWorker {
				return usecase.Validation("PHOTO_ACTOR_INVALID", "the actor of an administrator's photo change is ADMIN or SERVICE_ACCOUNT; a worker's own is SetOwnPhoto")
			}
			return nil
		},
		Authorize: usecaseop.Public[RemovePhotoCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RemovePhotoCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			return dropPhoto(ctx, s, ec, ident, cmd.Actor, cmd)
		},
	}
}

// storePhoto writes the worker's photo and version in the open transaction and
// records the event; command is what the audit row records. It re-reads the
// client's switch under a share lock: a policy change that switches photos off
// holds the row until it commits, so a photo cannot slip in behind it.
func storePhoto(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext, ident *Identity,
	img *imageproc.Photo, actor PhotoActor, actorID string, command any,
) (Event, error) {
	now := time.Now().UTC()
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		on := true
		if err := tx.QueryRow(ctx,
			`SELECT photos_enabled FROM field_policies WHERE client_id = $1 FOR SHARE`, ident.ClientID).Scan(&on); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !on {
			return errPhotosOff
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO field_worker_photos
			     (identity_id, client_id, version, content_type, full_bytes, full_width, full_height,
			      thumb_bytes, thumb_width, thumb_height, source_type, set_by_type, set_by, created_at, updated_at)
			 VALUES ($1,$2,$3,'image/jpeg',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
			 ON CONFLICT (identity_id) DO UPDATE SET
			     client_id = EXCLUDED.client_id, version = EXCLUDED.version,
			     full_bytes = EXCLUDED.full_bytes, full_width = EXCLUDED.full_width, full_height = EXCLUDED.full_height,
			     thumb_bytes = EXCLUDED.thumb_bytes, thumb_width = EXCLUDED.thumb_width, thumb_height = EXCLUDED.thumb_height,
			     source_type = EXCLUDED.source_type, set_by_type = EXCLUDED.set_by_type, set_by = EXCLUDED.set_by,
			     updated_at = EXCLUDED.updated_at`,
			ident.ID, ident.ClientID, img.Version, img.Full.JPEG, img.Full.Width, img.Full.Height,
			img.Thumb.JPEG, img.Thumb.Width, img.Thumb.Height, string(img.Source), string(actor), actorID, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE field_identities SET photo_version = $2, photo_updated_at = $3 WHERE id = $1`,
			ident.ID, img.Version, now)
		return err
	}); errors.Is(err, errPhotosOff) {
		return Event{}, usecase.Conflict(CodePhotosDisabledForClient, "this client has switched field-worker photos off")
	} else if err != nil {
		return Event{}, repoErr("set_field_worker_photo", err)
	}
	ev := NewEvent(ec, WorkerPhotoSetType, "field-worker", ident.ID, workerData(ident, map[string]any{
		"version": img.Version, "source": string(img.Source), "setByType": string(actor),
	}))
	return usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, command))
}

// dropPhoto removes the worker's photo and version in the open transaction and
// records the event when there was one (none: an empty event).
func dropPhoto(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext, ident *Identity,
	actor PhotoActor, command any,
) (Event, error) {
	removed := false
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM field_worker_photos WHERE identity_id = $1`, ident.ID)
		if err != nil {
			return err
		}
		removed = tag.RowsAffected() > 0
		_, err = tx.Exec(ctx,
			`UPDATE field_identities SET photo_version = NULL, photo_updated_at = NULL
			 WHERE id = $1 AND photo_version IS NOT NULL`, ident.ID)
		return err
	}); err != nil {
		return Event{}, repoErr("remove_field_worker_photo", err)
	}
	if !removed {
		return Event{}, nil
	}
	ev := NewEvent(ec, WorkerPhotoRemovedType, "field-worker", ident.ID, workerData(ident, map[string]any{
		"removedByType": string(actor),
	}))
	return usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, command))
}

// DeleteClientPhotosCommand deletes every stored photo of a client's field
// workers.
type DeleteClientPhotosCommand struct {
	ClientID string `json:"clientId"`
}

// DeleteClientPhotos deletes every photo of the client's workers and clears
// their versions, in one transaction, whatever the client's photo setting says
// (switching photos off only hides them). It is client-wide, so an
// application-confined caller may not call it. A repeat, or a client with no
// photos, removes nothing and writes no event and no audit row (the returned
// event is empty); otherwise one event carries the count in Data["removed"]
// (never image data).
func (o *Ops) DeleteClientPhotos() usecaseop.TxOperation[DeleteClientPhotosCommand, Event] {
	return usecaseop.TxOperation[DeleteClientPhotosCommand, Event]{
		Name: "DeleteClientFieldPhotos",
		Validate: func(_ context.Context, cmd DeleteClientPhotosCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd DeleteClientPhotosCommand) error {
			ac := auth.FromContext(ctx)
			if err := auth.CheckScopeAccess(ac, &cmd.ClientID); err != nil {
				return err
			}
			if ac.IsApplicationScoped() {
				return usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot delete a client's photos; remove a worker's photo on its own")
			}
			return nil
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteClientPhotosCommand, ec usecase.ExecutionContext) (Event, error) {
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return Event{}, err
			}
			removed := 0
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `DELETE FROM field_worker_photos WHERE client_id = $1`, cmd.ClientID)
				if err != nil {
					return err
				}
				removed = int(tag.RowsAffected())
				_, err = tx.Exec(ctx,
					`UPDATE field_identities SET photo_version = NULL, photo_updated_at = NULL
					 WHERE client_id = $1 AND photo_version IS NOT NULL`, cmd.ClientID)
				return err
			}); err != nil {
				return Event{}, repoErr("delete_client_field_photos", err)
			}
			if removed == 0 {
				return Event{}, nil
			}
			ev := NewEvent(ec, ClientPhotosDeletedType, "field-policy", cmd.ClientID, map[string]any{
				"clientId": cmd.ClientID, "removed": removed,
			})
			return usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd))
		},
	}
}

// PhotosRemoved is how many photos a DeleteClientPhotos run removed, from the
// event it returned (0 for the empty event of a run that removed none).
func PhotosRemoved(ev Event) int {
	n, _ := ev.Data["removed"].(int)
	return n
}

// ── a worker's own photo (the device protocol) ───────────────────────────

// SetOwnPhotoCommand is a signed-in worker setting their own photo from their
// device: ID and ClientID are the access token's subject and client, never a
// request's. Image is never serialised.
type SetOwnPhotoCommand struct {
	WorkerRef
	Version     string           `json:"version"`
	Source      string           `json:"source"`
	FullWidth   int              `json:"fullWidth"`
	FullHeight  int              `json:"fullHeight"`
	ThumbWidth  int              `json:"thumbWidth"`
	ThumbHeight int              `json:"thumbHeight"`
	Image       *imageproc.Photo `json:"-"`
}

// NewSetOwnPhotoCommand builds the command for a processed photo.
func NewSetOwnPhotoCommand(ref WorkerRef, p *imageproc.Photo) SetOwnPhotoCommand {
	return SetOwnPhotoCommand{
		WorkerRef: ref, Version: p.Version, Source: string(p.Source), FullWidth: p.Full.Width, FullHeight: p.Full.Height,
		ThumbWidth: p.Thumb.Width, ThumbHeight: p.Thumb.Height, Image: p,
	}
}

// RemoveOwnPhotoCommand is a signed-in worker removing their own photo.
type RemoveOwnPhotoCommand struct{ WorkerRef }

// loadSelf loads the worker a device token names: it must exist in the client
// and be ACTIVE. Nothing else is checked here: the protocol authenticated the
// token, and the client's policy is checked by the caller.
func (o *Ops) loadSelf(ctx context.Context, ref WorkerRef) (*Identity, error) {
	ident, err := o.R.Identities.FindByID(ctx, ref.ID)
	if err != nil {
		return nil, repoErr("find_field_worker", err)
	}
	if ident == nil || ident.ClientID != ref.ClientID {
		return nil, httperror.NotFound("FieldWorker", ref.ID)
	}
	if ident.Status != StatusActive {
		return nil, usecase.Conflict(CodeWorkerUnavailable, "the worker is not active")
	}
	return ident, nil
}

// ownPhotoPolicy is the client's rule for a worker's own photo: photos on, and
// the worker allowed to set (and remove) their own.
func (o *Ops) ownPhotoPolicy(ctx context.Context, clientID string) error {
	pol, err := o.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return repoErr("find_field_policy", err)
	}
	if !pol.PhotosEnabled {
		return usecase.Conflict(CodePhotosDisabledForClient, "this client has switched field-worker photos off")
	}
	if !pol.WorkerMaySetOwnPhoto {
		return usecase.Authorization(CodeOwnPhotoNotAllowed, "this client does not let a worker set their own photo")
	}
	return nil
}

// SetOwnPhoto stores the signed-in worker's own photo. The actor, in the
// event and in the audit row, is the worker themselves (WORKER, and the
// principal of the audit row is the worker's id).
func (o *Ops) SetOwnPhoto() usecaseop.TxOperation[SetOwnPhotoCommand, Event] {
	return usecaseop.TxOperation[SetOwnPhotoCommand, Event]{
		Name: "SetOwnFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd SetOwnPhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if cmd.ClientID == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			if cmd.Image == nil || len(cmd.Image.Full.JPEG) == 0 || len(cmd.Image.Thumb.JPEG) == 0 {
				return usecase.Validation("PHOTO_REQUIRED", "a processed photo is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[SetOwnPhotoCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd SetOwnPhotoCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadSelf(ctx, cmd.WorkerRef)
			if err != nil {
				return Event{}, err
			}
			if err := o.ownPhotoPolicy(ctx, ident.ClientID); err != nil {
				return Event{}, err
			}
			return storePhoto(ctx, s, ec, ident, cmd.Image, PhotoByWorker, ident.ID, cmd)
		},
	}
}

// RemoveOwnPhoto removes the signed-in worker's own photo (the same policy
// rule as setting it).
func (o *Ops) RemoveOwnPhoto() usecaseop.TxOperation[RemoveOwnPhotoCommand, Event] {
	return usecaseop.TxOperation[RemoveOwnPhotoCommand, Event]{
		Name: "RemoveOwnFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd RemoveOwnPhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if cmd.ClientID == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[RemoveOwnPhotoCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RemoveOwnPhotoCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadSelf(ctx, cmd.WorkerRef)
			if err != nil {
				return Event{}, err
			}
			if err := o.ownPhotoPolicy(ctx, ident.ClientID); err != nil {
				return Event{}, err
			}
			return dropPhoto(ctx, s, ec, ident, PhotoByWorker, cmd)
		},
	}
}
