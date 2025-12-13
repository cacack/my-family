# Write Tests

Write tests for a component, function, or feature following project patterns.

---

## Context

Target: $ARGUMENTS

## Instructions

Write comprehensive tests following the project's testing conventions.

### Go Unit Tests

Use table-driven tests:

```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:  "descriptive test name",
            input: InputType{...},
            want:  OutputType{...},
        },
        {
            name:    "error case",
            input:   InputType{...},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionUnderTest(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Test Categories

1. **Happy Path**: Normal, expected usage
2. **Edge Cases**: Empty inputs, max values, boundaries
3. **Error Cases**: Invalid inputs, missing data
4. **Genealogy-Specific**: Date parsing, name variants, GEDCOM quirks

### What to Test

For **API handlers**:
- Valid requests return correct status and data
- Invalid requests return appropriate errors
- Authentication/authorization enforced
- Edge cases (empty results, pagination boundaries)

For **Services/Business Logic**:
- Core logic produces expected outputs
- Validation rules enforced
- Error handling works correctly

For **Data Layer (Ent)**:
- CRUD operations work
- Relationships maintained correctly
- Constraints enforced

For **GEDCOM Processing**:
- Various date formats parsed correctly
- Name components extracted properly
- Edge cases (missing data, unusual structures)

### Test Naming

```go
// Format: TestFunctionName_Scenario_ExpectedResult
func TestParseName_WithSuffix_ExtractsSuffixCorrectly(t *testing.T)
func TestCreatePerson_MissingName_ReturnsValidationError(t *testing.T)
```

### Mocking

- Use interfaces for dependencies
- Create test doubles in `_test.go` files
- Consider testify/mock for complex mocking

### Integration Tests

For tests needing database:
- Use test containers or SQLite for isolation
- Clean up after each test
- Use `_integration_test.go` suffix if build-tagged

## Output

Provide:
1. Test file with comprehensive test cases
2. Any test helpers or fixtures needed
3. Instructions for running the tests
