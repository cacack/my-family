package repository

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseMapCoordinates(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	id := uuid.New()
	tests := []struct {
		name     string
		lat, lon string
		ok       bool
		wantLat  float64
		wantLon  float64
	}{
		{"valid", "N42.3601", "W71.0589", true, 42.3601, -71.0589},
		{"latitude carries E", "E42.3601", "W71.0589", false, 0, 0},
		{"longitude carries N", "N42.3601", "N71.0589", false, 0, 0},
		{"latitude out of range", "N95", "E10", false, 0, 0},
		{"longitude out of range", "S10", "W181", false, 0, 0},
		{"not a number", "Nnan", "E10", false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			lat, lon, ok := ParseMapCoordinates(id, "birth", tt.lat, tt.lon)
			if ok != tt.ok || lat != tt.wantLat || lon != tt.wantLon {
				t.Fatalf("ParseMapCoordinates(%q, %q) = %v, %v, %v; want %v, %v, %v",
					tt.lat, tt.lon, lat, lon, ok, tt.wantLat, tt.wantLon, tt.ok)
			}
			logged := buf.String()
			if tt.ok {
				if logged != "" {
					t.Errorf("valid coordinates logged %q", logged)
				}
				return
			}
			for _, want := range []string{id.String(), "event_type=birth", "latitude=" + tt.lat, "longitude=" + tt.lon} {
				if !strings.Contains(logged, want) {
					t.Errorf("log %q does not name %q", logged, want)
				}
			}
		})
	}
}
