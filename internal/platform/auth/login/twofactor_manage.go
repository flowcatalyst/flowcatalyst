package login

import (
	"context"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// The signed-in user's own factors (the Profile screen's /auth/2fa/* routes and
// the /ui security page). Every call acts on the principal the caller loaded
// from the session, and on nothing a request names but the item to act on.

// Status is a user's 2FA state.
type Status struct {
	// Methods are the confirmed factor types.
	Methods []string
	// Required: the domain compels 2FA. AllowedMethods: the methods the user
	// may enrol.
	Required       bool
	AllowedMethods []string
	// RecoveryCodes reports whether recovery codes apply (a usable TOTP
	// factor); RecoveryCodesLeft how many are unused.
	RecoveryCodes     bool
	RecoveryCodesLeft int
	// RememberDeviceEnabled: the domain lets browsers be remembered.
	RememberDeviceEnabled bool
	TrustedDevices        []mfa.TrustedDevice
}

// Status is p's 2FA state. The recovery-code count and the device list are
// best effort (an error leaves them empty).
func (t *TwoFactor) Status(ctx context.Context, p *principal.Principal) (*Status, error) {
	confirmed, err := t.MFA.ConfirmedMethods(ctx, string(p.ID))
	if err != nil {
		return nil, err
	}
	ev := t.Policy.Evaluate(ctx, emailOf(p))
	s := &Status{
		Methods:               methodStrings(confirmed),
		Required:              ev.Requires2FA(),
		AllowedMethods:        enrollable(ev),
		RecoveryCodes:         slices.Contains(usableMethods(confirmed, ev), string(mfa.MethodTOTP)),
		RememberDeviceEnabled: ev.RememberEnabled(),
	}
	s.RecoveryCodesLeft, _ = t.MFA.RemainingRecoveryCodes(ctx, string(p.ID))
	s.TrustedDevices, _ = t.MFA.ListTrustedDevices(ctx, string(p.ID))
	return s, nil
}

// BeginMethod starts enrolling method for p: the authenticator setup (TOTP),
// or the confirmation code emailed (EMAIL_PIN, nil setup).
func (t *TwoFactor) BeginMethod(ctx context.Context, p *principal.Principal, method mfa.MethodType) (*mfa.TOTPEnrollment, error) {
	return t.begin(ctx, p, method)
}

// ConfirmMethod confirms p's begun enrolment of method (ErrInvalidCode for a
// wrong code) and returns the first recovery codes, shown once, when there are
// any to show.
func (t *TwoFactor) ConfirmMethod(ctx context.Context, p *principal.Principal, method mfa.MethodType, code string) ([]string, error) {
	if err := t.confirm(ctx, p, method, code); err != nil {
		return nil, err
	}
	return t.ensureRecoveryCodes(ctx, p), nil
}

// RemoveMethod removes one of p's factors. A user whose domain compels 2FA
// cannot remove their last usable factor (a passkey does not satisfy the
// password path).
func (t *TwoFactor) RemoveMethod(ctx context.Context, p *principal.Principal, method string) error {
	if !mfa.ValidMethodType(method) {
		return ErrInvalidMethod
	}
	confirmed, err := t.MFA.ConfirmedMethods(ctx, string(p.ID))
	if err != nil {
		return err
	}
	ev := t.Policy.Evaluate(ctx, emailOf(p))
	if ev.Requires2FA() {
		rest := slices.DeleteFunc(slices.Clone(confirmed), func(m mfa.MethodType) bool { return string(m) == method })
		if len(usableMethods(rest, ev)) == 0 {
			return ErrLastFactor
		}
	}
	if err := t.MFA.RemoveMethod(ctx, string(p.ID), mfa.MethodType(method)); err != nil {
		return err
	}
	t.Notifier.TwoFactorMethodRemoved(ctx, emailOf(p), method)
	t.auditMFA(ctx, string(p.ID), "2FA_METHOD_REMOVED")
	return nil
}

// RegenerateRecoveryCodes replaces p's recovery codes and returns the new set,
// shown once. It needs a usable authenticator-app factor (ErrNoTOTP).
func (t *TwoFactor) RegenerateRecoveryCodes(ctx context.Context, p *principal.Principal) ([]string, error) {
	usable, _, err := t.usable(ctx, p)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(usable, string(mfa.MethodTOTP)) {
		return nil, ErrNoTOTP
	}
	codes, err := t.MFA.GenerateRecoveryCodes(ctx, string(p.ID))
	if err != nil {
		return nil, err
	}
	t.Notifier.RecoveryCodesRegenerated(ctx, emailOf(p))
	t.auditMFA(ctx, string(p.ID), "2FA_RECOVERY_REGENERATED")
	return codes, nil
}

// TrustedDevices lists p's remembered devices.
func (t *TwoFactor) TrustedDevices(ctx context.Context, p *principal.Principal) ([]mfa.TrustedDevice, error) {
	return t.MFA.ListTrustedDevices(ctx, string(p.ID))
}

// RevokeTrustedDevice removes one of p's remembered devices (scoped to p in
// the service: another user's id removes nothing).
func (t *TwoFactor) RevokeTrustedDevice(ctx context.Context, p *principal.Principal, id string) error {
	return t.MFA.RevokeTrustedDevice(ctx, string(p.ID), id)
}
