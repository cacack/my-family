package gedcom

import (
	"github.com/cacack/gedcom-go/v3/gedcom"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// PlaceData is what one PLAC line of a person's or family's event or
// attribute states about its place beyond the text (ADR-009 §8): the form of
// the text's parts, the PLAC's own FORM or else the file's HEAD.PLAC.FORM,
// and its EXIDs. Record and Tag name where it was read, for warnings.
type PlaceData struct {
	Record      string
	Tag         string
	Text        string
	Form        string
	ExternalIDs []domain.ExternalIdentifier
}

// placeFacts returns the PlaceData of every PLAC under a person's or family's
// event or attribute that names a place and states a form or an EXID, in file
// order: records in the order the file lists them; within a record, its
// events, then its attributes, each in file order. gedcom-go keeps no
// position across a record's events and attributes, so that order is the
// closest the decoded document gives to the file's.
func placeFacts(doc *gedcom.Document) []PlaceData {
	headerForm := ""
	if doc.Header != nil {
		headerForm = doc.Header.PlaceForm
	}
	var places []PlaceData
	add := func(record, tag string, detail *gedcom.PlaceDetail) {
		if detail == nil || repository.PlaceKey(detail.Name) == "" {
			return
		}
		form := detail.Form
		if form == "" {
			form = headerForm
		}
		ids := toDomainExternalIDs(detail.ExternalIDs)
		if form == "" && len(ids) == 0 {
			return
		}
		places = append(places, PlaceData{Record: record, Tag: tag, Text: detail.Name, Form: form, ExternalIDs: ids})
	}
	forEachPlaceDetail(doc, add)
	return places
}

// forEachPlaceDetail calls visit with the PLAC of every person's event and
// attribute and every family's event, in the order placeFacts documents, and
// the record and tag it was read under. Import and export both walk the
// document with it, so export writes place facts under exactly the PLACs
// import reads them from.
func forEachPlaceDetail(doc *gedcom.Document, visit func(record, tag string, detail *gedcom.PlaceDetail)) {
	for _, rec := range doc.Records {
		if indi, ok := rec.GetIndividual(); ok {
			for _, e := range indi.Events {
				visit("Individual "+indi.XRef, string(e.Type), e.PlaceDetail)
			}
			for _, a := range indi.Attributes {
				visit("Individual "+indi.XRef, string(a.Type), a.PlaceDetail)
			}
		}
		if fam, ok := rec.GetFamily(); ok {
			for _, e := range fam.Events {
				visit("Family "+fam.XRef, string(e.Type), e.PlaceDetail)
			}
		}
	}
}
