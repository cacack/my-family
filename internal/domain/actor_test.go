package domain_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestActorFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "no actor", ctx: context.Background(), want: ""},
		{name: "actor", ctx: domain.ContextWithActor(context.Background(), "user-1"), want: "user-1"},
		{name: "empty actor", ctx: domain.ContextWithActor(context.Background(), ""), want: ""},
		{
			name: "empty actor keeps the outer one",
			ctx:  domain.ContextWithActor(domain.ContextWithActor(context.Background(), "outer"), ""),
			want: "outer",
		},
		{
			name: "inner actor wins",
			ctx:  domain.ContextWithActor(domain.ContextWithActor(context.Background(), "outer"), "inner"),
			want: "inner",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ActorFromContext(tt.ctx); got != tt.want {
				t.Errorf("ActorFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
