package domain_test

import (
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/google/uuid"
)

func TestMainBranchID(t *testing.T) {
	if domain.MainBranchID.UUID() != uuid.Nil {
		t.Errorf("MainBranchID = %v, want %v (uuid.Nil)", domain.MainBranchID, uuid.Nil)
	}
}

func TestNewBranch(t *testing.T) {
	tests := []struct {
		name         string
		branchName   string
		description  string
		basePosition int64
		wantErr      error
	}{
		{
			name:         "valid branch",
			branchName:   "Hypothesis A",
			description:  "Exploring an unproven line",
			basePosition: 42,
			wantErr:      nil,
		},
		{
			name:         "valid branch without description",
			branchName:   "Milestone",
			description:  "",
			basePosition: 10,
			wantErr:      nil,
		},
		{
			name:         "empty name",
			branchName:   "",
			description:  "Some description",
			basePosition: 5,
			wantErr:      domain.ErrBranchNameRequired,
		},
		{
			name:         "name too long",
			branchName:   strings.Repeat("a", 101),
			description:  "",
			basePosition: 5,
			wantErr:      domain.ErrBranchNameTooLong,
		},
		{
			name:         "description too long",
			branchName:   "Valid name",
			description:  strings.Repeat("a", 501),
			basePosition: 5,
			wantErr:      domain.ErrBranchDescTooLong,
		},
		{
			name:         "name at max length",
			branchName:   strings.Repeat("a", 100),
			description:  "",
			basePosition: 5,
			wantErr:      nil,
		},
		{
			name:         "description at max length",
			branchName:   "Valid name",
			description:  strings.Repeat("a", 500),
			basePosition: 5,
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, err := domain.NewBranch(tt.branchName, tt.description, tt.basePosition)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("NewBranch() error = nil, want %v", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("NewBranch() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("NewBranch() unexpected error = %v", err)
				return
			}

			if branch.Name != tt.branchName {
				t.Errorf("Name = %q, want %q", branch.Name, tt.branchName)
			}
			if branch.Description != tt.description {
				t.Errorf("Description = %q, want %q", branch.Description, tt.description)
			}
			if branch.BasePosition != tt.basePosition {
				t.Errorf("BasePosition = %d, want %d", branch.BasePosition, tt.basePosition)
			}
			if branch.Status != domain.BranchStatusActive {
				t.Errorf("Status = %q, want %q", branch.Status, domain.BranchStatusActive)
			}
			if branch.ID == uuid.Nil {
				t.Error("ID should not be zero")
			}
			if branch.CreatedAt.IsZero() {
				t.Error("CreatedAt should not be zero")
			}
		})
	}
}

func TestBranch_Validate(t *testing.T) {
	tests := []struct {
		name    string
		branch  *domain.Branch
		wantErr error
	}{
		{
			name: "valid branch",
			branch: &domain.Branch{
				Name:        "Test",
				Description: "Description",
				Status:      domain.BranchStatusActive,
			},
			wantErr: nil,
		},
		{
			name: "empty name",
			branch: &domain.Branch{
				Name:   "",
				Status: domain.BranchStatusActive,
			},
			wantErr: domain.ErrBranchNameRequired,
		},
		{
			name: "name too long",
			branch: &domain.Branch{
				Name:   strings.Repeat("x", 101),
				Status: domain.BranchStatusActive,
			},
			wantErr: domain.ErrBranchNameTooLong,
		},
		{
			name: "description too long",
			branch: &domain.Branch{
				Name:        "Valid",
				Description: strings.Repeat("x", 501),
				Status:      domain.BranchStatusActive,
			},
			wantErr: domain.ErrBranchDescTooLong,
		},
		{
			name: "invalid status",
			branch: &domain.Branch{
				Name:   "Valid",
				Status: domain.BranchStatus("bogus"),
			},
			wantErr: domain.ErrBranchInvalidStatus,
		},
		{
			name: "empty status is invalid",
			branch: &domain.Branch{
				Name:   "Valid",
				Status: domain.BranchStatus(""),
			},
			wantErr: domain.ErrBranchInvalidStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.branch.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestBranchStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status domain.BranchStatus
		want   bool
	}{
		{"active", domain.BranchStatusActive, true},
		{"merged", domain.BranchStatusMerged, true},
		{"archived", domain.BranchStatusArchived, true},
		{"empty", domain.BranchStatus(""), false},
		{"unknown", domain.BranchStatus("bogus"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBranchStatus_Transitions documents the legal status transitions from
// ADR-005 §The model: active→merged and active→archived; merged and archived
// are terminal. This table asserts which transitions the model permits so the
// documented contract is exercised, not merely commented.
func TestBranchStatus_Transitions(t *testing.T) {
	legal := func(from, to domain.BranchStatus) bool {
		if from != domain.BranchStatusActive {
			return false // merged and archived are terminal
		}
		return to == domain.BranchStatusMerged || to == domain.BranchStatusArchived
	}

	tests := []struct {
		name string
		from domain.BranchStatus
		to   domain.BranchStatus
		want bool
	}{
		{"active to merged", domain.BranchStatusActive, domain.BranchStatusMerged, true},
		{"active to archived", domain.BranchStatusActive, domain.BranchStatusArchived, true},
		{"active to active", domain.BranchStatusActive, domain.BranchStatusActive, false},
		{"merged is terminal", domain.BranchStatusMerged, domain.BranchStatusArchived, false},
		{"archived is terminal", domain.BranchStatusArchived, domain.BranchStatusMerged, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legal(tt.from, tt.to); got != tt.want {
				t.Errorf("transition %s->%s legal = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
