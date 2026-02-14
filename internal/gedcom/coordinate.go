package gedcom

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseGEDCOMCoordinate converts a GEDCOM coordinate string to decimal degrees.
// GEDCOM format uses a direction prefix: "N42.3601", "S33.8688", "W71.0589", "E151.2093".
// N/E are positive, S/W are negative. Returns an error for invalid formats.
func ParseGEDCOMCoordinate(coord string) (float64, error) {
	coord = strings.TrimSpace(coord)
	if coord == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	direction := coord[0]
	value, err := strconv.ParseFloat(coord[1:], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid coordinate %q: %w", coord, err)
	}

	switch direction {
	case 'N', 'n', 'E', 'e':
		return value, nil
	case 'S', 's', 'W', 'w':
		return -value, nil
	default:
		return 0, fmt.Errorf("invalid direction %q in coordinate %q", string(direction), coord)
	}
}
