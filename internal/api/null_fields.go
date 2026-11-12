package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
)

// apiBasePath is the base the generated routes are registered under, the
// spec's server URL.
const apiBasePath = "/api/v1"

// notNullableFields maps "METHOD /spec/path" of every PUT and PATCH operation
// with a JSON object body to the body properties that are not nullable.
// Built once, from the embedded spec.
var notNullableFields = sync.OnceValue(func() map[string]map[string]bool {
	doc, err := openapi3.NewLoader().LoadFromData(openapiSpec)
	if err != nil {
		panic(fmt.Sprintf("loading embedded OpenAPI spec: %v", err))
	}
	out := make(map[string]map[string]bool)
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if method != http.MethodPut && method != http.MethodPatch {
				continue
			}
			if op.RequestBody == nil || op.RequestBody.Value == nil {
				continue
			}
			media := op.RequestBody.Value.Content.Get(echo.MIMEApplicationJSON)
			if media == nil || media.Schema == nil || media.Schema.Value == nil {
				continue
			}
			fields := make(map[string]bool)
			for name, prop := range media.Schema.Value.Properties {
				if prop.Value != nil && !prop.Value.Nullable {
					fields[name] = true
				}
			}
			out[method+" "+path] = fields
		}
	}
	return out
})

// echoParam matches an Echo path parameter (":id"), which the spec writes as
// "{id}".
var echoParam = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

// refuseNullOnRequiredFields enforces the update convention (#1359): in a PUT
// or PATCH body an explicit JSON null clears an optional field, which the
// spec marks nullable. A null on any other property — a required field, or
// one that cannot be cleared — is refused with 400, because the generated
// types would otherwise read it as "left out" and silently keep the value.
func refuseNullOnRequiredFields(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if req.Method != http.MethodPut && req.Method != http.MethodPatch {
			return next(c)
		}
		route, ok := strings.CutPrefix(c.Path(), apiBasePath)
		if !ok {
			return next(c)
		}
		fields, ok := notNullableFields()[req.Method+" "+echoParam.ReplaceAllString(route, "{$1}")]
		if !ok || len(fields) == 0 || req.Body == nil {
			return next(c)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))

		var members map[string]json.RawMessage
		if json.Unmarshal(body, &members) != nil {
			return next(c) // not a JSON object: the binder reports it
		}
		var nulled []string
		for name, raw := range members {
			if fields[name] && string(bytes.TrimSpace(raw)) == "null" {
				nulled = append(nulled, name)
			}
		}
		if len(nulled) > 0 {
			sort.Strings(nulled)
			return fmt.Errorf("%w: %s cannot be null", command.ErrInvalidInput, strings.Join(nulled, ", "))
		}
		return next(c)
	}
}
