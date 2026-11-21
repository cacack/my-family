package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// authContractViolations lists where an OpenAPI document fails to describe
// sign-in (#1423): the session cookie must be a security scheme applied to
// every operation, a public operation must opt out with `security: []`, and
// every other operation must document the 401 `UNAUTHENTICATED` and 503
// `SETUP_REQUIRED` refusals authenticate gives it.
func authContractViolations(doc *openapi3.T) []string {
	var problems []string
	var scheme *openapi3.SecuritySchemeRef
	if doc.Components != nil {
		scheme = doc.Components.SecuritySchemes["session"]
	}
	if scheme == nil || scheme.Value.Type != "apiKey" || scheme.Value.In != "cookie" || scheme.Value.Name != sessionCookieName {
		problems = append(problems, fmt.Sprintf("components.securitySchemes.session is not the %s cookie", sessionCookieName))
	}
	if len(doc.Security) != 1 || len(doc.Security[0]) != 1 || doc.Security[0]["session"] == nil {
		problems = append(problems, "the top-level security is not the session scheme alone")
	}
	responds := func(op *openapi3.Operation, status, code string) bool {
		ref := op.Responses.Value(status)
		return ref != nil && ref.Value != nil && ref.Value.Description != nil && strings.Contains(*ref.Value.Description, code)
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			where := fmt.Sprintf("%s %s (%s)", method, path, op.OperationID)
			if role, _ := op.Extensions["x-min-role"].(string); role == string(rolePublic) {
				if op.Security == nil || len(*op.Security) != 0 {
					problems = append(problems, where+" is public but does not declare security: []")
				}
				continue
			}
			if op.Security != nil {
				problems = append(problems, where+" needs a session but overrides the top-level security")
			}
			if !responds(op, "401", CodeUnauthenticated) {
				problems = append(problems, where+" does not document 401 "+CodeUnauthenticated)
			}
			if !responds(op, "503", CodeSetupRequired) {
				problems = append(problems, where+" does not document 503 "+CodeSetupRequired)
			}
		}
	}
	return problems
}

func TestOpenAPI_DescribesSignIn(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData(openapiSpec)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range authContractViolations(doc) {
		t.Error(problem)
	}
}

func TestAuthContractViolations(t *testing.T) {
	const head = `openapi: 3.0.3
info: {title: t, version: "1"}
security: [{session: []}]
components:
  securitySchemes:
    session: {type: apiKey, in: cookie, name: myfamily_session}
  responses:
    Unauthenticated: {description: "UNAUTHENTICATED"}
    SetupRequired: {description: "SETUP_REQUIRED"}
paths:
`
	for _, tc := range []struct {
		name, paths string
		want        []string
	}{
		{
			name: "described",
			paths: `  /open:
    get:
      x-min-role: public
      security: []
      responses: {'200': {description: ok}}
  /things:
    get:
      x-min-role: viewer
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
`,
		},
		{
			name: "protected operation without its refusals",
			paths: `  /things:
    post:
      operationId: createThing
      x-min-role: editor
      responses: {'201': {description: ok}}
`,
			want: []string{"createThing) does not document 401", "createThing) does not document 503"},
		},
		{
			name: "public operation without security: []",
			paths: `  /open:
    get:
      operationId: getOpen
      x-min-role: public
      responses: {'200': {description: ok}}
`,
			want: []string{"getOpen) is public but does not declare security: []"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := openapi3.NewLoader().LoadFromData([]byte(head + tc.paths))
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(authContractViolations(doc), "\n")
			if len(tc.want) == 0 && got != "" {
				t.Errorf("violations:\n%s\nwant none", got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("violations:\n%s\nwant one containing %q", got, want)
				}
			}
		})
	}
	doc, err := openapi3.NewLoader().LoadFromData([]byte("openapi: 3.0.3\ninfo: {title: t, version: \"1\"}\npaths: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(authContractViolations(doc)); got != 2 {
		t.Errorf("a spec with no security scheme: %d violations, want 2 (scheme and top-level security)", got)
	}
}
