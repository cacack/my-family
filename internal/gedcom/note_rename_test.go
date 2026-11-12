package gedcom_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// noteRenameGedcom points a NOTE at @N1@ from every place the importer keeps
// note pointers. With @N1@ already in the tree, the file's @N1@ is renamed and
// every one of those pointers must follow it (#1228, #1375).
const noteRenameGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
2 NOTE @N1@
1 SEX M
1 BIRT
2 DATE 1850
2 NOTE @N1@
1 DEAT
2 DATE 1920
2 NOTE @N1@
1 BAPM
2 DATE 1851
2 NOTE @N1@
1 OCCU Blacksmith
2 NOTE @N1@
1 ASSO @I2@
2 RELA godfather
2 NOTE @N1@
1 NOTE @N1@
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1875
2 NOTE @N1@
1 DIV
2 DATE 1890
2 NOTE @N1@
1 NOTE @N1@
0 @R1@ REPO
1 NAME Archive
1 NOTE @N1@
0 @S1@ SOUR
1 TITL Register
1 NOTE @N1@
0 @N1@ NOTE Imported note
0 TRLR
`

func TestImport_AttributeNoteFollowsRenamedNoteXref(t *testing.T) {
	_, _, _, _, _, _, _, attributes, notes, _, _, _, _, err := gedcom.NewImporter().ImportWithOptions(context.Background(),
		strings.NewReader(noteRenameGedcom), gedcom.ImportOptions{ExistingXrefs: []string{"@N1@"}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(notes) != 1 || notes[0].GedcomXref == "@N1@" {
		t.Fatalf("notes = %+v, want the one note renamed", notes)
	}
	if len(attributes) != 1 || attributes[0].Notes != notes[0].GedcomXref {
		t.Errorf("attributes = %+v, want Notes %q", attributes, notes[0].GedcomXref)
	}
}

// TestRenameCollidingNotes_EveryNoteField guards renameCollidingNotes: every
// string field named *Notes on any imported type must be filled by the
// fixture and point at the renamed note. A new note-bearing type or field
// fails here until the fixture and renameCollidingNotes cover it.
func TestRenameCollidingNotes_EveryNoteField(t *testing.T) {
	result, persons, families, sources, citations, repositories, events, attributes, notes, submitters, associations, ldsOrdinances, media, err := gedcom.NewImporter().ImportWithOptions(context.Background(),
		strings.NewReader(noteRenameGedcom), gedcom.ImportOptions{ExistingXrefs: []string{"@N1@"}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(notes) != 1 || notes[0].GedcomXref == "@N1@" {
		t.Fatalf("notes = %+v, want the one note renamed", notes)
	}
	renamed := notes[0].GedcomXref

	// filled records, per Type.Field, whether any value was non-empty.
	filled := map[string]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Type().Field(i)
				if !f.IsExported() {
					continue
				}
				fv := v.Field(i)
				if fv.Kind() == reflect.String && strings.HasSuffix(f.Name, "Notes") {
					key := v.Type().Name() + "." + f.Name
					filled[key] = filled[key] || fv.String() != ""
					if fv.String() != "" && fv.String() != renamed {
						t.Errorf("%s = %q, want %q", key, fv.String(), renamed)
					}
					continue
				}
				walk(fv)
			}
		}
	}
	// Declare every key up front so a type whose slice is empty still counts.
	var declare func(typ reflect.Type)
	declare = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ.PkgPath() != reflect.TypeOf(notes[0]).PkgPath() {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.String && strings.HasSuffix(f.Name, "Notes") {
				key := typ.Name() + "." + f.Name
				if _, ok := filled[key]; !ok {
					filled[key] = false
				}
				continue
			}
			declare(f.Type)
		}
	}
	for _, out := range []any{result, persons, families, sources, citations, repositories, events, attributes, notes, submitters, associations, ldsOrdinances, media} {
		declare(reflect.TypeOf(out))
		walk(reflect.ValueOf(out))
	}

	if len(filled) == 0 {
		t.Fatal("found no *Notes fields; the guard is not inspecting the import types")
	}
	for key, ok := range filled {
		if !ok {
			t.Errorf("%s is never filled by noteRenameGedcom: add a NOTE @N1@ for it to the fixture and rewrite it in renameCollidingNotes", key)
		}
	}
}
