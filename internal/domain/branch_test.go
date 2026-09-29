package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
			// Limits count characters, not bytes: 500 two-byte characters
			// (1000 bytes) is a valid description, as the API's maxLength says.
			name: "multibyte text at the limits",
			branch: &domain.Branch{
				Name:        strings.Repeat("é", 100),
				Description: strings.Repeat("é", 500),
				MergeNote:   strings.Repeat("é", 1000),
				Status:      domain.BranchStatusActive,
			},
			wantErr: nil,
		},
		{
			name: "multibyte description too long",
			branch: &domain.Branch{
				Name:        "Valid",
				Description: strings.Repeat("é", 501),
				Status:      domain.BranchStatusActive,
			},
			wantErr: domain.ErrBranchDescTooLong,
		},
		{
			name: "multibyte name too long",
			branch: &domain.Branch{
				Name:   strings.Repeat("é", 101),
				Status: domain.BranchStatusActive,
			},
			wantErr: domain.ErrBranchNameTooLong,
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
		{
			name: "merge note at max length",
			branch: &domain.Branch{
				Name:      "Valid",
				Status:    domain.BranchStatusMerged,
				MergeNote: strings.Repeat("x", 1000),
			},
			wantErr: nil,
		},
		{
			name: "merge note too long",
			branch: &domain.Branch{
				Name:      "Valid",
				Status:    domain.BranchStatusMerged,
				MergeNote: strings.Repeat("x", 1001),
			},
			wantErr: domain.ErrBranchMergeNoteTooLong,
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

// TestBranch_MergeFieldsJSON pins the omitempty tags on the merge metadata: an
// unmerged branch must not advertise a merge it never had, and a merged one
// must round-trip both fields.
func TestBranch_MergeFieldsJSON(t *testing.T) {
	active, err := domain.NewBranch("Hypothesis A", "", 42)
	if err != nil {
		t.Fatalf("NewBranch() unexpected error = %v", err)
	}

	if active.MergedAt != nil {
		t.Errorf("MergedAt = %v, want nil on a new branch", active.MergedAt)
	}
	if active.MergeNote != "" {
		t.Errorf("MergeNote = %q, want empty on a new branch", active.MergeNote)
	}

	data, err := json.Marshal(active)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if strings.Contains(string(data), "merged_at") {
		t.Errorf("merged_at should be omitted when unset, got %s", data)
	}
	if strings.Contains(string(data), "merge_note") {
		t.Errorf("merge_note should be omitted when unset, got %s", data)
	}

	mergedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	merged := *active
	merged.Status = domain.BranchStatusMerged
	merged.MergedAt = &mergedAt
	merged.MergeNote = "Confirmed by the 1880 census"

	data, err = json.Marshal(merged)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded domain.Branch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.MergedAt == nil || !decoded.MergedAt.Equal(mergedAt) {
		t.Errorf("MergedAt = %v, want %v", decoded.MergedAt, mergedAt)
	}
	if decoded.MergeNote != merged.MergeNote {
		t.Errorf("MergeNote = %q, want %q", decoded.MergeNote, merged.MergeNote)
	}
}

func TestNormalizeResolutionRationales(t *testing.T) {
	kept, blank := uuid.New(), uuid.New()

	got, err := domain.NormalizeResolutionRationales(map[uuid.UUID]string{kept: "  census 1881 ", blank: " \t"})
	if err != nil {
		t.Fatalf("NormalizeResolutionRationales failed: %v", err)
	}
	if len(got) != 1 || got[kept] != "census 1881" {
		t.Errorf("got %v, want only the trimmed non-blank rationale", got)
	}

	if got, err := domain.NormalizeResolutionRationales(map[uuid.UUID]string{blank: ""}); err != nil || got != nil {
		t.Errorf("all blank: got %v, %v; want nil, nil", got, err)
	}

	// Counted in characters, not bytes: 1000 multi-byte runes fit.
	if _, err := domain.NormalizeResolutionRationales(map[uuid.UUID]string{kept: strings.Repeat("é", domain.MaxResolutionRationaleLength)}); err != nil {
		t.Errorf("rationale at the limit: %v", err)
	}
	if _, err := domain.NormalizeResolutionRationales(map[uuid.UUID]string{kept: strings.Repeat("x", domain.MaxResolutionRationaleLength+1)}); !errors.Is(err, domain.ErrResolutionRationaleTooLong) {
		t.Errorf("rationale over the limit: err = %v, want ErrResolutionRationaleTooLong", err)
	}
}

// BranchMerged's rationales are additive: a claim recorded before #828 decodes
// with none, and one with rationales round-trips them.
func TestBranchMerged_ResolutionRationalesAreAdditive(t *testing.T) {
	var old domain.BranchMerged
	if err := json.Unmarshal([]byte(`{"branch_id":"`+uuid.NewString()+`","replay_stream_versions":{}}`), &old); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if old.ResolutionRationales != nil {
		t.Errorf("pre-#828 claim decoded rationales %v, want nil", old.ResolutionRationales)
	}

	stream := uuid.New()
	event := domain.NewBranchMerged(uuid.New(), 1, 2, "", nil)
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(encoded), "resolution_rationales") {
		t.Errorf("a claim without rationales encodes the key: %s", encoded)
	}
	event.ResolutionRationales = map[uuid.UUID]string{stream: "why"}
	if encoded, err = json.Marshal(event); err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded domain.BranchMerged
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ResolutionRationales[stream] != "why" {
		t.Errorf("round-tripped rationales = %v", decoded.ResolutionRationales)
	}
}
