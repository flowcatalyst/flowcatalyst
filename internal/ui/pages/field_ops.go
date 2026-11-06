package pages

import (
	"context"

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
	Applications(ctx context.Context) ([]application.Application, error)
	ActiveBadge(ctx context.Context, identityID string) (*fieldidentity.Badge, error)
	PrincipalName(ctx context.Context, id string) (name string, found bool, err error)
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
}

// FieldOps runs the field-identity operations through the unit of work,
// exactly as internal/platform/fieldidentity/api does.
type FieldOps struct {
	Ops *fieldidentity.Ops
	UoW *usecasepgx.UnitOfWork
}

func (o FieldOps) EnsureWorker(ctx context.Context, cmd fieldidentity.EnsureWorkerCommand) (fieldidentity.Event, error) {
	return usecaseop.Run(ctx, o.UoW, o.Ops.EnsureWorker(), cmd, auth.NewExecutionContext(ctx))
}

func (o FieldOps) UpdateWorker(ctx context.Context, cmd fieldidentity.UpdateWorkerCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.UpdateWorker(), cmd, auth.NewExecutionContext(ctx))
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

func (o FieldOps) PutPolicy(ctx context.Context, cmd fieldidentity.PutPolicyCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, o.Ops.PutPolicy(), cmd, auth.NewExecutionContext(ctx))
	return err
}
