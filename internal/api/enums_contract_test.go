package api_test

import (
	"slices"
	"sort"
	"testing"

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

// governedProperties are property names that always carry one domain enum.
// Declaring one inline (a bare string, or a copied enum) would let it drift
// from the domain without TestSpecEnumsMatchDomain noticing.
var governedProperties = map[string]string{
	"fact_type":       "FactType",
	"source_type":     "SourceType",
	"source_quality":  "SourceQuality",
	"informant_type":  "InformantType",
	"evidence_type":   "EvidenceType",
	"name_type":       "NameType",
	"research_status": "ResearchStatus",
	"gender":          "Gender",
}

func TestGovernedPropertiesReferenceEnumSchema(t *testing.T) {
	for schemaName, schema := range apiSpec.Components.Schemas {
		for prop, propRef := range schema.Value.Properties {
			enum, ok := governedProperties[prop]
			if !ok {
				continue
			}
			if want := "#/components/schemas/" + enum; propRef.Ref != want {
				t.Errorf("%s.%s: want $ref %s, got %q", schemaName, prop, want, propRef.Ref)
			}
		}
	}
}
