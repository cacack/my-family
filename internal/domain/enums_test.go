package domain

import "testing"

func TestGender_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		gender Gender
		want   bool
	}{
		{
			name:   "male is valid",
			gender: GenderMale,
			want:   true,
		},
		{
			name:   "female is valid",
			gender: GenderFemale,
			want:   true,
		},
		{
			name:   "unknown is valid",
			gender: GenderUnknown,
			want:   true,
		},
		{
			name:   "empty string is valid",
			gender: "",
			want:   true,
		},
		{
			name:   "invalid value",
			gender: "invalid",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gender.IsValid(); got != tt.want {
				t.Errorf("Gender(%q).IsValid() = %v, want %v", tt.gender, got, tt.want)
			}
		})
	}
}

func TestRelationType_IsValid(t *testing.T) {
	tests := []struct {
		name         string
		relationType RelationType
		want         bool
	}{
		{
			name:         "marriage is valid",
			relationType: RelationMarriage,
			want:         true,
		},
		{
			name:         "partnership is valid",
			relationType: RelationPartnership,
			want:         true,
		},
		{
			name:         "unknown is valid",
			relationType: RelationUnknown,
			want:         true,
		},
		{
			name:         "empty string is valid",
			relationType: "",
			want:         true,
		},
		{
			name:         "invalid value",
			relationType: "divorce",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.relationType.IsValid(); got != tt.want {
				t.Errorf("RelationType(%q).IsValid() = %v, want %v", tt.relationType, got, tt.want)
			}
		})
	}
}

func TestChildRelationType_IsValid(t *testing.T) {
	tests := []struct {
		name              string
		childRelationType ChildRelationType
		want              bool
	}{
		{
			name:              "biological is valid",
			childRelationType: ChildBiological,
			want:              true,
		},
		{
			name:              "adopted is valid",
			childRelationType: ChildAdopted,
			want:              true,
		},
		{
			name:              "foster is valid",
			childRelationType: ChildFoster,
			want:              true,
		},
		{
			name:              "empty string is invalid",
			childRelationType: "",
			want:              false,
		},
		{
			name:              "invalid value",
			childRelationType: "stepchild",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.childRelationType.IsValid(); got != tt.want {
				t.Errorf("ChildRelationType(%q).IsValid() = %v, want %v", tt.childRelationType, got, tt.want)
			}
		})
	}
}
