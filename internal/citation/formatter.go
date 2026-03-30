package citation

import (
	"fmt"
	"strings"
	"sync"
	"text/template"
)

// compiledTemplates caches parsed text/templates keyed by "templateID:full" or "templateID:short".
var (
	compiledMu    sync.RWMutex
	compiledCache = make(map[string]*template.Template)
)

// FormatContext provides the data available to citation format templates.
type FormatContext struct {
	fields map[string]string
}

// Field returns the value for the given key, or empty string if not set.
func (c *FormatContext) Field(key string) string {
	return c.fields[key]
}

// FormatFull renders a full citation using the template and provided fields.
func FormatFull(tmpl *Template, fields map[string]string) (string, error) {
	return renderFormat(tmpl.ID, "full", tmpl.FullFormat, fields)
}

// FormatShort renders a short citation using the template and provided fields.
func FormatShort(tmpl *Template, fields map[string]string) (string, error) {
	return renderFormat(tmpl.ID, "short", tmpl.ShortFormat, fields)
}

func renderFormat(templateID, variant, formatStr string, fields map[string]string) (string, error) {
	t, err := getOrCompile(templateID, variant, formatStr)
	if err != nil {
		return "", fmt.Errorf("compile %s/%s template: %w", templateID, variant, err)
	}

	ctx := &FormatContext{fields: fields}
	var buf strings.Builder
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute %s/%s template: %w", templateID, variant, err)
	}
	return buf.String(), nil
}

func getOrCompile(templateID, variant, formatStr string) (*template.Template, error) {
	key := templateID + ":" + variant

	compiledMu.RLock()
	if t, ok := compiledCache[key]; ok {
		compiledMu.RUnlock()
		return t, nil
	}
	compiledMu.RUnlock()

	compiledMu.Lock()
	defer compiledMu.Unlock()

	// Double-check after acquiring write lock.
	if t, ok := compiledCache[key]; ok {
		return t, nil
	}

	t, err := template.New(key).Parse(formatStr)
	if err != nil {
		return nil, err
	}
	compiledCache[key] = t
	return t, nil
}
