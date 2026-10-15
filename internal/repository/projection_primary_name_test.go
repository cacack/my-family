package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestProjector_PrimaryNameSync proves a person's given name and surname and
// its primary name stay in agreement whichever event changes them (#1093):
// PersonUpdated and PersonMerged copy onto the primary name, and a primary
// NameAdded/NameUpdated copies onto the person. Alternate names never move the
// person, and a person with no names is left as it is.
func TestProjector_PrimaryNameSync(t *testing.T) {
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)

			project := func(t *testing.T, event domain.Event, version int64) {
				t.Helper()
				if err := projector.Project(ctx, event, version, domain.MainBranchID); err != nil {
					t.Fatalf("Project %s: %v", event.EventType(), err)
				}
			}
			getPerson := func(t *testing.T, id uuid.UUID) *repository.PersonReadModel {
				t.Helper()
				rm, err := readStore.GetPerson(ctx, domain.MainBranchID, id)
				if err != nil || rm == nil {
					t.Fatalf("GetPerson = %v, %v", rm, err)
				}
				return rm
			}
			getName := func(t *testing.T, id uuid.UUID) *repository.PersonNameReadModel {
				t.Helper()
				rm, err := readStore.GetPersonName(ctx, domain.MainBranchID, id)
				if err != nil || rm == nil {
					t.Fatalf("GetPersonName = %v, %v", rm, err)
				}
				return rm
			}
			// seed creates a person with a primary name (carrying a prefix, which
			// a sync must keep) and an alternate.
			seed := func(t *testing.T) (person *domain.Person, primary, alt *domain.PersonName) {
				t.Helper()
				person = domain.NewPerson("Avery", "Original")
				project(t, domain.NewPersonCreated(person), 1)
				primary = domain.NewPersonName(person.ID, "Avery", "Original")
				primary.NamePrefix = "Dr."
				primary.IsPrimary = true
				project(t, domain.NewNameAdded(primary), 2)
				alt = domain.NewPersonName(person.ID, "Ava", "Alias")
				alt.NameType = domain.NameTypeAKA
				project(t, domain.NewNameAdded(alt), 3)
				return person, primary, alt
			}
			assertPerson := func(t *testing.T, id uuid.UUID, given, surname string, version int64) {
				t.Helper()
				p := getPerson(t, id)
				if p.GivenName != given || p.Surname != surname || p.FullName != domain.JoinFullName(given, surname) {
					t.Errorf("person = %q / %q / %q, want %s %s", p.GivenName, p.Surname, p.FullName, given, surname)
				}
				if p.Version != version {
					t.Errorf("person version = %d, want %d", p.Version, version)
				}
			}
			// assertName checks a name's components. The full name is checked
			// only to contain "given surname": the SQL backends generate it
			// from those two columns alone, the memory store adds the prefix.
			assertName := func(t *testing.T, id uuid.UUID, prefix, given, surname string) {
				t.Helper()
				n := getName(t, id)
				if n.NamePrefix != prefix || n.GivenName != given || n.Surname != surname ||
					!strings.Contains(n.FullName, domain.JoinFullName(given, surname)) {
					t.Errorf("name = %q / %q / %q (full %q), want %q / %q / %q",
						n.NamePrefix, n.GivenName, n.Surname, n.FullName, prefix, given, surname)
				}
			}

			t.Run("person update renames the primary name", func(t *testing.T) {
				person, primary, alt := seed(t)
				project(t, domain.NewPersonUpdated(person.ID, map[string]any{"given_name": "Blake", "surname": "Renamed"}), 4)
				assertPerson(t, person.ID, "Blake", "Renamed", 4)
				assertName(t, primary.ID, "Dr.", "Blake", "Renamed")
				assertName(t, alt.ID, "", "Ava", "Alias")
			})

			t.Run("person update of another field leaves names alone", func(t *testing.T) {
				person, primary, _ := seed(t)
				before := getName(t, primary.ID)
				project(t, domain.NewPersonUpdated(person.ID, map[string]any{"notes": "a note"}), 4)
				after := getName(t, primary.ID)
				if *before != *after {
					t.Errorf("primary name changed on a notes edit:\n before %+v\n after  %+v", before, after)
				}
			})

			t.Run("person with no names", func(t *testing.T) {
				person := domain.NewPerson("Casey", "Solo")
				project(t, domain.NewPersonCreated(person), 1)
				project(t, domain.NewPersonUpdated(person.ID, map[string]any{"surname": "Renamed"}), 2)
				assertPerson(t, person.ID, "Casey", "Renamed", 2)
				names, err := readStore.GetPersonNames(ctx, domain.MainBranchID, person.ID)
				if err != nil || len(names) != 0 {
					t.Errorf("names = %v, %v; want none", names, err)
				}
			})

			t.Run("primary name update renames the person", func(t *testing.T) {
				person, primary, _ := seed(t)
				primary.GivenName, primary.Surname = "Drew", "Final"
				project(t, domain.NewNameUpdated(primary), 4)
				assertPerson(t, person.ID, "Drew", "Final", 4)
			})

			t.Run("alternate name update leaves the person alone", func(t *testing.T) {
				person, _, alt := seed(t)
				alt.GivenName = "Avie"
				project(t, domain.NewNameUpdated(alt), 4)
				assertPerson(t, person.ID, "Avery", "Original", 4)
			})

			t.Run("promoting an alternate renames the person", func(t *testing.T) {
				person, primary, alt := seed(t)
				primary.IsPrimary = false
				project(t, domain.NewNameUpdated(primary), 4)
				alt.IsPrimary = true
				project(t, domain.NewNameUpdated(alt), 5)
				assertPerson(t, person.ID, "Ava", "Alias", 5)
			})

			t.Run("merge resolution renames the survivor's primary name", func(t *testing.T) {
				survivor, primary, _ := seed(t)
				merged := domain.NewPerson("Emery", "Other")
				project(t, domain.NewPersonCreated(merged), 1)
				project(t, domain.NewPersonMerged(survivor.ID, merged.ID, map[string]any{},
					map[string]any{"given_name": "Emery"}, nil, nil, nil, nil, nil), 4)
				assertPerson(t, survivor.ID, "Emery", "Original", 4)
				assertName(t, primary.ID, "Dr.", "Emery", "Original")
			})
		})
	}
}
