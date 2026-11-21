package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"github.com/cacack/my-family/internal/repository"
)

// hashGate replaces the process-wide hashing cap for one test: slots slots,
// a short wait, and an argon2id that holds its slot until release is called.
type hashGate struct {
	inFlight, maxInFlight, runs atomic.Int32
	entered                     chan struct{}
	release                     func()
}

func capHashing(t *testing.T, slots int) *hashGate {
	t.Helper()
	g := &hashGate{entered: make(chan struct{}, 64)}
	open := make(chan struct{})
	g.release = sync.OnceFunc(func() { close(open) })
	oldSlots, oldWait, oldArgon2id := hashSlots, slotWait, argon2id
	hashSlots, slotWait = make(chan struct{}, slots), 50*time.Millisecond
	argon2id = func(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) []byte {
		g.runs.Add(1)
		n := g.inFlight.Add(1)
		for {
			if m := g.maxInFlight.Load(); n <= m || g.maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		g.entered <- struct{}{}
		<-open
		defer g.inFlight.Add(-1)
		return argon2.IDKey(password, salt, passes, memory, threads, keyLen)
	}
	t.Cleanup(func() {
		g.release()
		hashSlots, slotWait, argon2id = oldSlots, oldWait, oldArgon2id
	})
	return g
}

func TestHashCap_RefusesSignInsBeyondTheCap(t *testing.T) {
	svc, store, _ := newTestService(t)
	ctx := context.Background()
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	const slots, attempts = 2, 5
	for i := range attempts {
		user := &repository.User{ID: uuid.New(), Username: fmt.Sprintf("user%d", i), DisplayName: "U", PasswordHash: hash}
		if err := store.CreateUser(ctx, user, repository.DefaultTreeID, repository.RoleViewer); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
	}

	svc.dummyPasswordHash() // made outside the cap, before it is shrunk
	gate := capHashing(t, slots)
	results := make(chan error, attempts)
	for i := range attempts {
		go func() {
			_, _, err := svc.SignIn(ctx, fmt.Sprintf("user%d", i), testPassword, fmt.Sprintf("192.0.2.%d", i))
			results <- err
		}()
	}
	// The excess give up while the first slots are still held.
	for range attempts - slots {
		if err := <-results; !errors.Is(err, ErrBusy) {
			t.Fatalf("an excess sign-in: err = %v, want ErrBusy", err)
		}
	}
	gate.release()
	for range slots {
		if err := <-results; err != nil {
			t.Errorf("a sign-in holding a slot: err = %v, want nil", err)
		}
	}
	if got := gate.maxInFlight.Load(); got != slots {
		t.Errorf("at most %d verified at once, want %d", got, slots)
	}
	if got := gate.runs.Load(); got != slots {
		t.Errorf("argon2id ran %d times, want %d: a refused sign-in must not hash", got, slots)
	}

	// A refusal for being busy is not a failed attempt: the client and name
	// are not locked out by it.
	capHashing(t, 0)
	for range maxFailuresPerName + 1 {
		if _, _, err := svc.SignIn(ctx, "user0", testPassword, "192.0.2.0"); !errors.Is(err, ErrBusy) {
			t.Fatalf("SignIn with no slots: err = %v, want ErrBusy", err)
		}
	}
	capHashing(t, slots).release()
	if _, _, err := svc.SignIn(ctx, "user0", testPassword, "192.0.2.0"); err != nil {
		t.Errorf("SignIn after busy refusals: err = %v, want nil", err)
	}
}

func TestHashCap_SharedAcrossEndpoints(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleViewer)
	reset, err := m.svc.IssuePasswordReset(ctx, m.by, grace.ID)
	if err != nil {
		t.Fatalf("IssuePasswordReset: %v", err)
	}
	link, err := m.svc.CreateInvite(ctx, m.by, repository.RoleViewer)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	m.svc.dummyPasswordHash()
	gate := capHashing(t, 1)
	accepted := make(chan error, 1)
	go func() {
		_, _, _, err := m.svc.AcceptInvite(ctx, link.Token, "hopper", "Hopper", testPassword, "")
		accepted <- err
	}()
	<-gate.entered // invite accept holds the only slot

	if _, _, err := m.svc.SignIn(ctx, "ada", testPassword, "192.0.2.8"); !errors.Is(err, ErrBusy) {
		t.Errorf("SignIn while invite accept hashes: err = %v, want ErrBusy", err)
	}
	if err := m.svc.ResetPassword(ctx, reset.Token, "a brand new password", "192.0.2.8"); !errors.Is(err, ErrBusy) {
		t.Errorf("ResetPassword while invite accept hashes: err = %v, want ErrBusy", err)
	}
	newPassword := "another new password"
	if _, err := m.svc.UpdateAccount(ctx, m.by, "", AccountChange{CurrentPassword: testPassword, NewPassword: &newPassword}); !errors.Is(err, ErrBusy) {
		t.Errorf("UpdateAccount while invite accept hashes: err = %v, want ErrBusy", err)
	}

	gate.release()
	if err := <-accepted; err != nil {
		t.Errorf("AcceptInvite holding the slot: err = %v, want nil", err)
	}
	if got := gate.runs.Load(); got != 1 {
		t.Errorf("argon2id ran %d times, want 1: refused calls must not hash", got)
	}
	// The refused reset link is still usable.
	if err := m.svc.ResetPassword(ctx, reset.Token, "a brand new password", "192.0.2.8"); err != nil {
		t.Errorf("ResetPassword once a slot is free: err = %v, want nil", err)
	}
}

func TestHashCap_SetupRefusedWhenBusy(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	token, err := svc.SetupToken(ctx)
	if err != nil {
		t.Fatalf("SetupToken: %v", err)
	}
	capHashing(t, 0)
	if _, _, err := svc.CompleteSetup(ctx, token, "ada", "Ada", testPassword, "192.0.2.1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("CompleteSetup with no slots: err = %v, want ErrBusy", err)
	}
	capHashing(t, 1).release()
	if _, _, err := svc.CompleteSetup(ctx, token, "ada", "Ada", testPassword, "192.0.2.1"); err != nil {
		t.Errorf("CompleteSetup once a slot is free: err = %v, want nil", err)
	}
}
