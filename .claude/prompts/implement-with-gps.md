# Implement with GPS (Genealogical Proof Standard) Support

Implement a feature with proper support for sources, citations, and evidence tracking.

---

## Context

Feature: $ARGUMENTS

## Instructions

When implementing this feature, ensure it aligns with GPS principles:

### 1. Source Support

Every piece of data should be traceable to a source:

```go
// Example: Facts should reference sources
type Fact struct {
    Value     string
    Date      *Date
    Place     *Place
    SourceID  *int      // Link to source
    Certainty string    // certain, probable, possible, uncertain
    Notes     string
}
```

### 2. Citation Handling

Support proper citation metadata:

- Source type (original, derivative, authored)
- Information type (primary, secondary, indeterminate)
- Evidence type (direct, indirect, negative)
- Full citation text (Evidence Explained format)

### 3. Evidence vs Conclusion

Separate what sources say from what we conclude:

- **Evidence**: Direct transcription/extraction from source
- **Conclusion**: Researcher's interpretation

### 4. Conflict Awareness

When data conflicts exist:

- Track conflicting evidence
- Don't silently overwrite
- Allow resolution documentation

### 5. Audit Trail

All changes should be traceable:

- Who made the change
- When it was made
- What the previous value was
- Why it was changed (optional note)

## Implementation Checklist

- [ ] Can users attach sources to this data?
- [ ] Is certainty/confidence level captured?
- [ ] Are conflicts detected and surfaced?
- [ ] Is change history preserved?
- [ ] Can users add research notes?

## Output

Implement the feature with these GPS considerations integrated, not bolted on afterward.
