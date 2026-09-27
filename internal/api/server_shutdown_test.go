package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

// startBlockingServer starts a real server on a free port with a /slow route
// that blocks until release is closed. It returns the server, the route URL
// and a channel that receives once the handler has been entered.
func startBlockingServer(t *testing.T, release <-chan struct{}) (*api.Server, string, <-chan struct{}) {
	t.Helper()
	events := memory.NewEventStore()
	srv := api.NewServer(&config.Config{Port: 0, LogFormat: "text"},
		events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
	entered := make(chan struct{}, 1)
	srv.Echo().GET("/slow", func(c echo.Context) error {
		entered <- struct{}{}
		<-release
		return c.String(http.StatusOK, "done")
	})

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start() }()
	var addr string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if a := srv.Echo().ListenerAddr(); a != nil {
			addr = a.String()
			break
		}
	}
	if addr == "" {
		t.Fatal("server did not start listening")
	}
	t.Cleanup(func() {
		_ = srv.Echo().Close()
		if err := <-startErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Start: %v", err)
		}
	})
	return srv, "http://" + addr + "/slow", entered
}

func TestServerShutdown_WaitsForInFlightRequests(t *testing.T) {
	release := make(chan struct{})
	srv, url, entered := startBlockingServer(t, release)

	type result struct {
		body string
		err  error
	}
	resp := make(chan result, 1)
	go func() {
		r, err := http.Get(url) //nolint:gosec,noctx // test URL on localhost
		if err != nil {
			resp <- result{err: err}
			return
		}
		defer r.Body.Close()
		b, err := io.ReadAll(r.Body)
		resp <- result{string(b), err}
	}()
	<-entered

	shutdown := make(chan error, 1)
	go func() { shutdown <- srv.Shutdown(context.Background()) }()

	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned (%v) while a request was still in flight", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	if err := <-shutdown; err != nil {
		t.Fatalf("Shutdown = %v, want nil", err)
	}
	r := <-resp
	if r.err != nil || r.body != "done" {
		t.Fatalf("in-flight request = (%q, %v), want it to complete with \"done\"", r.body, r.err)
	}
}

func TestServerShutdown_TimeoutForcesClose(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	srv, url, entered := startBlockingServer(t, release)

	go func() {
		if r, err := http.Get(url); err == nil { //nolint:gosec,noctx // test URL on localhost
			_ = r.Body.Close()
		}
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := srv.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown = %v, want it to report the deadline", err)
	}
}
