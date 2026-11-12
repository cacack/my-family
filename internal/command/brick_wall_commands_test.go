package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// TestBrickWall_SetAndResolveAreLogged is the #802 regression: marking and
// resolving a brick wall append BrickWallSet and BrickWallResolved to the
// person's stream, and the projection writes the person's row from them.
func TestBrickWall_SetAndResolveAreLogged(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	id := newDismissalPerson(t, f.handler, "Heinrich")

	if err := f.handler.SetBrickWall(ctx, id, "no baptism found"); err != nil {
		t.Fatalf("SetBrickWall: %v", err)
	}
	p, _ := f.readStore.GetPerson(ctx, domain.MainBranchID, id)
	if p == nil || p.BrickWallNote != "no baptism found" || p.BrickWallSince == nil || p.BrickWallResolvedAt != nil {
		t.Fatalf("person after set = %+v, want an unresolved brick wall", p)
	}
	since := *p.BrickWallSince

	if err := f.handler.ResolveBrickWall(ctx, id); err != nil {
		t.Fatalf("ResolveBrickWall: %v", err)
	}
	p, _ = f.readStore.GetPerson(ctx, domain.MainBranchID, id)
	if p == nil || p.BrickWallNote != "no baptism found" || p.BrickWallSince == nil || !p.BrickWallSince.Equal(since) || p.BrickWallResolvedAt == nil {
		t.Fatalf("person after resolve = %+v, want the brick wall resolved", p)
	}

	stream, err := f.eventStore.ReadStream(ctx, id)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	var types []string
	for i := range stream {
		types = append(types, stream[i].EventType)
	}
	if n := len(types); n < 2 || types[n-2] != "BrickWallSet" || types[n-1] != "BrickWallResolved" {
		t.Errorf("person stream = %v, want it to end BrickWallSet, BrickWallResolved", types)
	}
	if p.Version != int64(len(stream)) {
		t.Errorf("person version = %d, want the stream's %d", p.Version, len(stream))
	}

	// The row's version follows the stream, so an ordinary edit still applies.
	if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: id, Version: p.Version, Notes: command.Set("checked the parish")}); err != nil {
		t.Errorf("UpdatePerson after brick wall: %v", err)
	}
}

// TestResolveBrickWall_WithoutOneIsANoOp keeps the endpoint's old leniency: a
// person with no unresolved brick wall is left as it is, with no event.
func TestResolveBrickWall_WithoutOneIsANoOp(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	id := newDismissalPerson(t, f.handler, "Anna")
	before, _ := f.eventStore.ReadStream(ctx, id)

	if err := f.handler.ResolveBrickWall(ctx, id); err != nil {
		t.Fatalf("ResolveBrickWall: %v", err)
	}
	if after, _ := f.eventStore.ReadStream(ctx, id); len(after) != len(before) {
		t.Errorf("stream grew from %d to %d events, want no event", len(before), len(after))
	}
}

func TestBrickWall_UnknownPerson(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	if err := f.handler.SetBrickWall(ctx, uuid.New(), "note"); !errors.Is(err, command.ErrPersonNotFound) {
		t.Errorf("SetBrickWall error = %v, want ErrPersonNotFound", err)
	}
	if err := f.handler.ResolveBrickWall(ctx, uuid.New()); !errors.Is(err, command.ErrPersonNotFound) {
		t.Errorf("ResolveBrickWall error = %v, want ErrPersonNotFound", err)
	}
}

// TestBrickWall_RefusedOnBranch pins brick walls to main: their events are not
// branch-aware, so a branch-scoped handler refuses them (branch-scoping them is
// follow-up under #676).
func TestBrickWall_RefusedOnBranch(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	id := newDismissalPerson(t, f.handler, "Anna")
	branch, err := f.handler.CreateBranch(ctx, "walls", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := f.handler.WithBranch(branch).SetBrickWall(ctx, id, "note"); !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("SetBrickWall on a branch error = %v, want ErrEventTypeNotBranchAware", err)
	}
}
