package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// send makes a JSON request to the server and returns the recorder.
func send(server *api.Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	return rec
}

// A plain-English date is saved in GEDCOM form, and the response carries
// what was stored so the form can show it (issue #978).
func TestDateInput_EnglishIsStoredAsGEDCOM(t *testing.T) {
	server := setupTestServer()

	rec := send(server, http.MethodPost, "/api/v1/persons", `{"given_name":"John","birth_date":"about Sept 1850"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rec.Code, rec.Body.String())
	}
	person := decodeBody(t, rec)
	birth, _ := person["birth_date"].(map[string]any)
	if birth["raw"] != "ABT SEP 1850" {
		t.Errorf("birth_date.raw = %v, want ABT SEP 1850", birth["raw"])
	}

	body := fmt.Sprintf(`{"type":"BAPL","person_id":%q,"date":"before 1900"}`, person["id"])
	rec = send(server, http.MethodPost, "/api/v1/lds-ordinances", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create ordinance: %d %s", rec.Code, rec.Body.String())
	}
	ordinance := decodeBody(t, rec)
	date, _ := ordinance["date"].(map[string]any)
	if date["raw"] != "BEF 1900" {
		t.Errorf("ordinance date.raw = %v, want BEF 1900", date["raw"])
	}
}

// A rejected date names its field and says whether it can be kept as a date
// phrase, on the person, family and LDS endpoints alike.
func TestDateInput_RejectionDetails(t *testing.T) {
	server := setupTestServer()
	personID := createPerson(t, server, "John", "Doe")
	ordinanceID, version := createIndividualOrdinance(t, server, personID)

	tests := []struct {
		name, method, path, body string
		field                    string
		unreadable               bool
	}{
		{"unreadable person date", http.MethodPost, "/api/v1/persons", `{"given_name":"Jane","birth_date":"Sept 5, 1850"}`, "birth_date", true},
		{"impossible person date", http.MethodPost, "/api/v1/persons", `{"given_name":"Jane","death_date":"31 February 1900"}`, "death_date", false},
		{"unreadable marriage date", http.MethodPost, "/api/v1/families", fmt.Sprintf(`{"partner1_id":%q,"marriage_date":"spring 1875"}`, personID), "marriage_date", true},
		{"impossible source publish date", http.MethodPost, "/api/v1/sources", `{"title":"Census","publish_date":"31 FEB 1900"}`, "publish_date", false},
		{"unreadable ordinance date", http.MethodPut, "/api/v1/lds-ordinances/" + ordinanceID, fmt.Sprintf(`{"version":%d,"date":"garbage"}`, version), "date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(server, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s, want 400", rec.Code, rec.Body.String())
			}
			body := decodeBody(t, rec)
			if msg, _ := body["message"].(string); !strings.Contains(msg, tt.field) {
				t.Errorf("message = %q, want it to name %s", msg, tt.field)
			}
			details, _ := body["details"].(map[string]any)
			if details["field"] != tt.field || details["unreadable"] != tt.unreadable {
				t.Errorf("details = %v, want field %s unreadable %v", details, tt.field, tt.unreadable)
			}
		})
	}

	// Other validation errors carry no date details.
	rec := send(server, http.MethodPost, "/api/v1/persons", `{"given_name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if details, ok := decodeBody(t, rec)["details"]; ok {
		t.Errorf("details = %v, want none", details)
	}
}

// Dual-year, BC and TO-only dates are saved with a year and export to GEDCOM
// with the date text unchanged (issue #958).
func TestDateInput_DualYearBCAndToOnlyRoundTrip(t *testing.T) {
	tests := []struct {
		date     string
		wantYear float64
		wantQual string
	}{
		{date: "1750/51", wantYear: 1751, wantQual: "exact"},
		{date: "1 JAN 1750/51", wantYear: 1751, wantQual: "exact"},
		{date: "1 JAN 1850 BC", wantYear: -1850, wantQual: "exact"},
		{date: "TO 1860", wantYear: 1860, wantQual: "to"},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			server := setupTestServer()
			rec := send(server, http.MethodPost, "/api/v1/persons", fmt.Sprintf(`{"given_name":"John","birth_date":%q}`, tt.date))
			if rec.Code != http.StatusCreated {
				t.Fatalf("create person: %d %s", rec.Code, rec.Body.String())
			}
			birth, _ := decodeBody(t, rec)["birth_date"].(map[string]any)
			if birth["raw"] != tt.date || birth["year"] != tt.wantYear || birth["qualifier"] != tt.wantQual {
				t.Errorf("birth_date = %v, want raw %q, year %v, qualifier %q", birth, tt.date, tt.wantYear, tt.wantQual)
			}

			rec = send(server, http.MethodGet, "/api/v1/gedcom/export", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
			}
			if want := "2 DATE " + tt.date + "\n"; !strings.Contains(rec.Body.String(), want) {
				t.Errorf("export does not contain %q:\n%s", want, rec.Body.String())
			}
		})
	}
}
