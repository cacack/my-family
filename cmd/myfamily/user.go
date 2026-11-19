package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
)

// logAuthMode writes the startup line that says how this server signs people
// in, and, while the tree has no owner, the one-time setup token (ADR-007).
func logAuthMode(cfg *config.Config, svc *auth.Service, logger *log.Logger) error {
	if cfg.OpenMode() {
		reason := "AUTH_MODE=open"
		if cfg.DemoMode {
			reason = "DEMO_MODE (which implies AUTH_MODE=open)"
		}
		logger.Printf("Warning: %s: every request is treated as the owner, with no sign-in, and changes record no user. Use it only on a trusted network.", reason)
		return nil
	}
	token, err := svc.SetupToken(context.Background())
	if err != nil {
		return fmt.Errorf("check for an owner account: %w", err)
	}
	if token == "" {
		logger.Printf("Sign-in: accounts (set AUTH_MODE=open to run without sign-in on a trusted network)")
		return nil
	}
	logger.Printf("Setup required: there is no owner account, so the server answers only its setup page. "+
		"Open the web UI and enter the setup token %s, or run `myfamily user create`. "+
		"To keep running without sign-in on a trusted network, set AUTH_MODE=open instead.", token)
	return nil
}

// userUsage lists the `myfamily user` subcommands.
const userUsage = "usage: myfamily user create [--username NAME] [--display-name NAME]\n" +
	"       myfamily user reset-password --username NAME [--url URL]"

// runUser runs `myfamily user <subcommand>`.
func runUser(args []string) error {
	if len(args) == 0 || (args[0] != "create" && args[0] != "reset-password") {
		return errors.New(userUsage)
	}
	flags := flag.NewFlagSet("user "+args[0], flag.ContinueOnError)
	username := flags.String("username", "", "sign-in name (asked for by create when omitted)")
	displayName := flags.String("display-name", "", "name shown in history (defaults to the username)")
	baseURL := flags.String("url", "", "the address people open the web UI at (default http://localhost:PORT)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	cfg := config.Load()
	if cfg.DemoMode {
		return errors.New("DEMO_MODE keeps nothing between runs, accounts included; unset it to manage accounts")
	}
	stores, err := storage.Open(cfg, storage.AccessServe)
	if err != nil {
		return fmt.Errorf("failed to open %s storage: %w", storage.Select(cfg), err)
	}
	defer func() {
		if err := stores.Close(); err != nil {
			log.Printf("Error closing storage: %v", err)
		}
	}()

	svc := auth.NewService(stores.Accounts)
	if args[0] == "reset-password" {
		if *baseURL == "" {
			*baseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
		}
		return resetPassword(context.Background(), svc, os.Stdout, *username, *baseURL)
	}
	prompt := newPrompter(os.Stdin, os.Stderr)
	return createOwner(context.Background(), svc, prompt, os.Stdout, *username, *displayName)
}

// resetPasswordPath is the web UI page a reset link opens; the token rides in
// the fragment, which browsers never send to the server.
const resetPasswordPath = "/reset-password#token="

// resetPassword issues a password reset link for username and prints it.
func resetPassword(ctx context.Context, svc *auth.Service, out io.Writer, username, baseURL string) error {
	if username == "" {
		return errors.New("--username is required")
	}
	user, link, err := svc.IssuePasswordResetFor(ctx, auth.CommandLine, username)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Password reset link for %q, single-use, valid until %s:\n%s%s%s\n",
		user.Username, link.ExpiresAt.Local().Format(time.RFC1123), strings.TrimRight(baseURL, "/"), resetPasswordPath, link.Token)
	return nil
}

// prompter asks for what the flags left out: on a terminal with prompts and
// a hidden password, otherwise one line each from the input, so a script can
// pipe the password in.
type prompter struct {
	in       *bufio.Reader
	out      io.Writer
	terminal bool
	fd       int
}

func newPrompter(in *os.File, out io.Writer) *prompter {
	fd := int(in.Fd()) // #nosec G115 -- a file descriptor fits in an int.
	return &prompter{in: bufio.NewReader(in), out: out, terminal: term.IsTerminal(fd), fd: fd}
}

func (p *prompter) line(label string) (string, error) {
	if p.terminal {
		fmt.Fprint(p.out, label)
	}
	text, err := p.in.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || text == "") {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(strings.ToLower(label), ": "), err)
	}
	return strings.TrimRight(text, "\r\n"), nil
}

func (p *prompter) password() (string, error) {
	if !p.terminal {
		return p.line("Password: ")
	}
	read := func(label string) (string, error) {
		fmt.Fprint(p.out, label)
		b, err := term.ReadPassword(p.fd)
		fmt.Fprintln(p.out)
		return string(b), err
	}
	first, err := read("Password: ")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	again, err := read("Password again: ")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if first != again {
		return "", errors.New("the passwords do not match")
	}
	return first, nil
}

// accountPrompter is what createOwner asks for missing details.
type accountPrompter interface {
	line(label string) (string, error)
	password() (string, error)
}

// createOwner creates an owner account, asking for any detail not given.
func createOwner(ctx context.Context, svc *auth.Service, ask accountPrompter, out io.Writer, username, displayName string) error {
	var err error
	if username == "" {
		if username, err = ask.line("Username: "); err != nil {
			return err
		}
	}
	if displayName == "" {
		displayName = username
	}
	password, err := ask.password()
	if err != nil {
		return err
	}
	user, err := svc.CreateOwner(ctx, auth.CommandLine, username, displayName, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Created owner %q (%s). Sign in with this username in the web UI.\n", user.Username, user.DisplayName)
	return nil
}
