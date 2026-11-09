package repository_test

import (
	"testing"
	"time"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestSortTime covers when a date has a sort value: 1 JAN 1 AD is a real date
// even though it is time.Time{} (#1078).
func TestSortTime(t *testing.T) {
	year := 1850
	yearOne := domain.ParseGenDate("1 JAN 1")
	tests := []struct {
		name string
		gd   *domain.GenDate
		want *time.Time
	}{
		{"nil", nil, nil},
		{"no year", &domain.GenDate{Raw: "unknown"}, nil},
		{"unconvertible calendar", &domain.GenDate{Year: &year, Calendar: "BOGUS"}, nil},
		{"1 JAN 1 AD", &yearOne, &time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repository.SortTime(tt.gd)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("SortTime = %v, want nil", *got)
			case tt.want != nil && (got == nil || !got.Equal(*tt.want)):
				t.Errorf("SortTime = %v, want %v", got, *tt.want)
			}
		})
	}
}
