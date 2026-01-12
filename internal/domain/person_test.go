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

func TestNewPersonName(t *testing.T) {
	personID := uuid.New()
	pn := NewPersonName(personID, "Mary", "Smith")

	if pn.ID == uuid.Nil {
		t.Error("Expected non-nil UUID for PersonName.ID")
	}
	if pn.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", pn.PersonID, personID)
	}
	if pn.GivenName != "Mary" {
		t.Errorf("GivenName = %v, want Mary", pn.GivenName)
	}
	if pn.Surname != "Smith" {
		t.Errorf("Surname = %v, want Smith", pn.Surname)
	}
}

func TestPersonName_Validate(t *testing.T) {
	personID := uuid.New()

	tests := []struct {
		name    string
		pn      *PersonName
		wantErr bool
	}{
		{
			name:    "valid person name",
			pn:      NewPersonName(personID, "Mary", "Smith"),
			wantErr: false,
		},
		{
			name:    "empty given name",
			pn:      &PersonName{ID: uuid.New(), PersonID: personID, GivenName: "", Surname: "Smith"},
			wantErr: true,
		},
		{
			name:    "empty surname (valid for historical records)",
			pn:      &PersonName{ID: uuid.New(), PersonID: personID, GivenName: "Mary", Surname: ""},
			wantErr: false,
		},
		{
			name: "given name too long",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: string(make([]byte, 101)),
				Surname:   "Smith",
			},
			wantErr: true,
		},
		{
			name: "surname too long",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Mary",
				Surname:   string(make([]byte, 101)),
			},
			wantErr: true,
		},
		{
			name: "invalid name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Mary",
				Surname:   "Smith",
				NameType:  "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid birth name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Mary",
				Surname:   "Smith",
				NameType:  NameTypeBirth,
			},
			wantErr: false,
		},
		{
			name: "valid married name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Mary",
				Surname:   "Jones",
				NameType:  NameTypeMarried,
			},
			wantErr: false,
		},
		{
			name: "valid immigrant name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Maria",
				Surname:   "Kowalski",
				NameType:  NameTypeImmigrant,
			},
			wantErr: false,
		},
		{
			name: "valid religious name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Sister Mary",
				Surname:   "",
				NameType:  NameTypeReligious,
			},
			wantErr: false,
		},
		{
			name: "valid professional name type",
			pn: &PersonName{
				ID:        uuid.New(),
				PersonID:  personID,
				GivenName: "Mark",
				Surname:   "Twain",
				NameType:  NameTypeProfessional,
			},
			wantErr: false,
		},
		{
			name: "full name with all optional fields",
			pn: &PersonName{
				ID:            uuid.New(),
				PersonID:      personID,
				GivenName:     "Johann",
				Surname:       "Bach",
				NamePrefix:    "Dr.",
				NameSuffix:    "PhD",
				SurnamePrefix: "von",
				Nickname:      "Johnny",
				NameType:      NameTypeBirth,
				IsPrimary:     true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pn.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPersonName_FullName(t *testing.T) {
	personID := uuid.New()

	tests := []struct {
		name string
		pn   *PersonName
		want string
	}{
		{
			name: "given and surname",
			pn:   NewPersonName(personID, "Mary", "Smith"),
			want: "Mary Smith",
		},
		{
			name: "given name only",
			pn:   NewPersonName(personID, "Mary", ""),
			want: "Mary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pn.FullName(); got != tt.want {
				t.Errorf("FullName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPersonNameValidationError_Error(t *testing.T) {
	err := PersonNameValidationError{Field: "given_name", Message: "cannot be empty"}
	expected := "given_name: cannot be empty"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
