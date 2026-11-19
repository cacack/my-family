package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/repository"
)

// Error codes of user management.
const (
	CodeInvalidLink = "INVALID_LINK"
	CodeLastOwner   = "LAST_OWNER"
)

// accountsOnly is the refusal of an account operation in open mode.
var accountsOnly = AccountsOnlyJSONResponse(authError(CodeConflict, openModeMessage))

// actorFrom is the signed-in user and their address, for the auth audit log.
func actorFrom(ctx context.Context) auth.Actor {
	by := auth.Actor{ClientIP: requestInfoFrom(ctx).clientIP}
	if user := userFrom(ctx); user != nil {
		by.UserID = user.ID
	}
	return by
}

func userOf(m *repository.Member) User {
	return User{
		Id:          m.User.ID,
		Username:    m.User.Username,
		DisplayName: m.User.DisplayName,
		Role:        UserRole(m.Role),
		Disabled:    m.User.Disabled,
		CreatedAt:   m.User.CreatedAt,
	}
}

func linkOf(link *auth.Link) AccountLink {
	return AccountLink{Token: link.Token, ExpiresAt: link.ExpiresAt}
}

// AcceptInvite implements StrictServerInterface.
func (ss *StrictServer) AcceptInvite(ctx context.Context, request AcceptInviteRequestObject) (AcceptInviteResponseObject, error) {
	if ss.server.config.OpenMode() {
		return AcceptInvite409JSONResponse(authError(CodeConflict, openModeMessage)), nil
	}
	body := request.Body
	info := requestInfoFrom(ctx)
	user, role, token, err := ss.server.auth.AcceptInvite(ctx, body.Token, body.Username, body.DisplayName, body.Password, info.clientIP)
	switch {
	case err == nil:
		cookie := sessionCookie(token, info.secure)
		return AcceptInvite201JSONResponse{Body: accountOf(user, role), Headers: AcceptInvite201ResponseHeaders{SetCookie: &cookie}}, nil
	case errors.Is(err, auth.ErrInvalidInput):
		return AcceptInvite400JSONResponse(authError(CodeValidation, err.Error())), nil
	case errors.Is(err, auth.ErrInvalidLink):
		return AcceptInvite400JSONResponse(authError(CodeInvalidLink, "This invite link is wrong, expired or already used. Ask the owner for a new one.")), nil
	default:
		return nil, err
	}
}

// ResetPassword implements StrictServerInterface.
func (ss *StrictServer) ResetPassword(ctx context.Context, request ResetPasswordRequestObject) (ResetPasswordResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ResetPassword409JSONResponse(authError(CodeConflict, openModeMessage)), nil
	}
	err := ss.server.auth.ResetPassword(ctx, request.Body.Token, request.Body.Password, requestInfoFrom(ctx).clientIP)
	switch {
	case err == nil:
		return ResetPassword204Response{}, nil
	case errors.Is(err, auth.ErrInvalidInput):
		return ResetPassword400JSONResponse(authError(CodeValidation, err.Error())), nil
	case errors.Is(err, auth.ErrInvalidLink):
		return ResetPassword400JSONResponse(authError(CodeInvalidLink, "This reset link is wrong, expired or already used. Ask the owner for a new one.")), nil
	default:
		return nil, err
	}
}

// UpdateCurrentAccount implements StrictServerInterface.
func (ss *StrictServer) UpdateCurrentAccount(ctx context.Context, request UpdateCurrentAccountRequestObject) (UpdateCurrentAccountResponseObject, error) {
	if ss.server.config.OpenMode() {
		return UpdateCurrentAccount404JSONResponse(authError(CodeNotFound, openModeMessage)), nil
	}
	body := request.Body
	change := auth.AccountChange{DisplayName: body.DisplayName, NewPassword: body.NewPassword}
	if body.CurrentPassword != nil {
		change.CurrentPassword = *body.CurrentPassword
	}
	user, err := ss.server.auth.UpdateAccount(ctx, actorFrom(ctx), requestInfoFrom(ctx).sessionToken, change)
	switch {
	case err == nil:
		return UpdateCurrentAccount200JSONResponse(accountOf(user, roleFrom(ctx))), nil
	case errors.Is(err, auth.ErrInvalidInput):
		return UpdateCurrentAccount400JSONResponse(authError(CodeValidation, err.Error())), nil
	case errors.Is(err, auth.ErrInvalidCredentials):
		return UpdateCurrentAccount400JSONResponse(authError(CodeInvalidCredentials, "The current password is wrong")), nil
	case errors.Is(err, auth.ErrRateLimited):
		return UpdateCurrentAccount429JSONResponse{RateLimitedJSONResponse(authError(CodeRateLimited, err.Error()))}, nil
	default:
		return nil, err
	}
}

// ListUsers implements StrictServerInterface.
func (ss *StrictServer) ListUsers(ctx context.Context, _ ListUsersRequestObject) (ListUsersResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListUsers409JSONResponse{accountsOnly}, nil
	}
	members, err := ss.server.auth.Members(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]User, len(members))
	for i := range members {
		items[i] = userOf(&members[i])
	}
	return ListUsers200JSONResponse{Items: items}, nil
}

// UpdateUser implements StrictServerInterface. It changes the role first,
// then the disabled flag.
func (ss *StrictServer) UpdateUser(ctx context.Context, request UpdateUserRequestObject) (UpdateUserResponseObject, error) {
	if ss.server.config.OpenMode() {
		return UpdateUser409JSONResponse(authError(CodeConflict, openModeMessage)), nil
	}
	svc, by, id, body := ss.server.auth, actorFrom(ctx), request.Id, request.Body
	var (
		member *repository.Member
		err    error
	)
	if body.Role != nil {
		member, err = svc.SetRole(ctx, by, id, repository.Role(*body.Role))
	}
	if err == nil && body.Disabled != nil {
		member, err = svc.SetDisabled(ctx, by, id, *body.Disabled)
	}
	if body.Role == nil && body.Disabled == nil {
		member, err = svc.Member(ctx, id)
	}
	switch {
	case err == nil:
		return UpdateUser200JSONResponse(userOf(member)), nil
	case errors.Is(err, auth.ErrInvalidInput):
		return UpdateUser400JSONResponse{BadRequestJSONResponse(authError(CodeValidation, err.Error()))}, nil
	case errors.Is(err, auth.ErrUserNotFound):
		return UpdateUser404JSONResponse{NotFoundJSONResponse(authError(CodeNotFound, "User not found"))}, nil
	case errors.Is(err, auth.ErrLastOwner):
		return UpdateUser409JSONResponse(authError(CodeLastOwner, "This is the tree's last active owner. Make someone else an owner first.")), nil
	default:
		return nil, err
	}
}

// CreatePasswordReset implements StrictServerInterface.
func (ss *StrictServer) CreatePasswordReset(ctx context.Context, request CreatePasswordResetRequestObject) (CreatePasswordResetResponseObject, error) {
	if ss.server.config.OpenMode() {
		return CreatePasswordReset409JSONResponse{accountsOnly}, nil
	}
	link, err := ss.server.auth.IssuePasswordReset(ctx, actorFrom(ctx), request.Id)
	switch {
	case err == nil:
		return CreatePasswordReset201JSONResponse(linkOf(link)), nil
	case errors.Is(err, auth.ErrUserNotFound):
		return CreatePasswordReset404JSONResponse{NotFoundJSONResponse(authError(CodeNotFound, "User not found"))}, nil
	default:
		return nil, err
	}
}

// CreateInvite implements StrictServerInterface.
func (ss *StrictServer) CreateInvite(ctx context.Context, request CreateInviteRequestObject) (CreateInviteResponseObject, error) {
	if ss.server.config.OpenMode() {
		return CreateInvite409JSONResponse{accountsOnly}, nil
	}
	link, err := ss.server.auth.CreateInvite(ctx, actorFrom(ctx), repository.Role(request.Body.Role))
	switch {
	case err == nil:
		return CreateInvite201JSONResponse(linkOf(link)), nil
	case errors.Is(err, auth.ErrInvalidInput):
		return CreateInvite400JSONResponse{BadRequestJSONResponse(authError(CodeValidation, err.Error()))}, nil
	default:
		return nil, err
	}
}

// Auth audit page size, when the request names none.
const defaultAuthAuditLimit = 50

// ListAuthAudit implements StrictServerInterface. Each entry names its actor
// by display name, as history does.
func (ss *StrictServer) ListAuthAudit(ctx context.Context, request ListAuthAuditRequestObject) (ListAuthAuditResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListAuthAudit409JSONResponse{accountsOnly}, nil
	}
	limit, offset := defaultAuthAuditLimit, 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	entries, total, err := ss.server.auth.AuthAudit(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	var actorIDs []uuid.UUID
	for _, e := range entries {
		if e.ActorID != uuid.Nil {
			actorIDs = append(actorIDs, e.ActorID)
		}
	}
	names, err := ss.server.auth.DisplayNames(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	items := make([]AuthAuditEntry, len(entries))
	for i, e := range entries {
		item := AuthAuditEntry{
			At: e.At, Action: AuthAuditEntryAction(e.Action), Username: e.Username, Detail: e.Detail, ClientIp: e.ClientIP,
		}
		if e.ActorID != uuid.Nil {
			id := e.ActorID
			item.ActorId = &id
			if name, ok := names[id]; ok {
				item.ActorName = &name
			}
		}
		if e.SubjectID != uuid.Nil {
			id := e.SubjectID
			item.SubjectId = &id
		}
		items[i] = item
	}
	return ListAuthAudit200JSONResponse{Items: items, Total: total}, nil
}
