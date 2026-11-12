// Package pgtest provisions PostgreSQL databases for tests. It is the single
// place that starts the PostgreSQL testcontainer, so readiness rules live in
// one spot (#1299).
//
// A test gets a fresh, empty database on a shared server: the one named by
// MYFAMILY_TEST_POSTGRES_URL when set, otherwise one testcontainer started on
// first use and shared by every test in the process (the testcontainers reaper
// removes it when the process exits).
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // registers the "postgres" driver
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// EnvServerURL names an optional PostgreSQL server URL. When set, tests run
// against that server instead of a testcontainer, so a machine without Docker
// can still exercise the PostgreSQL backend.
const EnvServerURL = "MYFAMILY_TEST_POSTGRES_URL"

const (
	image          = "postgres:16-alpine"
	containerPort  = "5432/tcp"
	startupTimeout = 60 * time.Second
	readyTimeout   = 30 * time.Second
	retryInterval  = 200 * time.Millisecond
)

var (
	sharedOnce sync.Once
	sharedURL  string
	sharedErr  error
)

// ServerURL returns the URL of a PostgreSQL server that accepts connections.
// Without Docker or EnvServerURL it skips the test — except under CI, where a
// silent skip would hide that the PostgreSQL leg never ran (#879), so it fails
// instead. It also skips in -short mode.
func ServerURL(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PostgreSQL test in short mode")
	}
	if serverURL := os.Getenv(EnvServerURL); serverURL != "" {
		return serverURL
	}
	if exec.Command("docker", "info").Run() != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("PostgreSQL tests must run in CI: Docker is not available and " + EnvServerURL + " is unset")
		}
		t.Skip("Docker is not available and " + EnvServerURL + " is unset, skipping PostgreSQL test")
	}
	// Not t.Context(): the container outlives this test.
	sharedOnce.Do(func() { sharedURL, sharedErr = startContainer(context.Background()) })
	if sharedErr != nil {
		t.Fatalf("start postgres: %v", sharedErr)
	}
	return sharedURL
}

// NewDatabase creates a uniquely named, empty database on ServerURL and
// returns its connection URL. The database is dropped when the test ends;
// register the cleanup of any connection opened on it after calling this, so
// it closes before the drop.
func NewDatabase(t testing.TB) string {
	t.Helper()
	serverURL := ServerURL(t)

	admin, err := sql.Open("postgres", serverURL)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	name := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// #nosec G202 -- name is generated above from a UUID, never external input.
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil { // nosemgrep
		_ = admin.Close()
		t.Fatalf("create database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// #nosec G202 -- name is generated above from a UUID, never external input.
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)") // nosemgrep
		_ = admin.Close()
	})

	dsn, err := withDatabase(serverURL, name)
	if err != nil {
		t.Fatalf("parse postgres URL: %v", err)
	}
	return dsn
}

// withDatabase returns serverURL pointed at the database name.
func withDatabase(serverURL, name string) (string, error) {
	dsn, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	dsn.Path = "/" + name
	return dsn.String(), nil
}

// startContainer starts the PostgreSQL container and returns its URL once the
// host port is mapped and the server answers a ping. The log line alone is not
// enough: Docker can report the container before publishing its port, which
// surfaced as `port "5432/tcp" not found` (#1299).
func startContainer(ctx context.Context) (string, error) {
	container, err := postgres.Run(ctx, image,
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort(containerPort),
		).WithDeadline(startupTimeout)),
	)
	if err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	var connStr string
	err = retry(ctx, func(ctx context.Context) error {
		connStr, err = container.ConnectionString(ctx, "sslmode=disable")
		return err
	})
	if err != nil {
		_ = container.Terminate(ctx)
		return "", fmt.Errorf("connection string: %w", err)
	}
	if err := ping(ctx, connStr); err != nil {
		_ = container.Terminate(ctx)
		return "", err
	}
	return connStr, nil
}

// ping waits until the server at connStr accepts connections.
func ping(ctx context.Context, connStr string) error {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if err := retry(ctx, db.PingContext); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

// retry calls fn until it succeeds or readyTimeout passes, returning the last
// error. fn gets the bounded context, so a hung attempt cannot outlast it.
func retry(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(retryInterval):
		}
	}
}
