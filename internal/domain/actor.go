package domain

import "context"

// actorKey is the context key for the acting user. Unexported, so only
// ContextWithActor can set it.
type actorKey struct{}

// ContextWithActor returns a copy of ctx carrying userID as the acting user
// (ADR-007, Attribution; #1407). The command handler writes it to the
// EventMetadata.UserID of every event a command appends.
//
// Nothing sets an actor yet: the HTTP layer starts doing so with sign-in
// (#1408), and until then every event is written authorless. An empty userID
// returns ctx unchanged, so it never hides an actor set further out.
func ContextWithActor(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, userID)
}

// ActorFromContext returns the acting user's ID, or "" when ctx carries none:
// a change made before accounts existed or in open mode.
func ActorFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(actorKey{}).(string)
	return userID
}
