package repository

import (
	"context"
	"testing"
	"time"
)

func TestTreeSettings_Validate(t *testing.T) {
	tests := []struct {
		years   int
		wantErr bool
	}{
		{MinLivingThresholdYears - 1, true},
		{MinLivingThresholdYears, false},
		{110, false},
		{MaxLivingThresholdYears, false},
		{MaxLivingThresholdYears + 1, true},
	}
	for _, tt := range tests {
		err := TreeSettings{LivingThresholdYears: tt.years}.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("Validate(%d years) = %v, wantErr %v", tt.years, err, tt.wantErr)
		}
	}
	if err := DefaultTreeSettings().Validate(); err != nil {
		t.Errorf("the defaults are invalid: %v", err)
	}
}

func TestTreeSettingsFrom(t *testing.T) {
	if got := TreeSettingsFrom(context.Background()); got != DefaultTreeSettings() {
		t.Errorf("TreeSettingsFrom(empty) = %+v, want the defaults", got)
	}
	want := TreeSettings{LivingThresholdYears: 90, ViewersSeeLiving: false}
	if got := TreeSettingsFrom(ContextWithTreeSettings(context.Background(), want)); got != want {
		t.Errorf("TreeSettingsFrom = %+v, want %+v", got, want)
	}
}

func TestTreeSettings_LivingCutoff(t *testing.T) {
	now := time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC)
	got := TreeSettings{LivingThresholdYears: 90}.LivingCutoff(now)
	if want := time.Date(1936, 11, 19, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("LivingCutoff = %v, want %v", got, want)
	}
}
