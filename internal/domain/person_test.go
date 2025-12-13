package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewPerson(t *testing.T) {
	p := NewPerson("John", "Doe")

	if p.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if p.GivenName != "John" {
		t.Errorf("GivenName = %v, want John", p.GivenName)
	}
	if p.Surname != "Doe" {
		t.Errorf("Surname = %v, want Doe", p.Surname)
	}
	if p.Version != 1 {
		t.Errorf("Version = %v, want 1", p.Version)
	}
}

func TestPerson_Validate(t *testing.T) {
	tests := []struct {
		name    string
		person  *Person
		wantErr bool
	}{
		{
			name:    "valid person",
			person:  NewPerson("John", "Doe"),
			wantErr: false,
		},
		{
			name:    "empty given name",
			person:  &Person{ID: uuid.New(), GivenName: "", Surname: "Doe"},
			wantErr: true,
		},
		{
			name:    "empty surname (valid for historical records)",
			person:  &Person{ID: uuid.New(), GivenName: "John", Surname: ""},
			wantErr: false,
		},
		{
			name: "given name too long",
			person: &Person{
				ID:        uuid.New(),
				GivenName: string(make([]byte, 101)),
				Surname:   "Doe",
			},
			wantErr: true,
		},
		{
			name: "surname too long",
			person: &Person{
				ID:        uuid.New(),
				GivenName: "John",
				Surname:   string(make([]byte, 101)),
			},
			wantErr: true,
		},
		{
			name: "invalid gender",
			person: &Person{
				ID:        uuid.New(),
				GivenName: "John",
				Surname:   "Doe",
				Gender:    "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid male gender",
			person: &Person{
				ID:        uuid.New(),
				GivenName: "John",
				Surname:   "Doe",
				Gender:    GenderMale,
			},
			wantErr: false,
		},
		{
			name: "death before birth",
			person: func() *Person {
				p := NewPerson("John", "Doe")
				birth := ParseGenDate("1 JAN 1900")
				death := ParseGenDate("1 JAN 1850")
				p.BirthDate = &birth
				p.DeathDate = &death
				return p
			}(),
			wantErr: true,
		},
		{
			name: "death after birth",
			person: func() *Person {
				p := NewPerson("John", "Doe")
				birth := ParseGenDate("1 JAN 1850")
				death := ParseGenDate("1 JAN 1900")
				p.BirthDate = &birth
				p.DeathDate = &death
				return p
			}(),
			wantErr: false,
		},
		{
			name: "invalid birth date",
			person: func() *Person {
				p := NewPerson("John", "Doe")
				birth := GenDate{Year: intPtr(1850), Month: intPtr(13)}
				p.BirthDate = &birth
				return p
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.person.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPerson_FullName(t *testing.T) {
	p := NewPerson("John", "Doe")
	if got := p.FullName(); got != "John Doe" {
		t.Errorf("FullName() = %v, want John Doe", got)
	}
}

func TestPerson_SetBirthDate(t *testing.T) {
	p := NewPerson("John", "Doe")

	p.SetBirthDate("1 JAN 1850")
	if p.BirthDate == nil {
		t.Fatal("BirthDate should not be nil")
	}
	if *p.BirthDate.Year != 1850 {
		t.Errorf("BirthDate.Year = %v, want 1850", *p.BirthDate.Year)
	}

	p.SetBirthDate("")
	if p.BirthDate != nil {
		t.Error("BirthDate should be nil after setting empty string")
	}
}

func TestPerson_SetDeathDate(t *testing.T) {
	p := NewPerson("John", "Doe")

	p.SetDeathDate("1 JAN 1900")
	if p.DeathDate == nil {
		t.Fatal("DeathDate should not be nil")
	}
	if *p.DeathDate.Year != 1900 {
		t.Errorf("DeathDate.Year = %v, want 1900", *p.DeathDate.Year)
	}

	p.SetDeathDate("")
	if p.DeathDate != nil {
		t.Error("DeathDate should be nil after setting empty string")
	}
}
