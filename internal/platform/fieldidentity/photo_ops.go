package fieldidentity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
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
	// CodePhotoDeletionNotConfirmed: the policy change would delete stored
	// photos and confirmPhotoDeletion was not set (409; details.count).
	CodePhotoDeletionNotConfirmed = "PHOTO_DELETION_NOT_CONFIRMED"
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

// photoPolicy loads the client's effective policy and refuses a worker acting
// for themselves when the client has not allowed it.
func (o *Ops) photoPolicy(ctx context.Context, clientID string, actor PhotoActor) (Policy, error) {
	pol, err := o.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return pol, repoErr("find_field_policy", err)
	}
	if actor == PhotoByWorker && pol.PhotosEnabled && !pol.WorkerMaySetOwnPhoto {
		return pol, usecase.Authorization(CodeOwnPhotoNotAllowed, "this client does not let a worker set their own photo")
	}
	return pol, nil
}

// SetWorkerPhoto stores the worker's photo, replacing any earlier one. It is
// refused with 409 PHOTOS_DISABLED_FOR_CLIENT when the client has photos off
// and, for a worker acting for themselves, 403 OWN_PHOTO_NOT_ALLOWED unless the
// client's policy allows it.
func (o *Ops) SetWorkerPhoto() usecaseop.TxOperation[SetPhotoCommand, Event] {
	return usecaseop.TxOperation[SetPhotoCommand, Event]{
		Name: "SetFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd SetPhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if !cmd.Actor.Valid() {
				return usecase.Validation("PHOTO_ACTOR_INVALID", "the actor of a photo is ADMIN, SERVICE_ACCOUNT or WORKER")
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
			pol, err := o.photoPolicy(ctx, ident.ClientID, cmd.Actor)
			if err != nil {
				return Event{}, err
			}
			if !pol.PhotosEnabled {
				return Event{}, usecase.Conflict(CodePhotosDisabledForClient, "this client has switched field-worker photos off")
			}
			now := time.Now().UTC()
			img := cmd.Image
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				// Re-read the switch under a share lock: a policy change that
				// switches photos off holds the row until it has deleted them,
				// so a photo cannot slip in behind it.
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
					img.Thumb.JPEG, img.Thumb.Width, img.Thumb.Height, string(img.Source), string(cmd.Actor), cmd.ActorID, now); err != nil {
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
				"version": img.Version, "source": string(img.Source), "setByType": string(cmd.Actor),
			}))
			return usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd))
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
// (no event). A worker acting for themselves needs the client's policy to
// allow it.
func (o *Ops) RemoveWorkerPhoto() usecaseop.TxOperation[RemovePhotoCommand, Event] {
	return usecaseop.TxOperation[RemovePhotoCommand, Event]{
		Name: "RemoveFieldWorkerPhoto",
		Validate: func(ctx context.Context, cmd RemovePhotoCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if !cmd.Actor.Valid() {
				return usecase.Validation("PHOTO_ACTOR_INVALID", "the actor of a photo is ADMIN, SERVICE_ACCOUNT or WORKER")
			}
			return nil
		},
		Authorize: usecaseop.Public[RemovePhotoCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RemovePhotoCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			if _, err := o.photoPolicy(ctx, ident.ClientID, cmd.Actor); err != nil {
				return Event{}, err
			}
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
				"removedByType": string(cmd.Actor),
			}))
			return usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd))
		},
	}
}
