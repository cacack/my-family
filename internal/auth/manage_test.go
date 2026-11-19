package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// managed is a service with a signed-in owner.
type managed struct {
	svc   *Service
	clock *clock
	owner *repository.User
	by    Actor
}

func newManaged(t *testing.T) *managed {
	t.Helper()
	svc, _, c := newTestService(t)
	owner, err := svc.CreateOwner(context.Background(), Actor{}, "ada", "Ada", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	return &managed{svc: svc, clock: c, owner: owner, by: Actor{UserID: owner.ID, ClientIP: "192.0.2.1"}}
}

// invite creates an account with role through an invite and returns it with
// its session token.
func (m *managed) invite(t *testing.T, username string, role repository.Role) (*repository.User, string) {
	t.Helper()
	ctx := context.Background()
	link, err := m.svc.CreateInvite(ctx, m.by, role)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	user, got, token, err := m.svc.AcceptInvite(ctx, link.Token, username, username, testPassword, "192.0.2.9")
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if got != role {
		t.Fatalf("AcceptInvite role = %s, want %s", got, role)
	}
	return user, token
}

// actions returns the audit log's actions, oldest first.
func (m *managed) actions(t *testing.T) []repository.AuthAction {
	t.Helper()
	entries, _, err := m.svc.AuthAudit(context.Background(), 1000, 0)
	if err != nil {
		t.Fatalf("AuthAudit: %v", err)
	}
	actions := make([]repository.AuthAction, len(entries))
	for i, e := range entries {
		actions[len(entries)-1-i] = e.Action
	}
	return actions
}

func TestInvite_CreatesAccountWithRoleOnce(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	link, err := m.svc.CreateInvite(ctx, m.by, repository.RoleEditor)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if want := m.clock.Now().Add(InviteTTL); !link.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", link.ExpiresAt, want)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "Grace", "Grace Hopper", "short", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("AcceptInvite with a short password: err = %v, want ErrInvalidInput", err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "ada", "Ada again", testPassword, ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("AcceptInvite with a taken username: err = %v, want ErrInvalidInput", err)
	}
	user, role, token, err := m.svc.AcceptInvite(ctx, link.Token, "Grace", "Grace Hopper", testPassword, "")
	if err != nil {
		t.Fatalf("AcceptInvite: %v (refused attempts must not use the invite up)", err)
	}
	if role != repository.RoleEditor || user.Username != "grace" {
		t.Errorf("AcceptInvite = %s, %s; want grace, editor", user.Username, role)
	}
	if got, err := m.svc.Authenticate(ctx, token); err != nil || got.ID != user.ID {
		t.Errorf("the invited user is not signed in: %v, %v", got, err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "eve", "Eve", testPassword, ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("AcceptInvite twice: err = %v, want ErrInvalidLink", err)
	}
	if _, err := m.svc.CreateInvite(ctx, m.by, "admin"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateInvite with an unknown role: err = %v, want ErrInvalidInput", err)
	}
}

func TestInvite_Expires(t *testing.T) {
	m := newManaged(t)
	link, err := m.svc.CreateInvite(context.Background(), m.by, repository.RoleViewer)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	m.clock.advance(InviteTTL)
	if _, _, _, err := m.svc.AcceptInvite(context.Background(), link.Token, "grace", "Grace", testPassword, ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("AcceptInvite of an expired invite: err = %v, want ErrInvalidLink", err)
	}
}

func TestSetRole_AndLastOwner(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleViewer)

	if _, err := m.svc.SetRole(ctx, m.by, m.owner.ID, repository.RoleEditor); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the last owner: err = %v, want ErrLastOwner", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, m.owner.ID, true); !errors.Is(err, ErrLastOwner) {
		t.Errorf("disabling the last owner: err = %v, want ErrLastOwner", err)
	}
	member, err := m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleOwner)
	if err != nil || member.Role != repository.RoleOwner {
		t.Fatalf("SetRole(grace, owner) = %+v, %v", member, err)
	}
	// With two owners, one may step down; then the other is the last.
	if _, err := m.svc.SetRole(ctx, m.by, m.owner.ID, repository.RoleEditor); err != nil {
		t.Fatalf("demoting one of two owners: %v", err)
	}
	if _, err := m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleViewer); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the remaining owner: err = %v, want ErrLastOwner", err)
	}
	if setup, err := m.svc.SetupRequired(ctx); err != nil || setup {
		t.Errorf("SetupRequired after role changes = %v, %v; want false", setup, err)
	}
	// Setting the role a user already has changes and records nothing.
	before := len(m.actions(t))
	if _, err := m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleOwner); err != nil {
		t.Errorf("SetRole to the same role: %v", err)
	}
	if after := len(m.actions(t)); after != before {
		t.Errorf("a no-op role change was audited: %d entries, was %d", after, before)
	}
	if _, err := m.svc.SetRole(ctx, m.by, grace.ID, "admin"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("SetRole to an unknown role: err = %v, want ErrInvalidInput", err)
	}
	if _, err := m.svc.SetRole(ctx, m.by, uuid.New(), repository.RoleViewer); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("SetRole of an unknown user: err = %v, want ErrUserNotFound", err)
	}
}

func TestSetDisabled_EndsSessionsAndRefusesSignIn(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, token := m.invite(t, "grace", repository.RoleEditor)

	member, err := m.svc.SetDisabled(ctx, m.by, grace.ID, true)
	if err != nil || !member.User.Disabled {
		t.Fatalf("SetDisabled = %+v, %v", member, err)
	}
	if _, err := m.svc.Authenticate(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("a disabled user's session still works: err = %v", err)
	}
	if _, _, err := m.svc.SignIn(ctx, "grace", testPassword, "192.0.2.9"); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("SignIn of a disabled user: err = %v, want ErrAccountDisabled", err)
	}
	if _, _, err := m.svc.SignIn(ctx, "grace", "wrong password", "192.0.2.9"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("a wrong password for a disabled user: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, false); err != nil {
		t.Fatalf("re-enabling: %v", err)
	}
	if _, _, err := m.svc.SignIn(ctx, "grace", testPassword, "192.0.2.9"); err != nil {
		t.Errorf("SignIn after re-enabling: %v", err)
	}

	// A disabled owner does not count toward keeping one active.
	if _, err := m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleOwner); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, true); err != nil {
		t.Fatalf("disabling one of two owners: %v", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, m.owner.ID, true); !errors.Is(err, ErrLastOwner) {
		t.Errorf("disabling the last active owner: err = %v, want ErrLastOwner", err)
	}
	if _, err := m.svc.SetRole(ctx, m.by, m.owner.ID, repository.RoleViewer); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the last active owner: err = %v, want ErrLastOwner", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, uuid.New(), true); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("SetDisabled of an unknown user: err = %v, want ErrUserNotFound", err)
	}
}

func TestPasswordReset(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, token := m.invite(t, "grace", repository.RoleViewer)

	link, err := m.svc.IssuePasswordReset(ctx, m.by, grace.ID)
	if err != nil {
		t.Fatalf("IssuePasswordReset: %v", err)
	}
	if want := m.clock.Now().Add(ResetTTL); !link.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", link.ExpiresAt, want)
	}
	if err := m.svc.ResetPassword(ctx, link.Token, "short", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ResetPassword with a short password: err = %v, want ErrInvalidInput", err)
	}
	const newPassword = "a brand new password"
	if err := m.svc.ResetPassword(ctx, link.Token, newPassword, ""); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := m.svc.Authenticate(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("a reset left the user's session working: err = %v", err)
	}
	if _, _, err := m.svc.SignIn(ctx, "grace", newPassword, "192.0.2.9"); err != nil {
		t.Errorf("SignIn with the new password: %v", err)
	}
	if err := m.svc.ResetPassword(ctx, link.Token, "yet another password", ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("ResetPassword twice: err = %v, want ErrInvalidLink", err)
	}

	expiring, err := m.svc.IssuePasswordReset(ctx, m.by, grace.ID)
	if err != nil {
		t.Fatalf("IssuePasswordReset: %v", err)
	}
	m.clock.advance(ResetTTL)
	if err := m.svc.ResetPassword(ctx, expiring.Token, "yet another password", ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("ResetPassword of an expired link: err = %v, want ErrInvalidLink", err)
	}
	if _, err := m.svc.IssuePasswordReset(ctx, m.by, uuid.New()); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("IssuePasswordReset of an unknown user: err = %v, want ErrUserNotFound", err)
	}
	if _, _, err := m.svc.IssuePasswordResetFor(ctx, Actor{}, "nobody"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("IssuePasswordResetFor an unknown name: err = %v, want ErrUserNotFound", err)
	}
	user, cliLink, err := m.svc.IssuePasswordResetFor(ctx, CommandLine, " Grace ")
	if err != nil || user.ID != grace.ID || cliLink.Token == "" {
		t.Errorf("IssuePasswordResetFor = %v, %v, %v", user, cliLink, err)
	}
}

func TestUpdateAccount(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, token := m.invite(t, "grace", repository.RoleViewer)
	_, other, err := m.svc.SignIn(ctx, "grace", testPassword, "192.0.2.9")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	by := Actor{UserID: grace.ID}
	ptr := func(s string) *string { return &s }

	user, err := m.svc.UpdateAccount(ctx, by, token, AccountChange{DisplayName: ptr("  Rear Admiral Hopper ")})
	if err != nil || user.DisplayName != "Rear Admiral Hopper" {
		t.Fatalf("UpdateAccount(display name) = %+v, %v", user, err)
	}
	if _, err := m.svc.Authenticate(ctx, other); err != nil {
		t.Errorf("a name change ended another session: %v", err)
	}
	for name, change := range map[string]AccountChange{
		"empty name":     {DisplayName: ptr(" ")},
		"short password": {CurrentPassword: testPassword, NewPassword: ptr("short")},
	} {
		if _, err := m.svc.UpdateAccount(ctx, by, token, change); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	const newPassword = "a brand new password"
	if _, err := m.svc.UpdateAccount(ctx, by, token, AccountChange{CurrentPassword: "wrong password", NewPassword: ptr(newPassword)}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("a wrong current password: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := m.svc.UpdateAccount(ctx, by, token, AccountChange{CurrentPassword: testPassword, NewPassword: ptr(newPassword)}); err != nil {
		t.Fatalf("UpdateAccount(password): %v", err)
	}
	if _, err := m.svc.Authenticate(ctx, token); err != nil {
		t.Errorf("changing the password ended the session that did it: %v", err)
	}
	if _, err := m.svc.Authenticate(ctx, other); !errors.Is(err, ErrNoSession) {
		t.Errorf("changing the password left another session working: err = %v", err)
	}
	if _, _, err := m.svc.SignIn(ctx, "grace", newPassword, "192.0.2.9"); err != nil {
		t.Errorf("SignIn with the new password: %v", err)
	}
}

func TestUpdateAccount_RateLimitsWrongPasswords(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	by := Actor{UserID: m.owner.ID}
	newPassword := "a brand new password"
	for i := 0; i < maxFailuresPerName; i++ {
		_, _ = m.svc.UpdateAccount(ctx, by, "", AccountChange{CurrentPassword: "wrong password", NewPassword: &newPassword})
	}
	if _, err := m.svc.UpdateAccount(ctx, by, "", AccountChange{CurrentPassword: testPassword, NewPassword: &newPassword}); !errors.Is(err, ErrRateLimited) {
		t.Errorf("after %d wrong passwords: err = %v, want ErrRateLimited", maxFailuresPerName, err)
	}
	m.clock.advance(limitWindow)
	if _, err := m.svc.UpdateAccount(ctx, by, "", AccountChange{CurrentPassword: testPassword, NewPassword: &newPassword}); err != nil {
		t.Errorf("after the window: %v", err)
	}
}

func TestAuthAudit_RecordsEachAction(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleViewer)
	_, _, _ = m.svc.SignIn(ctx, "grace", "wrong password", "192.0.2.9")
	_, _, _ = m.svc.SignIn(ctx, "grace", testPassword, "192.0.2.9")
	_, _ = m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleEditor)
	_, _ = m.svc.SetDisabled(ctx, m.by, grace.ID, true)
	_, _ = m.svc.SetDisabled(ctx, m.by, grace.ID, false)
	link, _ := m.svc.IssuePasswordReset(ctx, m.by, grace.ID)
	_ = m.svc.ResetPassword(ctx, link.Token, "a brand new password", "192.0.2.9")
	newPassword := "another new password"
	_, _ = m.svc.UpdateAccount(ctx, Actor{UserID: grace.ID}, "", AccountChange{CurrentPassword: "a brand new password", NewPassword: &newPassword})

	want := []repository.AuthAction{
		repository.AuthActionUserCreated,
		repository.AuthActionInviteCreated,
		repository.AuthActionInviteAccepted,
		repository.AuthActionSignInFailed,
		repository.AuthActionSignIn,
		repository.AuthActionRoleChanged,
		repository.AuthActionUserDisabled,
		repository.AuthActionUserEnabled,
		repository.AuthActionResetIssued,
		repository.AuthActionPasswordReset,
		repository.AuthActionPasswordChanged,
	}
	got := m.actions(t)
	if len(got) != len(want) {
		t.Fatalf("audit actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("audit action %d = %s, want %s", i, got[i], want[i])
		}
	}

	entries, total, _ := m.svc.AuthAudit(ctx, 100, 0)
	if total != len(want) {
		t.Errorf("total = %d, want %d", total, len(want))
	}
	byAction := map[repository.AuthAction]repository.AuthAuditEntry{}
	for _, e := range entries {
		byAction[e.Action] = e
	}
	if e := byAction[repository.AuthActionRoleChanged]; e.ActorID != m.owner.ID || e.SubjectID != grace.ID || e.Detail != "viewer -> editor" || e.ClientIP != "192.0.2.1" {
		t.Errorf("role change entry = %+v", e)
	}
	if e := byAction[repository.AuthActionSignInFailed]; e.ActorID != uuid.Nil || e.Username != "grace" || e.ClientIP != "192.0.2.9" {
		t.Errorf("failed sign-in entry = %+v", e)
	}
	if e := byAction[repository.AuthActionPasswordReset]; e.ActorID != grace.ID || e.SubjectID != grace.ID {
		t.Errorf("reset entry = %+v, want grace acting on herself", e)
	}
	if e := byAction[repository.AuthActionInviteCreated]; e.Detail != "viewer" || e.ActorID != m.owner.ID {
		t.Errorf("invite entry = %+v", e)
	}
	if e := byAction[repository.AuthActionSignIn]; !e.At.Equal(m.clock.Now()) {
		t.Errorf("entry time = %v, want %v", e.At, m.clock.Now())
	}
}

// failingAudit is a store whose audit log cannot be written.
type failingAudit struct {
	repository.AccountStore
}

func (failingAudit) AppendAuthAudit(context.Context, *repository.AuthAuditEntry) error {
	return errors.New("disk full")
}

func TestAudit_FailureDoesNotFailTheAction(t *testing.T) {
	svc, store, _ := newTestService(t)
	svc.store = failingAudit{store}
	if _, err := svc.CreateOwner(context.Background(), Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner with a failing audit log: %v", err)
	}
	if _, _, err := svc.SignIn(context.Background(), "ada", testPassword, "x"); err != nil {
		t.Errorf("SignIn with a failing audit log: %v", err)
	}
}

func TestLinks_RevokedWhenIssuerDisabledOrDemoted(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleOwner)
	bob, _ := m.invite(t, "bob", repository.RoleViewer)
	byGrace := Actor{UserID: grace.ID}

	invite, err := m.svc.CreateInvite(ctx, byGrace, repository.RoleOwner)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	reset, err := m.svc.IssuePasswordReset(ctx, byGrace, bob.ID)
	if err != nil {
		t.Fatalf("IssuePasswordReset: %v", err)
	}
	kept, err := m.svc.CreateInvite(ctx, m.by, repository.RoleViewer)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, true); err != nil {
		t.Fatalf("SetDisabled: %v", err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, invite.Token, "eve", "Eve", testPassword, ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("an invite from a disabled owner: err = %v, want ErrInvalidLink", err)
	}
	if err := m.svc.ResetPassword(ctx, reset.Token, "a brand new password", ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("a reset from a disabled owner: err = %v, want ErrInvalidLink", err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, kept.Token, "carol", "Carol", testPassword, ""); err != nil {
		t.Errorf("another owner's invite stopped working: %v", err)
	}

	if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, false); err != nil {
		t.Fatalf("re-enabling: %v", err)
	}
	invite, err = m.svc.CreateInvite(ctx, byGrace, repository.RoleOwner)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if _, err := m.svc.SetRole(ctx, m.by, grace.ID, repository.RoleEditor); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, invite.Token, "eve", "Eve", testPassword, ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("an invite from a demoted owner: err = %v, want ErrInvalidLink", err)
	}
}

func TestLinks_ListAndRevokePending(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleViewer)

	invite, err := m.svc.CreateInvite(ctx, m.by, repository.RoleEditor)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	reset, err := m.svc.IssuePasswordReset(ctx, m.by, grace.ID)
	if err != nil {
		t.Fatalf("IssuePasswordReset: %v", err)
	}
	invites, err := m.svc.PendingInvites(ctx)
	if err != nil || len(invites) != 1 || invites[0].TokenHash != hashToken(invite.Token) || invites[0].Role != repository.RoleEditor {
		t.Fatalf("PendingInvites = %+v, %v; want the editor invite (the accepted one is gone)", invites, err)
	}
	resets, err := m.svc.PendingPasswordResets(ctx)
	if err != nil || len(resets) != 1 || resets[0].UserID != grace.ID || resets[0].CreatedBy != m.owner.ID {
		t.Fatalf("PendingPasswordResets = %+v, %v; want grace's reset", resets, err)
	}

	if err := m.svc.RevokeInvite(ctx, m.by, invites[0].TokenHash); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	if err := m.svc.RevokePasswordReset(ctx, m.by, resets[0].TokenHash); err != nil {
		t.Fatalf("RevokePasswordReset: %v", err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, invite.Token, "eve", "Eve", testPassword, ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("AcceptInvite of a revoked invite: err = %v, want ErrInvalidLink", err)
	}
	if err := m.svc.ResetPassword(ctx, reset.Token, "a brand new password", ""); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("ResetPassword of a revoked reset: err = %v, want ErrInvalidLink", err)
	}
	if err := m.svc.RevokeInvite(ctx, m.by, invites[0].TokenHash); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("RevokeInvite twice: err = %v, want ErrInvalidLink", err)
	}
	if err := m.svc.RevokePasswordReset(ctx, m.by, resets[0].TokenHash); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("RevokePasswordReset twice: err = %v, want ErrInvalidLink", err)
	}

	entries, _, _ := m.svc.AuthAudit(ctx, 2, 0)
	if len(entries) != 2 {
		t.Fatalf("AuthAudit = %+v, want the two revocations", entries)
	}
	if e := entries[0]; e.Action != repository.AuthActionResetRevoked || e.ActorID != m.owner.ID || e.SubjectID != grace.ID || e.Username != "grace" {
		t.Errorf("reset revocation entry = %+v", e)
	}
	if e := entries[1]; e.Action != repository.AuthActionInviteRevoked || e.ActorID != m.owner.ID || e.Detail != "editor" {
		t.Errorf("invite revocation entry = %+v", e)
	}

	// An expired link is no longer pending, so it cannot be revoked either.
	expiring, err := m.svc.CreateInvite(ctx, m.by, repository.RoleViewer)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	m.clock.advance(InviteTTL)
	if invites, _ := m.svc.PendingInvites(ctx); len(invites) != 0 {
		t.Errorf("PendingInvites after expiry = %+v, want none", invites)
	}
	if err := m.svc.RevokeInvite(ctx, m.by, hashToken(expiring.Token)); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("RevokeInvite of an expired invite: err = %v, want ErrInvalidLink", err)
	}
}

func TestLinks_RevokeStoreFailure(t *testing.T) {
	svc, store, _ := newTestService(t)
	svc.store = failingLinks{store}
	if err := svc.RevokeInvite(context.Background(), Actor{}, "x"); err == nil || errors.Is(err, ErrInvalidLink) {
		t.Errorf("RevokeInvite with a failing store: err = %v, want the store's error", err)
	}
	if err := svc.RevokePasswordReset(context.Background(), Actor{}, "x"); err == nil || errors.Is(err, ErrInvalidLink) {
		t.Errorf("RevokePasswordReset with a failing store: err = %v, want the store's error", err)
	}
}

// failingLinks is a store whose links cannot be revoked.
type failingLinks struct {
	repository.AccountStore
}

func (failingLinks) RevokeInvite(context.Context, string, time.Time) (*repository.Invite, error) {
	return nil, errors.New("disk full")
}

func (failingLinks) RevokePasswordReset(context.Context, string, time.Time) (*repository.PasswordReset, error) {
	return nil, errors.New("disk full")
}

func TestLinks_RateLimitInvalidTokens(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	link, err := m.svc.CreateInvite(ctx, m.by, repository.RoleViewer)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	// A refused password is not a guess and is not counted.
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "grace", "Grace", "short", "192.0.2.7"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("a short password: err = %v, want ErrInvalidInput", err)
	}
	for i := 0; i < maxFailuresPerClient; i++ {
		if err := m.svc.ResetPassword(ctx, "wrong", "a brand new password", "192.0.2.7"); !errors.Is(err, ErrInvalidLink) {
			t.Fatalf("attempt %d: err = %v, want ErrInvalidLink", i, err)
		}
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "grace", "Grace", testPassword, "192.0.2.7"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("after %d invalid links: err = %v, want ErrRateLimited", maxFailuresPerClient, err)
	}
	if _, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "grace", "Grace", testPassword, "192.0.2.8"); err != nil {
		t.Errorf("another client was limited: %v", err)
	}
}

// disableOnRead is a store that disables a user right after UpdateAccount
// first reads them, as an owner might while the password is being hashed.
type disableOnRead struct {
	repository.AccountStore
	disable func()
}

func (s *disableOnRead) GetUser(ctx context.Context, id uuid.UUID) (*repository.User, error) {
	user, err := s.AccountStore.GetUser(ctx, id)
	if s.disable != nil {
		disable := s.disable
		s.disable = nil
		disable()
	}
	return user, err
}

func TestUpdateAccount_KeepsADisableMadeMeanwhile(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleEditor)
	store := &disableOnRead{AccountStore: m.svc.store}
	m.svc.store = store
	store.disable = func() {
		if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, true); err != nil {
			t.Errorf("SetDisabled: %v", err)
		}
	}
	newPassword := "a brand new password"
	if _, err := m.svc.UpdateAccount(ctx, Actor{UserID: grace.ID}, "", AccountChange{CurrentPassword: testPassword, NewPassword: &newPassword}); err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if u, err := store.GetUser(ctx, grace.ID); err != nil || !u.Disabled {
		t.Errorf("after UpdateAccount, grace = %+v, %v; want still disabled", u, err)
	}
}
