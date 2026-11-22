package api_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A person or a name may have a surname and no given name (#1054).
func TestMissingGivenName_CreateAndUpdate(t *testing.T) {
	server := setupTestServer()

	rec := send(server, http.MethodPost, "/api/v1/persons", `{"surname":"Tudor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create surname-only person: status %d: %s", rec.Code, rec.Body.String())
	}
	person := decodeBody(t, rec)
	id := person["id"].(string)
	if person["given_name"] != "" || person["surname"] != "Tudor" {
		t.Errorf("created person = %v/%v, want \"\"/Tudor", person["given_name"], person["surname"])
	}

	rec = send(server, http.MethodPost, "/api/v1/persons/"+id+"/names", `{"given_name":"","surname":"Tydder","name_type":"aka"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add surname-only name: status %d: %s", rec.Code, rec.Body.String())
	}
	nameID := decodeBody(t, rec)["id"].(string)

	rec = send(server, http.MethodPut, "/api/v1/persons/"+id+"/names/"+nameID, `{"given_name":"","surname":"Tidder"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update name to surname only: status %d: %s", rec.Code, rec.Body.String())
	}

	version := decodeBody(t, send(server, http.MethodGet, "/api/v1/persons/"+id, ""))["version"]
	rec = send(server, http.MethodPut, "/api/v1/persons/"+id, fmt.Sprintf(`{"given_name":"","notes":"Only a surname survives","version":%v}`, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("update surname-only person: status %d: %s", rec.Code, rec.Body.String())
	}
}

// An imported individual with no NAME has no name at all, and can still be
// updated in another field (#1054).
func TestMissingName_ImportedPersonIsEditable(t *testing.T) {
	server := setupTestServer()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n0 @I1@ INDI\n1 SEX M\n0 TRLR\n")
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: status %d: %s", rec.Code, rec.Body.String())
	}

	items := decodeBody(t, send(server, http.MethodGet, "/api/v1/persons", ""))["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("persons = %d, want 1", len(items))
	}
	p := items[0].(map[string]any)
	if p["given_name"] != "" || p["surname"] != "" {
		t.Fatalf("imported person = %v/%v, want no name", p["given_name"], p["surname"])
	}

	rec = send(server, http.MethodPut, "/api/v1/persons/"+p["id"].(string), fmt.Sprintf(`{"birth_place":"London","version":%v}`, p["version"]))
	if rec.Code != http.StatusOK {
		t.Fatalf("update nameless person: status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody(t, rec)["birth_place"]; got != "London" {
		t.Errorf("birth_place = %v, want London", got)
	}
}
