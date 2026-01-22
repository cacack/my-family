package domain_test

import (
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestAddress_String(t *testing.T) {
	tests := []struct {
		name    string
		address *domain.Address
		want    string
	}{
		{
			name:    "nil address",
			address: nil,
			want:    "",
		},
		{
			name:    "empty address",
			address: &domain.Address{},
			want:    "",
		},
		{
			name: "full US address",
			address: &domain.Address{
				Line1:      "35 N West Temple St",
				City:       "Salt Lake City",
				State:      "UT",
				PostalCode: "84150",
				Country:    "USA",
			},
			want: "35 N West Temple St, Salt Lake City, UT 84150, USA",
		},
		{
			name: "address with multiple lines",
			address: &domain.Address{
				Line1: "Family History Library",
				Line2: "35 N West Temple St",
				Line3: "Suite 100",
				City:  "Salt Lake City",
				State: "UT",
			},
			want: "Family History Library, 35 N West Temple St, Suite 100, Salt Lake City, UT",
		},
		{
			name: "international address",
			address: &domain.Address{
				Line1:      "The National Archives",
				Line2:      "Kew",
				City:       "Richmond",
				PostalCode: "TW9 4DU",
				Country:    "United Kingdom",
			},
			want: "The National Archives, Kew, Richmond TW9 4DU, United Kingdom",
		},
		{
			name: "city only",
			address: &domain.Address{
				City: "London",
			},
			want: "London",
		},
		{
			name: "country only",
			address: &domain.Address{
				Country: "Germany",
			},
			want: "Germany",
		},
		{
			name: "city and state only",
			address: &domain.Address{
				City:  "Springfield",
				State: "IL",
			},
			want: "Springfield, IL",
		},
		{
			name: "state and postal code only",
			address: &domain.Address{
				State:      "CA",
				PostalCode: "90210",
			},
			want: "CA 90210",
		},
		{
			name: "postal code only",
			address: &domain.Address{
				PostalCode: "12345",
			},
			want: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.address.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddress_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		address *domain.Address
		want    bool
	}{
		{
			name:    "nil address",
			address: nil,
			want:    true,
		},
		{
			name:    "empty address",
			address: &domain.Address{},
			want:    true,
		},
		{
			name: "only Line1 set",
			address: &domain.Address{
				Line1: "123 Main St",
			},
			want: false,
		},
		{
			name: "only Line2 set",
			address: &domain.Address{
				Line2: "Apt 4B",
			},
			want: false,
		},
		{
			name: "only Line3 set",
			address: &domain.Address{
				Line3: "Building C",
			},
			want: false,
		},
		{
			name: "only City set",
			address: &domain.Address{
				City: "Boston",
			},
			want: false,
		},
		{
			name: "only State set",
			address: &domain.Address{
				State: "MA",
			},
			want: false,
		},
		{
			name: "only PostalCode set",
			address: &domain.Address{
				PostalCode: "02101",
			},
			want: false,
		},
		{
			name: "only Country set",
			address: &domain.Address{
				Country: "USA",
			},
			want: false,
		},
		{
			name: "only Phone set",
			address: &domain.Address{
				Phone: "555-1234",
			},
			want: false,
		},
		{
			name: "only Email set",
			address: &domain.Address{
				Email: "test@example.com",
			},
			want: false,
		},
		{
			name: "only Fax set",
			address: &domain.Address{
				Fax: "555-5678",
			},
			want: false,
		},
		{
			name: "only Website set",
			address: &domain.Address{
				Website: "https://example.com",
			},
			want: false,
		},
		{
			name: "full address",
			address: &domain.Address{
				Line1:      "123 Main St",
				City:       "Boston",
				State:      "MA",
				PostalCode: "02101",
				Country:    "USA",
				Phone:      "555-1234",
				Email:      "test@example.com",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.address.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress_StreetAddress(t *testing.T) {
	tests := []struct {
		name    string
		address *domain.Address
		want    string
	}{
		{
			name:    "nil address",
			address: nil,
			want:    "",
		},
		{
			name:    "empty address",
			address: &domain.Address{},
			want:    "",
		},
		{
			name: "single line",
			address: &domain.Address{
				Line1: "123 Main St",
			},
			want: "123 Main St",
		},
		{
			name: "two lines",
			address: &domain.Address{
				Line1: "123 Main St",
				Line2: "Apt 4B",
			},
			want: "123 Main St, Apt 4B",
		},
		{
			name: "three lines",
			address: &domain.Address{
				Line1: "Family History Library",
				Line2: "35 N West Temple St",
				Line3: "Suite 100",
			},
			want: "Family History Library, 35 N West Temple St, Suite 100",
		},
		{
			name: "lines with city ignored",
			address: &domain.Address{
				Line1: "123 Main St",
				City:  "Boston",
			},
			want: "123 Main St",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.address.StreetAddress()
			if got != tt.want {
				t.Errorf("StreetAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
