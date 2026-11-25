package gedcom

import (
	"fmt"

	"github.com/cacack/gedcom-go/v3/gedcom"

	"github.com/cacack/my-family/internal/repository"
)

// placeCoordinates returns the MAP coordinates of a PLAC, each half nil when
// absent. A pair under a PLAC that names no place (repository.PlaceKey is "")
// is kept as the record's own pair, so export can write it back, and reported
// as a warning naming the record and the event or attribute tag: the map
// leaves such a pair out, having no place to name it by (#1807).
func placeCoordinates(detail *gedcom.PlaceDetail, record, tag string, result *ImportResult) (lat, long *string) {
	if detail == nil || detail.Coordinates == nil {
		return nil, nil
	}
	if v := detail.Coordinates.Latitude; v != "" {
		lat = &v
	}
	if v := detail.Coordinates.Longitude; v != "" {
		long = &v
	}
	if (lat != nil || long != nil) && repository.PlaceKey(detail.Name) == "" {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%s: %s coordinates have no place name; kept on the record", record, tag))
	}
	return lat, long
}
