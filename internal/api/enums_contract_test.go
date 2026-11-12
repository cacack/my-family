package api_test

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/cacack/my-family/internal/domain"
)

// toStrings converts a domain enum's values for comparison with the spec.
func toStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// domainEnums maps each enum schema in openapi.yaml to the domain type it
// mirrors. The frontend's types come from the spec, so a value the backend
// accepts but the spec omits (or the reverse) is a form that cannot save.
var domainEnums = map[string][]string{
	"Gender":            toStrings(domain.AllGenders()),
	"RelationType":      toStrings(domain.AllRelationTypes()),
	"ChildRelationType": toStrings(domain.AllChildRelationTypes()),
	"SourceType":        toStrings(domain.AllSourceTypes()),
	"SourceQuality":     toStrings(domain.AllSourceQualities()),
	"InformantType":     toStrings(domain.AllInformantTypes()),
	"EvidenceType":      toStrings(domain.AllEvidenceTypes()),
	"MediaType":         toStrings(domain.AllMediaTypes()),
	"NameType":          toStrings(domain.AllNameTypes()),
	"ResearchStatus":    toStrings(domain.AllResearchStatuses()),
	"ConflictStatus":    toStrings(domain.AllConflictStatuses()),
	"ResearchOutcome":   toStrings(domain.AllResearchOutcomes()),
	"FactType":          toStrings(domain.AllFactTypes()),

	"BranchStatus":           toStrings(domain.AllBranchStatuses()),
	"BranchSubjectType":      toStrings(domain.AllBranchSubjectTypes()),
	"BranchOutcome":          toStrings(domain.AllBranchOutcomes()),
	"BranchCloseOutcome":     toStrings(domain.AllBranchCloseOutcomes()),
	"LDSOrdinanceType":       toStrings(domain.AllLDSOrdinanceTypes()),
	"LDSOrdinanceStatus":     toStrings(domain.AllLDSOrdinanceStatuses()),
	"DateQualifier":          toStrings(domain.AllDateQualifiers()),
	"ResearchLogSubjectType": toStrings(domain.AllResearchLogSubjectTypes()),
}

func TestSpecEnumsMatchDomain(t *testing.T) {
	for name, want := range domainEnums {
		t.Run(name, func(t *testing.T) {
			ref := apiSpec.Components.Schemas[name]
			if ref == nil || ref.Value == nil {
				t.Fatalf("openapi.yaml has no schema %q", name)
			}
			got := make([]string, 0, len(ref.Value.Enum))
			for _, v := range ref.Value.Enum {
				s, ok := v.(string)
				if !ok {
					t.Fatalf("enum value %v is not a string", v)
				}
				got = append(got, s)
			}
			sort.Strings(got)
			want = slices.Clone(want)
			sort.Strings(want)
			if !slices.Equal(got, want) {
				t.Errorf("spec enum %s = %v, domain has %v", name, got, want)
			}
		})
	}
}

// governedProperties are property names that always carry a domain enum
// (relationship_type, status, outcome and subject_type carry one of several). Declaring one inline (a bare string,
// or a copied enum) would let it drift from the domain without
// TestSpecEnumsMatchDomain noticing.
var governedProperties = map[string][]string{
	"fact_type":         {"FactType"},
	"source_type":       {"SourceType"},
	"source_quality":    {"SourceQuality"},
	"informant_type":    {"InformantType"},
	"evidence_type":     {"EvidenceType"},
	"name_type":         {"NameType"},
	"research_status":   {"ResearchStatus"},
	"gender":            {"Gender"},
	"media_type":        {"MediaType"},
	"relationship_type": {"RelationType", "ChildRelationType"},
	"qualifier":         {"DateQualifier"},
	"status":            {"BranchStatus", "ConflictStatus", "LDSOrdinanceStatus"},
	"outcome":           {"ResearchOutcome", "BranchOutcome", "BranchCloseOutcome"},
	"subject_type":      {"ResearchLogSubjectType", "BranchSubjectType"},
}

// ungovernedProperties share a governed name but hold a different vocabulary.
var ungovernedProperties = map[string]bool{
	"MediaFile.media_type": true, // the raw GEDCOM MEDI tag, e.g. PHOTO
}

// enumRef is the schema a property references, directly or through a
// single-element allOf (the form that lets it carry a default or description).
func enumRef(prop *openapi3.SchemaRef) string {
	if prop.Ref == "" && prop.Value != nil && len(prop.Value.AllOf) == 1 {
		return prop.Value.AllOf[0].Ref
	}
	return prop.Ref
}

func TestGovernedPropertiesReferenceEnumSchema(t *testing.T) {
	for schemaName, schema := range apiSpec.Components.Schemas {
		if schema.Value == nil {
			t.Errorf("schema %s did not resolve", schemaName)
			continue
		}
		for prop, propRef := range schema.Value.Properties {
			enums, ok := governedProperties[prop]
			if !ok || ungovernedProperties[schemaName+"."+prop] {
				continue
			}
			got := enumRef(propRef)
			if !slices.ContainsFunc(enums, func(e string) bool { return got == "#/components/schemas/"+e }) {
				t.Errorf("%s.%s: want $ref to one of %v, got %q", schemaName, prop, enums, got)
			}
		}
	}
}

// enumKey is an enum's value set in a form that ignores order.
func enumKey(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x00")
}

// inlineEnumDuplicates lists every inline schema (a property, array item,
// allOf member, parameter, request or response body) whose enum has the same
// value set as a named enum schema. Such a copy drifts from the domain list
// without TestSpecEnumsMatchDomain noticing; it should $ref the schema.
func inlineEnumDuplicates(spec *openapi3.T) []string {
	// Two named schemas can share a value set (ResearchLogSubjectType and
	// BranchSubjectType), so a key maps to every schema that has it.
	named := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(spec.Components.Schemas)) {
		if s := spec.Components.Schemas[name].Value; s != nil && len(s.Enum) > 0 {
			key := enumKey(s.Enum)
			named[key] = append(named[key], name)
		}
	}

	var found []string
	var walk func(at string, ref *openapi3.SchemaRef)
	walkChildren := func(at string, s *openapi3.Schema) {
		for _, prop := range slices.Sorted(maps.Keys(s.Properties)) {
			walk(at+"."+prop, s.Properties[prop])
		}
		walk(at+"[]", s.Items)
		for i, sub := range s.AllOf {
			walk(fmt.Sprintf("%s.allOf[%d]", at, i), sub)
		}
		for i, sub := range s.OneOf {
			walk(fmt.Sprintf("%s.oneOf[%d]", at, i), sub)
		}
		for i, sub := range s.AnyOf {
			walk(fmt.Sprintf("%s.anyOf[%d]", at, i), sub)
		}
		walk(at+".additionalProperties", s.AdditionalProperties.Schema)
	}
	// A $ref is not inline: the named schema it points to is walked on its own.
	walk = func(at string, ref *openapi3.SchemaRef) {
		if ref == nil || ref.Ref != "" || ref.Value == nil {
			return
		}
		if len(ref.Value.Enum) > 0 {
			if names, ok := named[enumKey(ref.Value.Enum)]; ok {
				found = append(found, fmt.Sprintf("%s: inline enum duplicates %s", at, strings.Join(names, ", ")))
			}
		}
		walkChildren(at, ref.Value)
	}
	walkContent := func(at string, content openapi3.Content) {
		for _, mediaType := range slices.Sorted(maps.Keys(content)) {
			walk(at+" "+mediaType, content[mediaType].Schema)
		}
	}

	for name, ref := range spec.Components.Schemas {
		if ref.Value != nil {
			walkChildren(name, ref.Value)
		}
	}
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			at := method + " " + path
			for _, param := range append(slices.Clone(item.Parameters), op.Parameters...) {
				if param.Value != nil {
					walk(at+" param "+param.Value.Name, param.Value.Schema)
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				walkContent(at+" request", op.RequestBody.Value.Content)
			}
			for status, resp := range op.Responses.Map() {
				if resp.Value != nil {
					walkContent(at+" response "+status, resp.Value.Content)
				}
			}
		}
	}
	sort.Strings(found)
	return found
}

func TestNoInlineEnumDuplicatesNamedEnum(t *testing.T) {
	for _, dup := range inlineEnumDuplicates(apiSpec) {
		t.Error(dup)
	}
}

// TestInlineEnumDuplicatesDetects proves the check above can fail: a copied
// enum is reported wherever it sits and in any order, and a $ref is not.
func TestInlineEnumDuplicatesDetects(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromData([]byte(`
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /things:
    get:
      parameters:
        - {name: color, in: query, schema: {type: string, enum: [blue, red]}}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  colors: {type: array, items: {type: string, enum: [red, blue]}}
components:
  schemas:
    Color: {type: string, enum: [red, blue]}
    Thing:
      type: object
      properties:
        color: {type: string, enum: [blue, red]}
        ref: {$ref: '#/components/schemas/Color'}
        wrapped: {allOf: [{$ref: '#/components/schemas/Color'}]}
        size: {type: string, enum: [small, large]}
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /things param color: inline enum duplicates Color",
		"GET /things response 200 application/json.colors[]: inline enum duplicates Color",
		"Thing.color: inline enum duplicates Color",
	}
	if got := inlineEnumDuplicates(spec); !slices.Equal(got, want) {
		t.Errorf("inlineEnumDuplicates = %q, want %q", got, want)
	}
}
