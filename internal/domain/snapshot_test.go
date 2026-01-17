package domain_test

import (
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestNewSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		snapshotName string
		description  string
		position     int64
		wantErr      error
	}{
		{
			name:         "valid snapshot",
			snapshotName: "Pre-DNA results",
			description:  "Research state before DNA test results arrived",
			position:     42,
			wantErr:      nil,
		},
		{
			name:         "valid snapshot without description",
			snapshotName: "Milestone",
			description:  "",
			position:     10,
			wantErr:      nil,
		},
		{
			name:         "empty name",
			snapshotName: "",
			description:  "Some description",
			position:     5,
			wantErr:      domain.ErrSnapshotNameRequired,
		},
		{
			name:         "name too long",
			snapshotName: strings.Repeat("a", 101),
			description:  "",
			position:     5,
			wantErr:      domain.ErrSnapshotNameTooLong,
		},
		{
			name:         "description too long",
			snapshotName: "Valid name",
			description:  strings.Repeat("a", 501),
			position:     5,
			wantErr:      domain.ErrSnapshotDescTooLong,
		},
		{
			name:         "name at max length",
			snapshotName: strings.Repeat("a", 100),
			description:  "",
			position:     5,
			wantErr:      nil,
		},
		{
			name:         "description at max length",
			snapshotName: "Valid name",
			description:  strings.Repeat("a", 500),
			position:     5,
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := domain.NewSnapshot(tt.snapshotName, tt.description, tt.position)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("NewSnapshot() error = nil, want %v", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("NewSnapshot() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("NewSnapshot() unexpected error = %v", err)
				return
			}

			if snapshot.Name != tt.snapshotName {
				t.Errorf("Name = %q, want %q", snapshot.Name, tt.snapshotName)
			}
			if snapshot.Description != tt.description {
				t.Errorf("Description = %q, want %q", snapshot.Description, tt.description)
			}
			if snapshot.Position != tt.position {
				t.Errorf("Position = %d, want %d", snapshot.Position, tt.position)
			}
			if snapshot.ID == [16]byte{} {
				t.Error("ID should not be zero")
			}
			if snapshot.CreatedAt.IsZero() {
				t.Error("CreatedAt should not be zero")
			}
		})
	}
}

func TestSnapshot_Validate(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *domain.Snapshot
		wantErr  error
	}{
		{
			name: "valid snapshot",
			snapshot: &domain.Snapshot{
				Name:        "Test",
				Description: "Description",
			},
			wantErr: nil,
		},
		{
			name: "empty name",
			snapshot: &domain.Snapshot{
				Name:        "",
				Description: "Description",
			},
			wantErr: domain.ErrSnapshotNameRequired,
		},
		{
			name: "name too long",
			snapshot: &domain.Snapshot{
				Name:        strings.Repeat("x", 101),
				Description: "",
			},
			wantErr: domain.ErrSnapshotNameTooLong,
		},
		{
			name: "description too long",
			snapshot: &domain.Snapshot{
				Name:        "Valid",
				Description: strings.Repeat("x", 501),
			},
			wantErr: domain.ErrSnapshotDescTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.snapshot.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
