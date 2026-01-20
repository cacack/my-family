package domain_test

import (
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestNewRepository(t *testing.T) {
	name := "Family History Library"
	repo := domain.NewRepository(name)

	if repo.Name != name {
		t.Errorf("Name = %q, want %q", repo.Name, name)
	}
	if repo.ID.String() == "" {
		t.Error("ID should not be empty")
	}
	if repo.Version != 1 {
		t.Errorf("Version = %d, want 1", repo.Version)
	}
}

func TestRepository_Validate(t *testing.T) {
	tests := []struct {
		name    string
		repo    *domain.Repository
		wantErr bool
	}{
		{
			name:    "valid repository",
			repo:    domain.NewRepository("Family History Library"),
			wantErr: false,
		},
		{
			name:    "empty name",
			repo:    domain.NewRepository(""),
			wantErr: true,
		},
		{
			name:    "name too long",
			repo:    domain.NewRepository(strings.Repeat("a", 201)),
			wantErr: true,
		},
		{
			name:    "name at limit",
			repo:    domain.NewRepository(strings.Repeat("a", 200)),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.repo.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRepository_FullAddress(t *testing.T) {
	tests := []struct {
		name string
		repo *domain.Repository
		want string
	}{
		{
			name: "full address",
			repo: &domain.Repository{
				StreetAddress: "35 N West Temple St",
				City:          "Salt Lake City",
				State:         "UT",
				PostalCode:    "84150",
				Country:       "USA",
			},
			want: "35 N West Temple St, Salt Lake City, UT, 84150, USA",
		},
		{
			name: "city and state only",
			repo: &domain.Repository{
				City:  "Springfield",
				State: "IL",
			},
			want: "Springfield, IL",
		},
		{
			name: "empty address",
			repo: &domain.Repository{},
			want: "",
		},
		{
			name: "just country",
			repo: &domain.Repository{
				Country: "USA",
			},
			want: "USA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.FullAddress()
			if got != tt.want {
				t.Errorf("FullAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepository_GetAddress(t *testing.T) {
	tests := []struct {
		name     string
		repo     *domain.Repository
		wantNil  bool
		wantCity string
	}{
		{
			name:    "empty repository returns nil",
			repo:    &domain.Repository{},
			wantNil: true,
		},
		{
			name: "repository with address fields",
			repo: &domain.Repository{
				StreetAddress: "35 N West Temple St",
				City:          "Salt Lake City",
				State:         "UT",
			},
			wantNil:  false,
			wantCity: "Salt Lake City",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := tt.repo.GetAddress()
			if tt.wantNil {
				if addr != nil {
					t.Error("GetAddress() = non-nil, want nil")
				}
			} else {
				if addr == nil {
					t.Fatal("GetAddress() = nil, want non-nil")
				}
				if addr.City != tt.wantCity {
					t.Errorf("GetAddress().City = %q, want %q", addr.City, tt.wantCity)
				}
			}
		})
	}
}

func TestRepository_SetAddress(t *testing.T) {
	repo := domain.NewRepository("Test Repo")
	addr := &domain.Address{
		Line1:      "123 Main St",
		Line2:      "Suite 100",
		City:       "Boston",
		State:      "MA",
		PostalCode: "02101",
		Country:    "USA",
		Phone:      "555-1234",
		Email:      "test@example.com",
		Website:    "https://example.com",
	}

	repo.SetAddress(addr)

	// StreetAddress should be the combined lines
	if repo.StreetAddress != "123 Main St, Suite 100" {
		t.Errorf("StreetAddress = %q, want %q", repo.StreetAddress, "123 Main St, Suite 100")
	}
	if repo.City != "Boston" {
		t.Errorf("City = %q, want %q", repo.City, "Boston")
	}
	if repo.State != "MA" {
		t.Errorf("State = %q, want %q", repo.State, "MA")
	}
	if repo.Phone != "555-1234" {
		t.Errorf("Phone = %q, want %q", repo.Phone, "555-1234")
	}

	// Test nil address does nothing
	repo2 := domain.NewRepository("Test Repo 2")
	repo2.SetAddress(nil)
	if repo2.City != "" {
		t.Error("SetAddress(nil) should not modify fields")
	}
}

func TestRepositoryValidationError_Error(t *testing.T) {
	err := domain.RepositoryValidationError{
		Field:   "name",
		Message: "cannot be empty",
	}
	want := "name: cannot be empty"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
