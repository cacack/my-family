package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/cacack/my-family/internal/repository"
)

// authContractViolations lists where an OpenAPI document fails to describe
// sign-in (#1423, #1416): the session cookie and the API token must be
// security schemes, either of which every operation accepts; a public
// operation must opt out with `security: []`; a viewer operation a visitor
// may call (not in signedInRoutes) must make both optional with
// `security: [{session: []}, {apiToken: []}, {}]`; an operation in
// sessionOnlyRoutes must declare `security: [{session: []}]`; and every
// non-public operation must document the 401 `UNAUTHENTICATED` and 503
// `SETUP_REQUIRED` refusals authenticate gives it.
func authContractViolations(doc *openapi3.T) []string {
	var problems []string
	var session, token *openapi3.SecuritySchemeRef
	if doc.Components != nil {
		session = doc.Components.SecuritySchemes["session"]
		token = doc.Components.SecuritySchemes["apiToken"]
	}
	if session == nil || session.Value.Type != "apiKey" || session.Value.In != "cookie" || session.Value.Name != sessionCookieName {
		problems = append(problems, fmt.Sprintf("components.securitySchemes.session is not the %s cookie", sessionCookieName))
	}
	if token == nil || token.Value.Type != "http" || token.Value.Scheme != "bearer" {
		problems = append(problems, "components.securitySchemes.apiToken is not an http bearer scheme")
	}
	if !securityIs(&doc.Security, "session", "apiToken") {
		problems = append(problems, "the top-level security is not the session or the API token")
	}
	responds := func(op *openapi3.Operation, status, code string) bool {
		ref := op.Responses.Value(status)
		return ref != nil && ref.Value != nil && ref.Value.Description != nil && strings.Contains(*ref.Value.Description, code)
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			where := fmt.Sprintf("%s %s (%s)", method, path, op.OperationID)
			role, _ := op.Extensions["x-min-role"].(string)
			if role == string(rolePublic) {
				if op.Security == nil || len(*op.Security) != 0 {
					problems = append(problems, where+" is public but does not declare security: []")
				}
				continue
			}
			key := routeKey(method, apiBase+specParam.ReplaceAllString(path, ":$1"))
			switch {
			case sessionOnlyRoutes[key]:
				if !securityIs(op.Security, "session") {
					problems = append(problems, where+" needs a session alone but does not declare security: [{session: []}]")
				}
			case role == string(repository.RoleViewer) && !signedInRoutes[key]:
				if !securityIs(op.Security, "session", "apiToken", "") {
					problems = append(problems, where+" may be called by a visitor but does not declare security: [{session: []}, {apiToken: []}, {}]")
				}
			case op.Security != nil:
				problems = append(problems, where+" needs a session or a token but overrides the top-level security")
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

// securityIs reports whether security is exactly the alternatives schemes,
// in any order, each one scheme alone; "" is the empty requirement (none).
func securityIs(security *openapi3.SecurityRequirements, schemes ...string) bool {
	if security == nil || len(*security) != len(schemes) {
		return false
	}
	want := make(map[string]bool, len(schemes))
	for _, scheme := range schemes {
		want[scheme] = true
	}
	for _, req := range *security {
		if len(req) > 1 {
			return false
		}
		name := ""
		for scheme := range req {
			name = scheme
		}
		if !want[name] {
			return false
		}
		delete(want, name)
	}
	return len(want) == 0
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
security: [{session: []}, {apiToken: []}]
components:
  securitySchemes:
    session: {type: apiKey, in: cookie, name: myfamily_session}
    apiToken: {type: http, scheme: bearer}
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
      security: [{session: []}, {apiToken: []}, {}]
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
  /auth/me/tokens:
    get:
      x-min-role: viewer
      security: [{session: []}]
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
  /history:
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
			name: "visitor-callable operation without an optional session",
			paths: `  /things:
    get:
      operationId: listThings
      x-min-role: viewer
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
`,
			want: []string{"listThings) may be called by a visitor but does not declare security: [{session: []}, {apiToken: []}, {}]"},
		},
		{
			name: "visitor-callable operation that refuses API tokens",
			paths: `  /things:
    get:
      operationId: listThings
      x-min-role: viewer
      security: [{session: []}, {}]
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
`,
			want: []string{"listThings) may be called by a visitor but does not declare security: [{session: []}, {apiToken: []}, {}]"},
		},
		{
			name: "session-only operation that accepts API tokens",
			paths: `  /auth/me/tokens:
    post:
      operationId: createAPIToken
      x-min-role: viewer
      responses:
        '201': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
`,
			want: []string{"createAPIToken) needs a session alone but does not declare security: [{session: []}]"},
		},
		{
			name: "signed-in operation with an optional session",
			paths: `  /history:
    get:
      operationId: getHistory
      x-min-role: viewer
      security: [{session: []}, {apiToken: []}, {}]
      responses:
        '200': {description: ok}
        '401': {$ref: '#/components/responses/Unauthenticated'}
        '503': {$ref: '#/components/responses/SetupRequired'}
`,
			want: []string{"getHistory) needs a session or a token but overrides the top-level security"},
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
	if got := len(authContractViolations(doc)); got != 3 {
		t.Errorf("a spec with no security scheme: %d violations, want 3 (two schemes and top-level security)", got)
	}
}
