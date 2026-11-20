package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

func TestPrivacyFor(t *testing.T) {
	now := time.Date(2026, time.November, 19, 15, 0, 0, 0, time.UTC)
	seeLiving := DefaultTreeSettings()
	hideLiving := DefaultTreeSettings()
	hideLiving.ViewersSeeLiving = false
	hideLiving.LivingThresholdYears = 90
	tests := []struct {
		name      string
		principal Principal
		settings  TreeSettings
		want      Privacy
	}{
		{"owner", Principal{Role: RoleOwner}, hideLiving, Privacy{}},
		{"editor", Principal{Role: RoleEditor}, hideLiving, Privacy{}},
		{"viewer who sees living", Principal{Role: RoleViewer}, seeLiving, Privacy{HidePrivate: true}},
		{"viewer who does not", Principal{Role: RoleViewer}, hideLiving,
			Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Date(1936, time.November, 19, 0, 0, 0, 0, time.UTC)}},
		{"anonymous visitor", Principal{Role: RoleViewer, Anonymous: true}, seeLiving,
			Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Date(1916, time.November, 19, 0, 0, 0, 0, time.UTC), HideSubmitters: true, HideAttribution: true}},
		{"anonymous owner role", Principal{Role: RoleOwner, Anonymous: true}, seeLiving,
			Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Date(1916, time.November, 19, 0, 0, 0, 0, time.UTC), HideSubmitters: true, HideAttribution: true}},
		{"no role", Principal{}, seeLiving,
			Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Date(1916, time.November, 19, 0, 0, 0, 0, time.UTC), HideSubmitters: true, HideAttribution: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrivacyFor(tt.principal, tt.settings, now)
			if got != tt.want {
				t.Errorf("PrivacyFor = %+v, want %+v", got, tt.want)
			}
			if got.SeesAll() != (tt.want == Privacy{}) {
				t.Errorf("SeesAll = %v for %+v", got.SeesAll(), got)
			}
		})
	}
}

// A context with no principal is an anonymous visitor's (ADR-008 §4).
func TestPrivacyFrom_MissingPrincipalFailsSafe(t *testing.T) {
	now := time.Now()
	if got := PrincipalFrom(context.Background()); !got.Anonymous {
		t.Errorf("PrincipalFrom(no principal) = %+v, want anonymous", got)
	}
	if got := PrivacyFrom(context.Background(), now); !got.HideLiving || !got.HidePrivate {
		t.Errorf("PrivacyFrom(no principal) = %+v, want everything protected hidden", got)
	}
	ctx := ContextWithPrincipal(context.Background(), Principal{Role: RoleEditor})
	if got := PrivacyFrom(ctx, now); !got.SeesAll() {
		t.Errorf("PrivacyFrom(editor) = %+v, want nothing hidden", got)
	}
}

func TestPrivacy_Person(t *testing.T) {
	cutoff := time.Date(1916, time.January, 1, 0, 0, 0, 0, time.UTC)
	old := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)
	living := &PersonReadModel{LivingRefDate: &recent}
	dead := &PersonReadModel{LivingRefDate: &old}
	private := &PersonReadModel{LivingRefDate: &old, Restriction: "locked, Confidential"}
	locked := &PersonReadModel{LivingRefDate: &old, Restriction: "locked"}
	tests := []struct {
		name    string
		privacy Privacy
		person  *PersonReadModel
		want    Visibility
	}{
		{"nothing hidden", Privacy{}, private, Visible},
		{"living, living hidden", Privacy{HideLiving: true, LivingCutoff: cutoff}, living, Placeholder},
		{"living, only private hidden", Privacy{HidePrivate: true}, living, Visible},
		{"dead", Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: cutoff}, dead, Visible},
		{"private", Privacy{HidePrivate: true}, private, Hidden},
		{"private, only living hidden", Privacy{HideLiving: true, LivingCutoff: cutoff}, private, Visible},
		{"locked is not private", Privacy{HidePrivate: true}, locked, Visible},
		{"unresolved", Privacy{HidePrivate: true}, nil, Hidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.privacy.Person(tt.person); got != tt.want {
				t.Errorf("Person = %v, want %v", got, tt.want)
			}
			if got := tt.privacy.Protects(tt.person); got != (tt.want != Visible) {
				t.Errorf("Protects = %v, want %v", got, tt.want != Visible)
			}
		})
	}
}

func TestPlaceholderPerson_KeepsOnlyIDAndSex(t *testing.T) {
	ref := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)
	p := &PersonReadModel{
		ID: uuid.New(), GivenName: "Ada", Surname: "Byron", FullName: "Ada Byron", Gender: domain.GenderFemale,
		BirthDateRaw: "1990", BirthPlace: "London", Notes: "n", Restriction: "confidential",
		LivingRefDate: &ref, ResearchStatus: domain.ResearchStatusCertain, Version: 3, UpdatedAt: time.Now(),
	}
	got := PlaceholderPerson(p, Placeholder)
	want := &PersonReadModel{ID: p.ID, GivenName: "Living", FullName: "Living", Gender: domain.GenderFemale, Version: 3, Visibility: Placeholder}
	if *got != *want {
		t.Errorf("PlaceholderPerson = %+v, want %+v", got, want)
	}
}
