package api

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The JSON export's person converter carries the living verdict, the override
// and the private flag, as GET /persons does (ADR-008).
func TestConvertReadModelPersonToGenerated_CarriesPrivacy(t *testing.T) {
	cutoff := repository.DefaultTreeSettings().LivingCutoff(time.Now())
	private := convertReadModelPersonToGenerated(repository.PersonReadModel{
		ID:             uuid.New(),
		Restriction:    "confidential",
		LivingOverride: domain.LivingOverrideDeceased,
	}, cutoff)
	if private.Private == nil || !*private.Private {
		t.Errorf("Private = %v, want true", private.Private)
	}
	if private.Living == nil || *private.Living {
		t.Errorf("Living = %v, want false", private.Living)
	}
	if private.LivingOverride == nil || *private.LivingOverride != LivingOverrideDeceased {
		t.Errorf("LivingOverride = %v, want deceased", private.LivingOverride)
	}

	public := convertReadModelPersonToGenerated(repository.PersonReadModel{ID: uuid.New()}, cutoff)
	if public.Private != nil {
		t.Errorf("Private = %v, want omitted", *public.Private)
	}
	if public.Living == nil || !*public.Living {
		t.Errorf("Living = %v, want true for a person with no dates", public.Living)
	}
	if public.LivingOverride == nil || *public.LivingOverride != LivingOverrideAuto {
		t.Errorf("LivingOverride = %v, want auto", public.LivingOverride)
	}
}

// The living verdict follows the cutoff it is given, which the tree's
// threshold sets (ADR-008 §6).
func TestConvertReadModelPersonToGenerated_JudgesLivingAtTheCutoff(t *testing.T) {
	born := time.Date(1930, 6, 1, 0, 0, 0, 0, time.UTC)
	rm := repository.PersonReadModel{ID: uuid.New(), LivingRefDate: &born}
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	for years, want := range map[int]bool{110: true, 90: false} {
		cutoff := repository.TreeSettings{LivingThresholdYears: years}.LivingCutoff(now)
		if got := convertReadModelPersonToGenerated(rm, cutoff).Living; got == nil || *got != want {
			t.Errorf("born 1930, threshold %d: Living = %v, want %v", years, got, want)
		}
	}
}
