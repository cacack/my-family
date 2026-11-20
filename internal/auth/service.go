// Package auth signs people in (ADR-007): it creates the first owner, checks
// passwords and issues, resolves and revokes sessions. Accounts live in a
// repository.AccountStore, never in the event log.
//
// Nothing secret leaves this package except the session token handed to the
// caller to set as a cookie: passwords are kept only as argon2id hashes and
// tokens only as SHA-256 hashes, and no error or log line carries either.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Session lifetimes. A session ends after IdleTimeout without a request, or
// AbsoluteTimeout after sign-in, whichever comes first.
const (
	IdleTimeout     = 7 * 24 * time.Hour
	AbsoluteTimeout = 30 * 24 * time.Hour

	// touchInterval bounds how often a request writes a session's last-seen
	// time: a write per request would be wasted at this idle timeout.
	touchInterval = time.Minute
)

// Sign-in rate limits: failures within limitWindow per username and per
// client address. A refused attempt is not checked, so it costs no hash. An
// attempt is counted before its hash is checked, which also caps how many
// checks one client can have running at once.
const (
	limitWindow          = 15 * time.Minute
	maxFailuresPerName   = 5
	maxFailuresPerClient = 20
)

// Account field limits.
const (
	maxUsernameLen    = 64
	maxDisplayNameLen = 100
	minPasswordLen    = 8
	maxPasswordLen    = 256
)

// Errors returned by Service.
var (
	// ErrInvalidCredentials is a failed sign-in. It never says whether the
	// username exists.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrRateLimited refuses a sign-in or setup attempt after too many
	// failures.
	ErrRateLimited = errors.New("too many failed attempts; try again later")
	// ErrNoSession is a missing, unknown, expired or revoked session.
	ErrNoSession = errors.New("not signed in")
	// ErrSetupDone refuses setup once an owner exists.
	ErrSetupDone = errors.New("setup is already complete")
	// ErrInvalidSetupToken is a setup attempt without the server's token.
	ErrInvalidSetupToken = errors.New("invalid setup token")
	// ErrInvalidInput wraps a rejected username, display name or password.
	ErrInvalidInput = errors.New("invalid input")
	// ErrAccountDisabled refuses a sign-in with the right password to a
	// disabled account.
	ErrAccountDisabled = errors.New("this account is disabled")
)

// Service signs people in. It is safe for concurrent use.
type Service struct {
	store repository.AccountStore
	// settings, when set, holds the site-wide home person the first owner's
	// account starts with.
	settings repository.SettingsStore
	now      func() time.Time
	limiter  *limiter

	// hasOwner caches a true CountMembers: the last owner can be neither
	// demoted nor disabled, so once one exists setup is over for good. While it is false every check asks
	// the store, so an owner created by `myfamily user create` against a
	// running server ends setup there too.
	hasOwner atomic.Bool

	// setupMu serializes setup, so the one-time token admits one owner.
	setupMu    sync.Mutex
	setupToken string

	// manageMu serializes role changes, disabling and other writes of a user
	// row, so two at once cannot both pass the last-owner check or overwrite
	// each other. It is per process: the guard holds for one server (ADR-004),
	// not for several servers sharing one database.
	manageMu sync.Mutex

	dummyOnce sync.Once
	dummyHash string
}

// Option configures a Service.
type Option func(*Service)

// WithSettings supplies the settings store, so the first owner's account
// starts with the home person chosen for the site before accounts were
// enabled (#1443). Without it that account starts with none.
func WithSettings(settings repository.SettingsStore) Option {
	return func(s *Service) {
		s.settings = settings
	}
}

// NewService returns a Service over store.
func NewService(store repository.AccountStore, opts ...Option) *Service {
	s := &Service{store: store, now: time.Now, limiter: newLimiter(limitWindow)}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NormalizeUsername is the stored form of a username: trimmed and lowercase,
// so "Ada" and "ada " sign in as the same account.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// validateAccount checks a normalized username, a trimmed display name and a
// password.
func validateAccount(username, displayName, password string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if err := validateDisplayName(displayName); err != nil {
		return err
	}
	return validatePassword(password)
}

func validateUsername(username string) error {
	if username == "" || utf8.RuneCountInString(username) > maxUsernameLen {
		return invalid("username must be 1 to %d characters", maxUsernameLen)
	}
	for _, r := range username {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-@", r) {
			return invalid("username may contain only letters, digits and . _ - @")
		}
	}
	return nil
}

func validateDisplayName(displayName string) error {
	if displayName == "" || utf8.RuneCountInString(displayName) > maxDisplayNameLen {
		return invalid("display name must be 1 to %d characters", maxDisplayNameLen)
	}
	return nil
}

func validatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		return invalid("password must be %d to %d characters", minPasswordLen, maxPasswordLen)
	}
	return nil
}

// CreateOwner creates an owner account of the tree, recording by as who did
// it. It signs no one in and does no setup gating: each call adds another
// owner, which is how `myfamily user create` adds one after setup.
func (s *Service) CreateOwner(ctx context.Context, by Actor, username, displayName, password string) (*repository.User, error) {
	user, err := s.createOwner(ctx, username, displayName, password)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, by, repository.AuthActionUserCreated, user, string(repository.RoleOwner))
	return user, nil
}

// createOwner creates an owner account, unaudited. It is what CreateOwner
// and setup share. The tree's first owner starts with the site's home person.
func (s *Service) createOwner(ctx context.Context, username, displayName, password string) (*repository.User, error) {
	user, err := s.newUser(username, displayName, password)
	if err != nil {
		return nil, err
	}
	first, err := s.SetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateUser(ctx, user, repository.DefaultTreeID, repository.RoleOwner); err != nil {
		if errors.Is(err, repository.ErrUsernameTaken) {
			return nil, invalid("username %q is already taken", user.Username)
		}
		return nil, fmt.Errorf("create owner: %w", err)
	}
	s.hasOwner.Store(true)
	if first {
		s.seedHomePerson(ctx, user.ID)
	}
	return user, nil
}

// seedHomePerson gives the first owner's account the site-wide home person,
// the one chosen while the server ran in open mode. The account already
// exists, so a failure is logged, not returned: the owner can choose again.
func (s *Service) seedHomePerson(ctx context.Context, ownerID uuid.UUID) {
	if s.settings == nil {
		return
	}
	if err := s.settings.SeedHomePerson(ctx, ownerID); err != nil {
		slog.Error("auth: could not give the first owner the site's home person", "error", err)
	}
}

// CarryOverHomePerson gives the tree's only owner the site-wide home person
// if that owner has never chosen or cleared one: an owner set up before home
// persons were per account (#1474) was never seeded. With more than one
// owner it does nothing, since the site's choice may be any one of theirs.
// The server calls it at startup; it is safe to call again.
func (s *Service) CarryOverHomePerson(ctx context.Context) error {
	if s.settings == nil {
		return nil
	}
	members, err := s.store.ListMembers(ctx, repository.DefaultTreeID)
	if err != nil {
		return fmt.Errorf("carry over home person: %w", err)
	}
	var owners []uuid.UUID
	for _, m := range members {
		if m.Role == repository.RoleOwner {
			owners = append(owners, m.User.ID)
		}
	}
	if len(owners) != 1 {
		return nil
	}
	if err := s.settings.SeedHomePerson(ctx, owners[0]); err != nil {
		return fmt.Errorf("carry over home person: %w", err)
	}
	return nil
}

// newUser validates the details of a new account and builds it, with its
// password hashed.
func (s *Service) newUser(username, displayName, password string) (*repository.User, error) {
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if err := validateAccount(username, displayName, password); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	return &repository.User{
		ID:           uuid.New(),
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		CreatedAt:    s.now().UTC(),
	}, nil
}

// SetupRequired reports whether the tree has no owner yet, in which case the
// server serves only setup.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	if s.hasOwner.Load() {
		return false, nil
	}
	n, err := s.store.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner)
	if err != nil {
		return false, fmt.Errorf("count owners: %w", err)
	}
	if n > 0 {
		s.hasOwner.Store(true)
		return false, nil
	}
	return true, nil
}

// SetupToken returns the one-time token that creating the first owner needs,
// generating it on first call, or "" when setup is not required. The server
// writes it to its log at startup: whoever can read that log controls the
// deployment anyway (ADR-007).
func (s *Service) SetupToken(ctx context.Context) (string, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil || !required {
		return "", err
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupToken == "" {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return "", fmt.Errorf("generate setup token: %w", err)
		}
		s.setupToken = hex.EncodeToString(raw)
	}
	return s.setupToken, nil
}

// CompleteSetup creates the first owner with the server's setup token and
// signs them in, returning the session token. clientKey identifies the
// caller for rate limiting (its address). The token must already have been
// issued by SetupToken (the server does so at startup); until then every
// attempt is ErrInvalidSetupToken.
func (s *Service) CompleteSetup(ctx context.Context, token, username, displayName, password, clientKey string) (*repository.User, string, error) {
	now := s.now()
	limitKey := "setup:" + clientKey
	if !s.limiter.acquire(now, limit{limitKey, maxFailuresPerClient}) {
		return nil, "", ErrRateLimited
	}

	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	required, err := s.SetupRequired(ctx)
	if err != nil {
		s.limiter.release(now, limitKey)
		return nil, "", err
	}
	if !required {
		s.limiter.release(now, limitKey)
		return nil, "", ErrSetupDone
	}
	if s.setupToken == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(s.setupToken)) != 1 {
		return nil, "", ErrInvalidSetupToken // stays counted as a failure
	}
	s.limiter.release(now, limitKey)
	user, err := s.createOwner(ctx, username, displayName, password)
	if err != nil {
		return nil, "", err
	}
	// Whoever holds the setup token acts as the owner it creates.
	s.audit(ctx, Actor{UserID: user.ID, ClientIP: clientKey}, repository.AuthActionUserCreated, user, string(repository.RoleOwner))
	s.setupToken = ""
	sessionToken, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	return user, sessionToken, nil
}

// SignIn checks username and password and starts a session, returning its
// token. clientKey identifies the caller for rate limiting (its address).
func (s *Service) SignIn(ctx context.Context, username, password, clientKey string) (*repository.User, string, error) {
	now := s.now()
	username = NormalizeUsername(username)
	by := Actor{ClientIP: clientKey}
	nameKey, clientLimitKey := "name:"+username, "client:"+clientKey
	if !s.limiter.acquire(now, limit{nameKey, maxFailuresPerName}, limit{clientLimitKey, maxFailuresPerClient}) {
		return nil, "", ErrRateLimited
	}

	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		s.limiter.release(now, nameKey, clientLimitKey)
		return nil, "", fmt.Errorf("sign in: %w", err)
	}
	hash := s.dummyPasswordHash()
	if user != nil {
		hash = user.PasswordHash
	}
	// An unknown username is checked against a dummy hash, so its answer takes
	// as long as a wrong password's and does not reveal which names exist.
	ok, verifyErr := VerifyPassword(password, hash)
	if verifyErr != nil && user != nil {
		slog.Error("auth: stored password hash does not parse", "user_id", user.ID, "error", verifyErr)
	}
	if user == nil || !ok {
		s.auditEntry(ctx, repository.AuthAuditEntry{Action: repository.AuthActionSignInFailed, Username: username, ClientIP: clientKey})
		return nil, "", ErrInvalidCredentials // stays counted as a failure
	}
	s.limiter.release(now, clientLimitKey)
	s.limiter.reset(nameKey)
	if user.Disabled {
		s.auditEntry(ctx, repository.AuthAuditEntry{Action: repository.AuthActionSignInFailed, SubjectID: user.ID,
			Username: username, Detail: "account disabled", ClientIP: clientKey})
		return nil, "", ErrAccountDisabled
	}

	if _, err := s.store.DeleteExpiredSessions(ctx, now.Add(-IdleTimeout), now.Add(-AbsoluteTimeout)); err != nil {
		slog.Warn("auth: could not delete expired sessions", "error", err)
	}
	token, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	by.UserID = user.ID
	s.audit(ctx, by, repository.AuthActionSignIn, user, "")
	return user, token, nil
}

// dummyPasswordHash is a hash no password is checked against for real.
func (s *Service) dummyPasswordHash() string {
	s.dummyOnce.Do(func() {
		hash, err := HashPassword("not a real password")
		if err != nil {
			// rand.Read does not fail on supported platforms; a malformed
			// hash still fails every check.
			hash = "$argon2id$"
		}
		s.dummyHash = hash
	})
	return s.dummyHash
}

// startSession stores a new session for userID and returns its token.
func (s *Service) startSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if err := s.store.CreateSession(ctx, &repository.Session{
		TokenHash: hashToken(token), UserID: userID, CreatedAt: now, LastSeenAt: now,
	}); err != nil {
		return "", fmt.Errorf("start session: %w", err)
	}
	return token, nil
}

// newToken returns a random token for a session, invite or password reset
// link.
func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// hashToken is the stored form of a session, invite or reset token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Authenticate returns the user a session token belongs to, or ErrNoSession
// when it is unknown, expired or revoked. An expired session is deleted.
func (s *Service) Authenticate(ctx context.Context, token string) (*repository.User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	tokenHash := hashToken(token)
	session, err := s.store.GetSession(ctx, tokenHash)
	if errors.Is(err, repository.ErrSessionNotFound) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	now := s.now().UTC()
	if now.Sub(session.LastSeenAt) >= IdleTimeout || now.Sub(session.CreatedAt) >= AbsoluteTimeout {
		if err := s.store.DeleteSession(ctx, tokenHash); err != nil {
			slog.Warn("auth: could not delete an expired session", "error", err)
		}
		return nil, ErrNoSession
	}
	user, err := s.store.GetUser(ctx, session.UserID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	if user.Disabled {
		// Disabling ends the user's sessions; this refuses any that a race
		// left behind.
		return nil, ErrNoSession
	}
	if now.Sub(session.LastSeenAt) >= touchInterval {
		if err := s.store.TouchSession(ctx, tokenHash, now); err != nil && !errors.Is(err, repository.ErrSessionNotFound) {
			slog.Warn("auth: could not record session activity", "error", err)
		}
	}
	return user, nil
}

// SignOut revokes the session token belongs to. Signing out a session that
// is already gone succeeds.
func (s *Service) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.store.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("sign out: %w", err)
	}
	return nil
}

// DisplayNames returns the display name of each of ids that is a user, for
// naming who made a change in history and merge records.
func (s *Service) DisplayNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return s.store.DisplayNames(ctx, ids)
}

// Role returns the role userID holds in treeID, or repository.ErrNotMember.
func (s *Service) Role(ctx context.Context, treeID, userID uuid.UUID) (repository.Role, error) {
	return s.store.MemberRole(ctx, treeID, userID)
}
