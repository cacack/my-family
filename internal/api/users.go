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
	return AccountLink{Id: link.ID, Token: link.Token, ExpiresAt: link.ExpiresAt}
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
	case errors.Is(err, auth.ErrRateLimited):
		return AcceptInvite429JSONResponse{RateLimitedJSONResponse(authError(CodeRateLimited, err.Error()))}, nil
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
	case errors.Is(err, auth.ErrRateLimited):
		return ResetPassword429JSONResponse{RateLimitedJSONResponse(authError(CodeRateLimited, err.Error()))}, nil
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

// ListAuthAudit implements StrictServerInterface. Each entry names its actor
// by display name, as history does.
func (ss *StrictServer) ListAuthAudit(ctx context.Context, request ListAuthAuditRequestObject) (ListAuthAuditResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListAuthAudit409JSONResponse{accountsOnly}, nil
	}
	// The auth service applies the default and clamps both.
	limit, offset := 0, 0
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

// nameIn returns names[id] when it is there, for an optional display name.
func nameIn(names map[uuid.UUID]string, id uuid.UUID) *string {
	if name, ok := names[id]; ok {
		return &name
	}
	return nil
}

// ListInvites implements StrictServerInterface. Each invite names its issuer.
func (ss *StrictServer) ListInvites(ctx context.Context, _ ListInvitesRequestObject) (ListInvitesResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListInvites409JSONResponse{accountsOnly}, nil
	}
	invites, err := ss.server.auth.PendingInvites(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(invites))
	for i := range invites {
		ids[i] = invites[i].CreatedBy
	}
	names, err := ss.server.auth.DisplayNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]PendingInvite, len(invites))
	for i, inv := range invites {
		items[i] = PendingInvite{
			Id: inv.TokenHash, Role: PendingInviteRole(inv.Role), CreatedBy: inv.CreatedBy,
			CreatedByName: nameIn(names, inv.CreatedBy), CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt,
		}
	}
	return ListInvites200JSONResponse{Items: items}, nil
}

// RevokeInvite implements StrictServerInterface.
func (ss *StrictServer) RevokeInvite(ctx context.Context, request RevokeInviteRequestObject) (RevokeInviteResponseObject, error) {
	if ss.server.config.OpenMode() {
		return RevokeInvite409JSONResponse{accountsOnly}, nil
	}
	err := ss.server.auth.RevokeInvite(ctx, actorFrom(ctx), request.LinkId)
	switch {
	case err == nil:
		return RevokeInvite204Response{}, nil
	case errors.Is(err, auth.ErrInvalidLink):
		return RevokeInvite404JSONResponse{NotFoundJSONResponse(authError(CodeNotFound, "No such invite is pending"))}, nil
	default:
		return nil, err
	}
}

// ListPasswordResets implements StrictServerInterface. Each reset names its
// user and, unless the CLI issued it, its issuer.
func (ss *StrictServer) ListPasswordResets(ctx context.Context, _ ListPasswordResetsRequestObject) (ListPasswordResetsResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListPasswordResets409JSONResponse{accountsOnly}, nil
	}
	resets, err := ss.server.auth.PendingPasswordResets(ctx)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, r := range resets {
		ids = append(ids, r.UserID)
		if r.CreatedBy != uuid.Nil {
			ids = append(ids, r.CreatedBy)
		}
	}
	names, err := ss.server.auth.DisplayNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]PendingPasswordReset, len(resets))
	for i, r := range resets {
		item := PendingPasswordReset{
			Id: r.TokenHash, UserId: r.UserID, UserName: nameIn(names, r.UserID), CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
		}
		if r.CreatedBy != uuid.Nil {
			id := r.CreatedBy
			item.CreatedBy, item.CreatedByName = &id, nameIn(names, id)
		}
		items[i] = item
	}
	return ListPasswordResets200JSONResponse{Items: items}, nil
}

// RevokePasswordReset implements StrictServerInterface.
func (ss *StrictServer) RevokePasswordReset(ctx context.Context, request RevokePasswordResetRequestObject) (RevokePasswordResetResponseObject, error) {
	if ss.server.config.OpenMode() {
		return RevokePasswordReset409JSONResponse{accountsOnly}, nil
	}
	err := ss.server.auth.RevokePasswordReset(ctx, actorFrom(ctx), request.LinkId)
	switch {
	case err == nil:
		return RevokePasswordReset204Response{}, nil
	case errors.Is(err, auth.ErrInvalidLink):
		return RevokePasswordReset404JSONResponse{NotFoundJSONResponse(authError(CodeNotFound, "No such password reset is pending"))}, nil
	default:
		return nil, err
	}
}
