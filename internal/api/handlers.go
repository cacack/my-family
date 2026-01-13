package api

import (
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
)

// convertQueryAhnentafelEntryToGenerated converts a query AhnentafelEntry to the generated AhnentafelEntry type.
func convertQueryAhnentafelEntryToGenerated(entry query.AhnentafelEntry) AhnentafelEntry {
	resp := AhnentafelEntry{
		Number:       entry.Number,
		Generation:   entry.Generation,
		Relationship: getRelationLabel(entry.Number),
	}

	// Only include ID if the person is known (not nil UUID)
	if entry.ID != [16]byte{} {
		id := entry.ID
		resp.Id = &id
		resp.GivenName = &entry.GivenName
		resp.Surname = &entry.Surname
		gender := AhnentafelEntryGender(entry.Gender)
		resp.Gender = &gender

		if entry.BirthDate != nil {
			resp.BirthDate = convertDomainGenDateToGenerated(entry.BirthDate)
		}
		if entry.BirthPlace != nil {
			resp.BirthPlace = entry.BirthPlace
		}
		if entry.DeathDate != nil {
			resp.DeathDate = convertDomainGenDateToGenerated(entry.DeathDate)
		}
		if entry.DeathPlace != nil {
			resp.DeathPlace = entry.DeathPlace
		}
	}

	return resp
}

// convertDomainGenDateToGenerated converts a domain.GenDate to the generated GenDate type.
func convertDomainGenDateToGenerated(qd *domain.GenDate) *GenDate {
	if qd == nil {
		return nil
	}
	gd := &GenDate{}
	if qd.Year != nil {
		gd.Year = qd.Year
	}
	if qd.Month != nil {
		gd.Month = qd.Month
	}
	if qd.Day != nil {
		gd.Day = qd.Day
	}
	if qd.Qualifier != "" {
		q := GenDateQualifier(string(qd.Qualifier))
		gd.Qualifier = &q
	}
	if qd.Raw != "" {
		gd.Raw = &qd.Raw
	}
	return gd
}

// getRelationLabel returns the relationship label for a given Ahnentafel number.
func getRelationLabel(num int) string {
	if num == 1 {
		return ""
	}
	if num == 2 {
		return "Father"
	}
	if num == 3 {
		return "Mother"
	}

	// For higher numbers, build the relationship string
	// Start from the person and work backwards to find the path
	var path []string
	n := num
	for n > 1 {
		if n%2 == 0 {
			path = append([]string{"Father"}, path...)
		} else {
			path = append([]string{"Mother"}, path...)
		}
		n /= 2
	}

	// Convert path to a label like "Father's Father" or "Mother's Mother"
	if len(path) == 0 {
		return ""
	}

	result := path[0]
	for i := 1; i < len(path); i++ {
		result += "'s " + path[i]
	}
	return result
}
