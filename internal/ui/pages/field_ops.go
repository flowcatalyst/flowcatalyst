package pages

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// FieldStore is the slice of the field-identity repositories the field pages
// read (the reads behind GET /api/field-workers, /api/field-devices and
// /api/field-policies).
type FieldStore interface {
	Workers(ctx context.Context, f fieldidentity.IdentityFilter) ([]fieldidentity.Identity, int64, error)
	Worker(ctx context.Context, id string) (*fieldidentity.Identity, error)
	WorkerByCode(ctx context.Context, clientID, code string) (*fieldidentity.Identity, error)
	SignIns(ctx context.Context, identityID string, offset, limit int) ([]fieldidentity.LoginAttempt, int64, error)
	Devices(ctx context.Context, f fieldidentity.DeviceFilter) ([]fieldidentity.Device, int64, error)
	Device(ctx context.Context, id string) (*fieldidentity.Device, error)
	ActiveSessions(ctx context.Context, deviceIDs []string) (map[string]int, error)
	Policy(ctx context.Context, clientID string) (fieldidentity.Policy, error)
	// PolicyOverrides are the applications' overrides of the client's policy
	// (docs/field-policy-application-overrides.md).
	PolicyOverrides(ctx context.Context, clientID string) ([]fieldidentity.PolicyOverride, error)
	// ApplicationServes reports whether the application is active and enabled
	// for the client (its override applies only then).
	ApplicationServes(ctx context.Context, clientID, applicationID string) (bool, error)
	Applications(ctx context.Context) ([]application.Application, error)
	ActiveBadge(ctx context.Context, identityID string) (*fieldidentity.Badge, error)
	PrincipalName(ctx context.Context, id string) (name string, found bool, err error)
	// PrincipalRefs names the platform users workers are linked to; an id that
	// no longer resolves is absent from the map.
	PrincipalRefs(ctx context.Context, ids []string) (map[string]fieldidentity.PrincipalRef, error)
	// PrincipalByEmail finds a platform user by email address, nil if none.
	PrincipalByEmail(ctx context.Context, email string) (*fieldidentity.PrincipalRef, error)
	// PhotoCount is how many photos the client's workers hold.
	PhotoCount(ctx context.Context, clientID string) (int, error)
	// UnlockKeys are a worker's unlock keys, newest first, live and revoked;
	// LiveUnlockKeys the live key of each device that has one.
	UnlockKeys(ctx context.Context, identityID string) ([]fieldidentity.UnlockKey, error)
	LiveUnlockKeys(ctx context.Context, deviceIDs []string) (map[string]*fieldidentity.UnlockKey, error)
	// UnlockKeySessions is how many live sessions each key signed in (keys
	// with none are absent).
	UnlockKeySessions(ctx context.Context, keyIDs []string) (map[string]int64, error)
	// DeviceLabels names devices by id (one query); an unlabelled or unknown
	// device is absent.
	DeviceLabels(ctx context.Context, deviceIDs []string) (map[string]string, error)
}

// FieldReader reads the field-identity repositories and the applications.
type FieldReader struct {
	Repos *fieldidentity.Repos
	Apps  *application.Repository
}

// Workers searches a client's workers.
func (r FieldReader) Workers(ctx context.Context, f fieldidentity.IdentityFilter) ([]fieldidentity.Identity, int64, error) {
	return r.Repos.Identities.Search(ctx, f)
}

// Worker finds one worker by id.
func (r FieldReader) Worker(ctx context.Context, id string) (*fieldidentity.Identity, error) {
	return r.Repos.Identities.FindByID(ctx, id)
}

// WorkerByCode finds a client's worker by worker code.
func (r FieldReader) WorkerByCode(ctx context.Context, clientID, code string) (*fieldidentity.Identity, error) {
	return r.Repos.Identities.FindByClientAndCode(ctx, clientID, code)
}

// SignIns pages a worker's sign-in attempts, newest first.
func (r FieldReader) SignIns(ctx context.Context, identityID string, offset, limit int) ([]fieldidentity.LoginAttempt, int64, error) {
	return r.Repos.Sessions.SignIns(ctx, identityID, offset, limit)
}

// Devices searches a client's devices.
func (r FieldReader) Devices(ctx context.Context, f fieldidentity.DeviceFilter) ([]fieldidentity.Device, int64, error) {
	return r.Repos.Devices.Search(ctx, f)
}

// Device finds one device by id.
func (r FieldReader) Device(ctx context.Context, id string) (*fieldidentity.Device, error) {
	return r.Repos.Devices.FindByID(ctx, id)
}

// ActiveSessions counts each device's live sessions.
func (r FieldReader) ActiveSessions(ctx context.Context, deviceIDs []string) (map[string]int, error) {
	return r.Repos.Sessions.ActiveCountByDevice(ctx, deviceIDs)
}

// PolicyOverrides are the applications' overrides of the client's policy.
func (r FieldReader) PolicyOverrides(ctx context.Context, clientID string) ([]fieldidentity.PolicyOverride, error) {
	return r.Repos.Policies.Overrides(ctx, clientID)
}

// ApplicationServes reports whether the application is active and enabled for
// the client.
func (r FieldReader) ApplicationServes(ctx context.Context, clientID, applicationID string) (bool, error) {
	return r.Repos.Policies.ApplicationServes(ctx, clientID, applicationID)
}

// Policy is a client's effective policy (the defaults when it has no row).
func (r FieldReader) Policy(ctx context.Context, clientID string) (fieldidentity.Policy, error) {
	return r.Repos.Policies.Effective(ctx, clientID)
}

// Applications lists every application (names for the grants shown).
func (r FieldReader) Applications(ctx context.Context) ([]application.Application, error) {
	return r.Apps.FindWithFilters(ctx, nil, nil)
}

// ActiveBadge is a worker's ACTIVE badge, nil when it has none.
func (r FieldReader) ActiveBadge(ctx context.Context, identityID string) (*fieldidentity.Badge, error) {
	return r.Repos.Badges.ActiveByIdentity(ctx, identityID)
}

// PrincipalName is a platform principal's display name (the actor of a
// sign-in made with a console code); found is false for an unknown id.
func (r FieldReader) PrincipalName(ctx context.Context, id string) (string, bool, error) {
	name, _, found, err := r.Repos.Sessions.PrincipalName(ctx, id)
	return name, found, err
}

// PrincipalRefs names the platform users the given ids belong to.
func (r FieldReader) PrincipalRefs(ctx context.Context, ids []string) (map[string]fieldidentity.PrincipalRef, error) {
	return r.Repos.Identities.PrincipalRefs(ctx, ids)
}

// PrincipalByEmail finds a platform user by email address.
func (r FieldReader) PrincipalByEmail(ctx context.Context, email string) (*fieldidentity.PrincipalRef, error) {
	return r.Repos.Identities.PrincipalByEmail(ctx, email)
}

// PhotoCount is how many photos the client's workers hold.
func (r FieldReader) PhotoCount(ctx context.Context, clientID string) (int, error) {
	return r.Repos.Photos.CountForClient(ctx, clientID)
}

// UnlockKeys are a worker's unlock keys, newest first.
func (r FieldReader) UnlockKeys(ctx context.Context, identityID string) ([]fieldidentity.UnlockKey, error) {
	return r.Repos.UnlockKeys.ListByIdentity(ctx, identityID)
}

// UnlockKeySessions is how many live sessions each key signed in.
func (r FieldReader) UnlockKeySessions(ctx context.Context, keyIDs []string) (map[string]int64, error) {
	return r.Repos.UnlockKeys.LiveSessionCounts(ctx, keyIDs, time.Now().UTC())
}

// LiveUnlockKeys is the live unlock key of each device that has one.
func (r FieldReader) LiveUnlockKeys(ctx context.Context, deviceIDs []string) (map[string]*fieldidentity.UnlockKey, error) {
	return r.Repos.UnlockKeys.LiveByDevices(ctx, deviceIDs)
}

// DeviceLabels names devices by id.
func (r FieldReader) DeviceLabels(ctx context.Context, deviceIDs []string) (map[string]string, error) {
	return r.Repos.UnlockKeys.DeviceLabels(ctx, deviceIDs)
}

// FieldCommands are the field-identity operations the pages run; FieldOps
// runs the API's operations. The permission check (CanManageFieldWorkers or
// CanManageFieldDevices for the target's client) is the page's, as it is the
// API handler's.
type FieldCommands interface {
	EnsureWorker(ctx context.Context, cmd fieldidentity.EnsureWorkerCommand) (fieldidentity.Event, error)
	UpdateWorker(ctx context.Context, cmd fieldidentity.UpdateWorkerCommand) error
	GrantWorkerApp(ctx context.Context, cmd fieldidentity.WorkerAppCommand) error
	RevokeWorkerApp(ctx context.Context, cmd fieldidentity.WorkerAppCommand) error
	SuspendWorker(ctx context.Context, ref fieldidentity.WorkerRef) error
	ActivateWorker(ctx context.Context, ref fieldidentity.WorkerRef) error
	UnlockWorker(ctx context.Context, ref fieldidentity.WorkerRef) error
	LinkWorker(ctx context.Context, cmd fieldidentity.LinkWorkerCommand) error
	UnlinkWorker(ctx context.Context, ref fieldidentity.WorkerRef) error
	DeleteWorker(ctx context.Context, ref fieldidentity.WorkerRef) error
	ResetWorkerPin(ctx context.Context, cmd fieldidentity.ResetPinCommand) (fieldidentity.ResetPinResult, error)
	IssueBadge(ctx context.Context, cmd fieldidentity.IssueBadgeCommand) (fieldidentity.IssuedBadge, error)
	IssueBadges(ctx context.Context, cmd fieldidentity.IssueBadgesCommand) ([]fieldidentity.IssuedBadge, error)
	RevokeBadge(ctx context.Context, ref fieldidentity.BadgeRef) error
	CreateSignInCode(ctx context.Context, cmd fieldidentity.CreateSignInCodeCommand) (fieldidentity.CreateSignInCodeResult, error)
	CreateEnrolmentCode(ctx context.Context, cmd fieldidentity.CreateEnrolmentCodeCommand) (fieldidentity.CreateEnrolmentCodeResult, error)
	SuspendDevice(ctx context.Context, ref fieldidentity.DeviceRef) error
	ActivateDevice(ctx context.Context, ref fieldidentity.DeviceRef) error
	RevokeDevice(ctx context.Context, ref fieldidentity.DeviceRef) error
	PutPolicy(ctx context.Context, cmd fieldidentity.PutPolicyCommand) error
	PatchPolicyOverride(ctx context.Context, cmd fieldidentity.PatchPolicyOverrideCommand) error
	SetPolicyLocks(ctx context.Context, cmd fieldidentity.SetPolicyLocksCommand) error
	// LockingRevokesKeys reports whether the locks would revoke every unlock
	// key of the client (the page asks first).
	LockingRevokesKeys(ctx context.Context, clientID string, locked []fieldidentity.PolicyMember) (bool, error)
	SetWorkerPhoto(ctx context.Context, cmd fieldidentity.SetPhotoCommand) error
	RemoveWorkerPhoto(ctx context.Context, cmd fieldidentity.RemovePhotoCommand) error
	// DeleteClientPhotos deletes every photo of the client's workers; the count.
	DeleteClientPhotos(ctx context.Context, cmd fieldidentity.DeleteClientPhotosCommand) (int, error)
	RevokeUnlockKey(ctx context.Context, ref fieldidentity.UnlockKeyRef) error
}

// FieldOps runs the field-identity operations through the unit of work,
// exactly as internal/platform/fieldidentity/api does.
type FieldOps struct {
	Ops *fieldidentity.Ops
	UoW *usecasepgx.UnitOfWork
}

func (o FieldOps) EnsureWorker(ctx context.Context, cmd fieldidentity.EnsureWorkerCommand) (fieldidentity.Event, error) {
	return o.Ops.RunEnsureWorker(ctx, o.UoW, cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) UpdateWorker(ctx context.Context, cmd fieldidentity.UpdateWorkerCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.UpdateWorker(), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) LinkWorker(ctx context.Context, cmd fieldidentity.LinkWorkerCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.LinkWorker(), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) UnlinkWorker(ctx context.Context, ref fieldidentity.WorkerRef) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.UnlinkWorker(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) GrantWorkerApp(ctx context.Context, cmd fieldidentity.WorkerAppCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.GrantWorkerApp(), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) RevokeWorkerApp(ctx context.Context, cmd fieldidentity.WorkerAppCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.RevokeWorkerApp(), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) SuspendWorker(ctx context.Context, ref fieldidentity.WorkerRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.SuspendWorker(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) ActivateWorker(ctx context.Context, ref fieldidentity.WorkerRef) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.ActivateWorker(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) UnlockWorker(ctx context.Context, ref fieldidentity.WorkerRef) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.UnlockWorker(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) DeleteWorker(ctx context.Context, ref fieldidentity.WorkerRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.DeleteWorker(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) ResetWorkerPin(ctx context.Context, cmd fieldidentity.ResetPinCommand) (fieldidentity.ResetPinResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, o.Ops.ResetWorkerPin(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) IssueBadge(ctx context.Context, cmd fieldidentity.IssueBadgeCommand) (fieldidentity.IssuedBadge, error) {
	return usecaseop.RunTx(ctx, o.UoW, o.Ops.IssueBadge(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) IssueBadges(ctx context.Context, cmd fieldidentity.IssueBadgesCommand) ([]fieldidentity.IssuedBadge, error) {
	return usecaseop.RunTx(ctx, o.UoW, o.Ops.IssueBadges(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) RevokeBadge(ctx context.Context, ref fieldidentity.BadgeRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.RevokeBadge(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) CreateSignInCode(ctx context.Context, cmd fieldidentity.CreateSignInCodeCommand) (fieldidentity.CreateSignInCodeResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, o.Ops.CreateSignInCode(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) CreateEnrolmentCode(ctx context.Context, cmd fieldidentity.CreateEnrolmentCodeCommand) (fieldidentity.CreateEnrolmentCodeResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, o.Ops.CreateEnrolmentCode(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) SuspendDevice(ctx context.Context, ref fieldidentity.DeviceRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.SuspendDevice(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) ActivateDevice(ctx context.Context, ref fieldidentity.DeviceRef) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.ActivateDevice(), ref, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) RevokeDevice(ctx context.Context, ref fieldidentity.DeviceRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.RevokeDevice(), ref, auth.NewExecutionContext(ctx))
	return err
}

// PatchPolicyOverride runs the admin API's PatchFieldPolicyOverride use case.
func (o FieldOps) PatchPolicyOverride(ctx context.Context, cmd fieldidentity.PatchPolicyOverrideCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.PatchPolicyOverride(), cmd, auth.NewExecutionContext(ctx))
	return err
}

// LockingRevokesKeys asks the operations whether the locks would revoke every
// key of the client.
func (o FieldOps) LockingRevokesKeys(ctx context.Context, clientID string, locked []fieldidentity.PolicyMember) (bool, error) {
	return o.Ops.LockingRevokesKeys(ctx, clientID, locked)
}

// SetPolicyLocks runs the admin API's SetFieldPolicyLocks use case.
func (o FieldOps) SetPolicyLocks(ctx context.Context, cmd fieldidentity.SetPolicyLocksCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.SetPolicyLocks(), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o FieldOps) PutPolicy(ctx context.Context, cmd fieldidentity.PutPolicyCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.PutPolicy(), cmd, auth.NewExecutionContext(ctx))
	return err
}

// SetWorkerPhoto runs the admin API's SetFieldWorkerPhoto use case.
func (o FieldOps) SetWorkerPhoto(ctx context.Context, cmd fieldidentity.SetPhotoCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.SetWorkerPhoto(), cmd, auth.NewExecutionContext(ctx))
	return err
}

// RemoveWorkerPhoto runs the admin API's RemoveFieldWorkerPhoto use case.
func (o FieldOps) RemoveWorkerPhoto(ctx context.Context, cmd fieldidentity.RemovePhotoCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.RemoveWorkerPhoto(), cmd, auth.NewExecutionContext(ctx))
	return err
}

// DeleteClientPhotos runs the admin API's DeleteClientFieldPhotos use case.
func (o FieldOps) DeleteClientPhotos(ctx context.Context, cmd fieldidentity.DeleteClientPhotosCommand) (int, error) {
	ev, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.DeleteClientPhotos(), cmd, auth.NewExecutionContext(ctx))
	return fieldidentity.PhotosRemoved(ev), err
}

// RevokeUnlockKey runs the admin API's RevokeFieldUnlockKey use case.
func (o FieldOps) RevokeUnlockKey(ctx context.Context, ref fieldidentity.UnlockKeyRef) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, o.Ops.RevokeUnlockKey(), ref, auth.NewExecutionContext(ctx))
	return err
}
