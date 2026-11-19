package main

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func pipedPrompter(input string) *prompter {
	return &prompter{in: bufio.NewReader(strings.NewReader(input)), out: &bytes.Buffer{}}
}

func TestCreateOwner_FromPipedInput(t *testing.T) {
	store := memory.NewAccountStore()
	svc := auth.NewService(store)
	var out bytes.Buffer
	if err := createOwner(context.Background(), svc, pipedPrompter("Ada\ncorrect horse battery\n"), &out, "", ""); err != nil {
		t.Fatalf("createOwner: %v", err)
	}
	if !strings.Contains(out.String(), `"ada"`) {
		t.Errorf("output %q does not name the account", out.String())
	}
	user, err := store.GetUserByUsername(context.Background(), "ada")
	if err != nil {
		t.Fatalf("no user ada: %v", err)
	}
	if user.DisplayName != "Ada" {
		t.Errorf("display name = %q, want the username as given", user.DisplayName)
	}
	if n, _ := store.CountMembers(context.Background(), repository.DefaultTreeID, repository.RoleOwner); n != 1 {
		t.Errorf("owners = %d, want 1", n)
	}
	entries, _, _ := svc.AuthAudit(context.Background(), 10, 0)
	if len(entries) != 1 || entries[0].Action != repository.AuthActionUserCreated ||
		entries[0].ActorID != uuid.Nil || entries[0].ClientIP != "cli" {
		t.Errorf("audit = %+v, want one owner created from the command line", entries)
	}
	if _, _, err := svc.SignIn(context.Background(), "ada", "correct horse battery", "cli"); err != nil {
		t.Errorf("the created owner cannot sign in: %v", err)
	}
}

func TestCreateOwner_Refusals(t *testing.T) {
	tests := []struct{ name, input, username string }{
		{"short password", "short\n", "ada"},
		{"no input", "", ""},
		{"no password", "", "ada"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := auth.NewService(memory.NewAccountStore())
			if err := createOwner(context.Background(), svc, pipedPrompter(tt.input), &bytes.Buffer{}, tt.username, "Ada"); err == nil {
				t.Error("createOwner succeeded")
			}
		})
	}
}

func TestLogAuthMode(t *testing.T) {
	tests := []struct {
		name  string
		cfg   *config.Config
		owner bool
		want  string
	}{
		{"open mode warns", &config.Config{AuthMode: config.AuthModeOpen}, false, "Warning: AUTH_MODE=open"},
		{"demo mode implies open", &config.Config{DemoMode: true}, false, "DEMO_MODE (which implies AUTH_MODE=open)"},
		{"no owner asks for setup", &config.Config{}, false, "Setup required"},
		{"with an owner", &config.Config{}, true, "Sign-in: accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := auth.NewService(memory.NewAccountStore())
			if tt.owner {
				if _, err := svc.CreateOwner(context.Background(), auth.CommandLine, "ada", "Ada", "correct horse battery"); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			if err := logAuthMode(tt.cfg, svc, log.New(&buf, "", 0)); err != nil {
				t.Fatalf("logAuthMode: %v", err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("logged %q, want it to contain %q", buf.String(), tt.want)
			}
			if tt.want == "Setup required" {
				token, _ := svc.SetupToken(context.Background())
				if token == "" || !strings.Contains(buf.String(), token) {
					t.Errorf("the setup line %q does not carry the token", buf.String())
				}
			}
		})
	}
}

func TestResetPassword_PrintsASingleUseLink(t *testing.T) {
	ctx := context.Background()
	svc := auth.NewService(memory.NewAccountStore())
	if _, err := svc.CreateOwner(ctx, auth.CommandLine, "ada", "Ada", "correct horse battery"); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	var out bytes.Buffer
	if err := resetPassword(ctx, svc, &out, "Ada", "https://tree.example/"); err != nil {
		t.Fatalf("resetPassword: %v", err)
	}
	const prefix = "https://tree.example/reset-password#token="
	i := strings.Index(out.String(), prefix)
	if i < 0 {
		t.Fatalf("output %q has no link starting %s", out.String(), prefix)
	}
	token := strings.TrimSpace(out.String()[i+len(prefix):])
	entries, _, _ := svc.AuthAudit(ctx, 10, 0)
	if len(entries) != 2 || entries[0].Action != repository.AuthActionResetIssued ||
		entries[0].ActorID != uuid.Nil || entries[0].ClientIP != entries[1].ClientIP {
		t.Errorf("audit = %+v, want the reset marked as the owner's creation is, from the command line", entries)
	}
	if err := svc.ResetPassword(ctx, token, "a brand new password", "test"); err != nil {
		t.Fatalf("the printed link does not reset the password: %v", err)
	}
	if _, _, err := svc.SignIn(ctx, "ada", "a brand new password", "test"); err != nil {
		t.Errorf("SignIn with the new password: %v", err)
	}
	if err := svc.ResetPassword(ctx, token, "yet another password", "test"); err == nil {
		t.Error("the link worked twice")
	}
}

func TestResetPassword_Refusals(t *testing.T) {
	svc := auth.NewService(memory.NewAccountStore())
	for _, username := range []string{"", "nobody"} {
		if err := resetPassword(context.Background(), svc, &bytes.Buffer{}, username, "http://localhost:8080"); err == nil {
			t.Errorf("resetPassword(%q) succeeded", username)
		}
	}
}
