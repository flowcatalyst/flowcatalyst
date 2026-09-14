package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
)

// fakeInviteEmailer records SendInvite/InviteLink calls so tests can assert
// which path notifyNewUser took, without touching the passwordreset package.
type fakeInviteEmailer struct {
	sendInviteCalls int
	sendInviteErr   error
	inviteLinkCalls int
	inviteLinkErr   error
	linkToReturn    string
}

func (f *fakeInviteEmailer) SendInvite(context.Context, *principal.Principal) error {
	f.sendInviteCalls++
	return f.sendInviteErr
}

func (f *fakeInviteEmailer) InviteLink(_ context.Context, _ *principal.Principal, _ *string) (string, error) {
	f.inviteLinkCalls++
	if f.inviteLinkErr != nil {
		return "", f.inviteLinkErr
	}
	return f.linkToReturn, nil
}

// fakeEmailService records every send so tests can assert whether the
// "account created" welcome (routed through notify.Notifier) went out.
type fakeEmailService struct {
	sent []email.Message
}

func (f *fakeEmailService) Send(_ context.Context, m email.Message) error {
	f.sent = append(f.sent, m)
	return nil
}

func testPrincipal(passwordSet bool) *principal.Principal {
	return &principal.Principal{
		ID:   "prn_test1",
		Type: principal.TypeUser,
		UserIdentity: &principal.UserIdentity{
			Email: "new-user@example.test",
		},
	}
}

func oidcPrincipal() *principal.Principal {
	provider := "OIDC"
	return &principal.Principal{
		ID:   "prn_test_oidc",
		Type: principal.TypeUser,
		UserIdentity: &principal.UserIdentity{
			Email:    "fed-user@example.test",
			Provider: &provider,
		},
	}
}

func newState(invite *fakeInviteEmailer, mail *fakeEmailService) *State {
	return &State{
		InviteEmailer: invite,
		Notifier:      notify.New(mail),
	}
}

// Default behaviour (sendInvitation omitted/true, returnInviteLink omitted/false):
// a passwordless user gets the platform invite email; no link is returned.
func TestNotifyNewUser_DefaultSendsInvite(t *testing.T) {
	invite := &fakeInviteEmailer{}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), testPrincipal(false), nil, true, false)

	if link != nil {
		t.Fatalf("expected no link returned, got %q", *link)
	}
	if invite.sendInviteCalls != 1 {
		t.Fatalf("expected SendInvite called once, got %d", invite.sendInviteCalls)
	}
	if invite.inviteLinkCalls != 0 {
		t.Fatalf("expected InviteLink not called, got %d", invite.inviteLinkCalls)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("expected no welcome email sent, got %d", len(mail.sent))
	}
}

// A user created WITH a password gets the welcome email by default.
func TestNotifyNewUser_DefaultWithPasswordSendsWelcome(t *testing.T) {
	invite := &fakeInviteEmailer{}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	pw := "s3cret!!"
	link := s.notifyNewUser(context.Background(), testPrincipal(true), &pw, true, false)

	if link != nil {
		t.Fatalf("expected no link returned, got %q", *link)
	}
	if invite.sendInviteCalls != 0 {
		t.Fatalf("expected SendInvite not called, got %d", invite.sendInviteCalls)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("expected one welcome email, got %d", len(mail.sent))
	}
}

// sendInvitation:false suppresses ALL platform email — neither the invite
// nor the welcome goes out — regardless of whether a password was supplied.
func TestNotifyNewUser_SendInvitationFalseSuppressesAllEmail(t *testing.T) {
	invite := &fakeInviteEmailer{}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), testPrincipal(false), nil, false, false)
	if link != nil {
		t.Fatalf("expected no link returned, got %q", *link)
	}
	if invite.sendInviteCalls != 0 {
		t.Fatalf("expected SendInvite not called, got %d", invite.sendInviteCalls)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("expected no email sent, got %d", len(mail.sent))
	}

	// Same with a password supplied — welcome must also be suppressed.
	pw := "s3cret!!"
	link = s.notifyNewUser(context.Background(), testPrincipal(true), &pw, false, false)
	if link != nil {
		t.Fatalf("expected no link returned, got %q", *link)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("expected no welcome email sent, got %d", len(mail.sent))
	}
}

// returnInviteLink:true mints and returns the link, and does NOT also send
// the platform's own invite email — the precedence rule chosen for this
// feature (documented on CreateUserRequest.ReturnInviteLink).
func TestNotifyNewUser_ReturnInviteLinkSuppressesPlatformEmail(t *testing.T) {
	invite := &fakeInviteEmailer{linkToReturn: "https://example.test/auth/set-password?token=abc123"}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), testPrincipal(false), nil, true, true)

	if link == nil || *link != invite.linkToReturn {
		t.Fatalf("expected link %q, got %v", invite.linkToReturn, link)
	}
	if invite.inviteLinkCalls != 1 {
		t.Fatalf("expected InviteLink called once, got %d", invite.inviteLinkCalls)
	}
	if invite.sendInviteCalls != 0 {
		t.Fatalf("expected SendInvite NOT called (precedence: returnInviteLink wins), got %d", invite.sendInviteCalls)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("expected no welcome email sent, got %d", len(mail.sent))
	}
}

// returnInviteLink:true with sendInvitation:false behaves the same as
// returnInviteLink:true with sendInvitation:true — the link wins either way.
func TestNotifyNewUser_ReturnInviteLinkWinsOverSendInvitationFalse(t *testing.T) {
	invite := &fakeInviteEmailer{linkToReturn: "https://example.test/auth/set-password?token=xyz"}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), testPrincipal(false), nil, false, true)

	if link == nil || *link != invite.linkToReturn {
		t.Fatalf("expected link %q, got %v", invite.linkToReturn, link)
	}
	if invite.sendInviteCalls != 0 {
		t.Fatalf("expected SendInvite NOT called, got %d", invite.sendInviteCalls)
	}
}

// returnInviteLink:true is moot for a user created WITH a password — there is
// no invite token to mint, so no link is returned and (sendInvitation
// defaulting true) the normal welcome email still goes out.
func TestNotifyNewUser_ReturnInviteLinkIgnoredWhenPasswordSupplied(t *testing.T) {
	invite := &fakeInviteEmailer{linkToReturn: "https://example.test/should-not-be-used"}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	pw := "s3cret!!"
	link := s.notifyNewUser(context.Background(), testPrincipal(true), &pw, true, true)

	if link != nil {
		t.Fatalf("expected no link (user has a password), got %q", *link)
	}
	if invite.inviteLinkCalls != 0 {
		t.Fatalf("expected InviteLink not called, got %d", invite.inviteLinkCalls)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("expected the welcome email to still be sent, got %d", len(mail.sent))
	}
}

// An OIDC/federated user never gets a link or any platform email, regardless
// of the flags.
func TestNotifyNewUser_OIDCUserNeverNotified(t *testing.T) {
	invite := &fakeInviteEmailer{linkToReturn: "https://example.test/should-not-be-used"}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), oidcPrincipal(), nil, true, true)

	if link != nil {
		t.Fatalf("expected no link for an OIDC user, got %q", *link)
	}
	if invite.sendInviteCalls != 0 || invite.inviteLinkCalls != 0 {
		t.Fatalf("expected no InviteEmailer calls for an OIDC user, got send=%d link=%d",
			invite.sendInviteCalls, invite.inviteLinkCalls)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("expected no welcome email for an OIDC user, got %d", len(mail.sent))
	}
}

// A service-account principal (no UserIdentity) is always a no-op.
func TestNotifyNewUser_ServiceAccountNoOp(t *testing.T) {
	invite := &fakeInviteEmailer{}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	sa := &principal.Principal{ID: "prn_sa1", Type: principal.TypeService}
	link := s.notifyNewUser(context.Background(), sa, nil, true, true)

	if link != nil {
		t.Fatalf("expected no link for a service account, got %q", *link)
	}
	if invite.sendInviteCalls != 0 || invite.inviteLinkCalls != 0 || len(mail.sent) != 0 {
		t.Fatalf("expected no notification activity for a service account")
	}
}

// A failed mint (InviteLink error) is logged and swallowed — best-effort,
// same as the existing SendInvite failure handling — and returns no link.
func TestNotifyNewUser_ReturnInviteLinkMintFailureIsBestEffort(t *testing.T) {
	invite := &fakeInviteEmailer{inviteLinkErr: errMint}
	mail := &fakeEmailService{}
	s := newState(invite, mail)

	link := s.notifyNewUser(context.Background(), testPrincipal(false), nil, true, true)
	if link != nil {
		t.Fatalf("expected nil link on mint failure, got %q", *link)
	}
}

var errMint = &mintError{}

type mintError struct{}

func (*mintError) Error() string { return "mint failed" }
