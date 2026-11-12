package command

import (
	"fmt"
	"regexp"
)

// maxGedcomXrefLength and gedcomXrefPattern are the gedcom_xref shape the API
// accepts, kept in step with maxLength and pattern on the gedcom_xref
// properties of the create/update schemas in openapi.yaml: empty (no xref), or
// one @-delimited pointer with no whitespace or interior @, so the value can
// be written as a GEDCOM xref verbatim.
const maxGedcomXrefLength = 64

var gedcomXrefPattern = regexp.MustCompile(`^(@[^@\s]+@)?$`)

// validateGedcomXref checks a newly entered gedcom_xref. Only new input is
// checked: GEDCOM import keeps a file's xrefs verbatim (5.5.1 allows `@N 1@`),
// so a stored xref outside this shape never blocks an unrelated edit.
func validateGedcomXref(xref string) error {
	if len(xref) > maxGedcomXrefLength {
		return fmt.Errorf("%w: gedcom_xref: cannot exceed %d characters", ErrInvalidInput, maxGedcomXrefLength)
	}
	if !gedcomXrefPattern.MatchString(xref) {
		return fmt.Errorf("%w: gedcom_xref: must be an @-delimited pointer such as @R1@ with no whitespace or interior @", ErrInvalidInput)
	}
	return nil
}
