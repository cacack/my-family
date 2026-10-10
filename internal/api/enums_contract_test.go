package api_test

import (
	"slices"
	"sort"
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
// (relationship_type carries one of two). Declaring one inline (a bare string,
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
