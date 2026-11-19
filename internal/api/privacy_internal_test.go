package api

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The JSON export's person converter carries the living verdict, the override
// and the private flag, as GET /persons does (ADR-008).
func TestConvertReadModelPersonToGenerated_CarriesPrivacy(t *testing.T) {
	private := convertReadModelPersonToGenerated(repository.PersonReadModel{
		ID:             uuid.New(),
		Restriction:    "confidential",
		LivingOverride: domain.LivingOverrideDeceased,
	})
	if private.Private == nil || !*private.Private {
		t.Errorf("Private = %v, want true", private.Private)
	}
	if private.Living == nil || *private.Living {
		t.Errorf("Living = %v, want false", private.Living)
	}
	if private.LivingOverride == nil || *private.LivingOverride != LivingOverrideDeceased {
		t.Errorf("LivingOverride = %v, want deceased", private.LivingOverride)
	}

	public := convertReadModelPersonToGenerated(repository.PersonReadModel{ID: uuid.New()})
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
