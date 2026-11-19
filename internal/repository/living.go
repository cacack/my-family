package repository

import (
	"github.com/cacack/my-family/internal/domain"
)

// SetLivingFacts sets the projected inputs of the living rule on person from
// its own fields and its life events (ADR-008 §1): DeathEvidence when it has
// a death, burial or cremation that is not negated, with or without a date;
// LivingRefDate from the birth date, else the christening and baptism dates
// (domain.LivingReferenceDate), clamped to what every backend can store.
// events are the person's life events on the same branch.
func SetLivingFacts(person *PersonReadModel, events []EventReadModel) {
	person.DeathEvidence = person.DeathDateRaw != "" || person.DeathPlace != "" ||
		person.DeathNotes != "" || person.DeathTypeDetail != ""
	var birth *domain.GenDate
	if person.BirthDateRaw != "" {
		gd := domain.ParseGenDate(person.BirthDateRaw)
		birth = &gd
	}
	var christenings []*domain.GenDate
	for _, e := range events {
		if e.IsNegated {
			continue
		}
		switch e.FactType {
		case domain.FactPersonDeath, domain.FactPersonBurial, domain.FactPersonCremation:
			person.DeathEvidence = true
		case domain.FactPersonChristening, domain.FactPersonBaptism:
			if e.DateRaw != "" {
				gd := domain.ParseGenDate(e.DateRaw)
				christenings = append(christenings, &gd)
			}
		}
	}
	person.LivingRefDate = domain.LivingReferenceDate(birth, christenings)
	if person.LivingRefDate != nil && person.LivingRefDate.Before(domain.EarliestSortDate) {
		clamped := domain.EarliestSortDate
		person.LivingRefDate = &clamped
	}
}

// livingFactTypes are the life events SetLivingFacts reads.
var livingFactTypes = map[domain.FactType]bool{
	domain.FactPersonDeath:       true,
	domain.FactPersonBurial:      true,
	domain.FactPersonCremation:   true,
	domain.FactPersonChristening: true,
	domain.FactPersonBaptism:     true,
}
