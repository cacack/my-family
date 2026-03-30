package citation

import (
	"testing"
)

func TestFormatFullCensus(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"year":        "1850",
		"state":       "Virginia",
		"county":      "Augusta",
		"town":        "Staunton",
		"sheet":       "12",
		"line":        "5",
		"person_name": "John Smith",
		"nara_pub":    "M432",
		"nara_roll":   "943",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `1850 U.S. Census, Virginia, Augusta County, Staunton, sheet 12, line 5; John Smith household; NARA microfilm publication M432, roll 943.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

func TestFormatShortCensus(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"year":        "1850",
		"state":       "Virginia",
		"county":      "Augusta",
		"sheet":       "12",
		"person_name": "John Smith",
	}

	short, err := FormatShort(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `1850 U.S. Census, Augusta Co., Virginia, sheet 12, John Smith.`
	if short != expected {
		t.Errorf("unexpected short citation:\ngot:  %s\nwant: %s", short, expected)
	}
}

func TestFormatFullVitalBirth(t *testing.T) {
	tmpl := GetTemplate("vital.birth")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"jurisdiction":     "Ohio",
		"record_type":      "Certificate of Birth",
		"registrant":       "Jane Doe",
		"date":             "15 March 1920",
		"certificate_num":  "12345",
		"registrar_office": "Ohio Department of Health",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `Ohio, Certificate of Birth, Jane Doe, 15 March 1920; certificate no. 12345; Ohio Department of Health.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

func TestFormatFullNewspaperByline(t *testing.T) {
	tmpl := GetTemplate("newspaper.article_byline")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"author":        "John Reporter",
		"article_title": "Local Man Wins Award",
		"newspaper":     "Daily Gazette",
		"location":      "Springfield, IL",
		"date":          "5 June 1955",
		"page":          "3",
		"column":        "2",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `John Reporter, "Local Man Wins Award," Daily Gazette (Springfield, IL), 5 June 1955, p. 3, col. 2.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

func TestFormatWithMissingOptionalFields(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	// Minimal fields — only required ones plus some basics.
	fields := map[string]string{
		"year":        "1900",
		"state":       "Ohio",
		"county":      "Franklin",
		"person_name": "Mary Jones",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	// Should not contain empty sections or stray punctuation.
	expected := `1900 U.S. Census, Ohio, Franklin County; Mary Jones household.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

func TestFormatBook(t *testing.T) {
	tmpl := GetTemplate("published.book")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"author":        "Elizabeth Shown Mills",
		"title":         "Evidence Explained",
		"publisher_loc": "Baltimore",
		"publisher":     "Genealogical Publishing Co.",
		"year":          "2017",
		"page":          "p. 45",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `Elizabeth Shown Mills, Evidence Explained (Baltimore: Genealogical Publishing Co., 2017), p. 45.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

func TestFormatInterview(t *testing.T) {
	tmpl := GetTemplate("personal.interview")
	if tmpl == nil {
		t.Fatal("template not found")
	}

	fields := map[string]string{
		"interviewee":  "Grandma Smith",
		"relationship": "grandmother of researcher",
		"date":         "25 December 2023",
		"interviewer":  "Chris Smith",
		"location":     "Springfield, Ohio",
		"format":       "in person",
	}

	full, err := FormatFull(tmpl, fields)
	if err != nil {
		t.Fatal(err)
	}

	expected := `Grandma Smith (grandmother of researcher), interview by Chris Smith, 25 December 2023; Springfield, Ohio; in person.`
	if full != expected {
		t.Errorf("unexpected full citation:\ngot:  %s\nwant: %s", full, expected)
	}
}

// TestAllTemplatesCompile verifies every template's format strings parse without error.
func TestAllTemplatesCompile(t *testing.T) {
	for _, tmpl := range allTemplates {
		t.Run(tmpl.ID+"/full", func(t *testing.T) {
			_, err := FormatFull(&tmpl, map[string]string{})
			if err != nil {
				t.Errorf("full format failed to execute: %v", err)
			}
		})
		t.Run(tmpl.ID+"/short", func(t *testing.T) {
			_, err := FormatShort(&tmpl, map[string]string{})
			if err != nil {
				t.Errorf("short format failed to execute: %v", err)
			}
		})
	}
}
