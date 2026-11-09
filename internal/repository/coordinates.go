package repository

import (
	"log/slog"

	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"
)

// ParseMapCoordinates parses a stored latitude/longitude pair for a map pin
// (#801). Each value is checked against its own axis, so a latitude carrying
// E/W, a longitude carrying N/S, or a value out of range is rejected rather
// than plotted in the wrong place. A rejected pair is logged with the person,
// the event type and the raw values, so a missing pin can be traced to its
// record; ok is false and the caller skips the pin.
func ParseMapCoordinates(personID uuid.UUID, eventType, rawLat, rawLon string) (lat, lon float64, ok bool) {
	lat, errLat := gedcom.ParseLatitude(rawLat)
	lon, errLon := gedcom.ParseLongitude(rawLon)
	if errLat != nil || errLon != nil {
		slog.Warn("map location skipped: invalid coordinates",
			"person_id", personID,
			"event_type", eventType,
			"latitude", rawLat,
			"longitude", rawLon,
			"latitude_error", errString(errLat),
			"longitude_error", errString(errLon))
		return 0, 0, false
	}
	return lat, lon, true
}

// errString is err's message, or "" for nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
