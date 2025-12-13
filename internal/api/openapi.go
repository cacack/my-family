// Package api provides the HTTP API server for the genealogy application.
package api

import (
	_ "embed"
	"net/http"
	"strings"
	"text/template"

	"github.com/labstack/echo/v4"
)

//go:embed openapi.yaml
var openapiSpec []byte

// swaggerUITemplate is a minimal Swagger UI HTML page
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>My Family API Documentation</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "{{.SpecURL}}",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
    };
  </script>
</body>
</html>`

var swaggerUITemplate = template.Must(template.New("swagger-ui").Parse(swaggerUIHTML))

// registerDocsRoutes registers the API documentation endpoints.
func (s *Server) registerDocsRoutes(api *echo.Group) {
	// Serve raw OpenAPI spec
	api.GET("/openapi.yaml", s.serveOpenAPISpec)

	// Serve Swagger UI
	api.GET("/docs", s.serveSwaggerUI)
}

// serveOpenAPISpec returns the OpenAPI specification as YAML.
func (s *Server) serveOpenAPISpec(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "application/x-yaml")
	return c.Blob(http.StatusOK, "application/x-yaml", openapiSpec)
}

// serveSwaggerUI returns the Swagger UI HTML page.
func (s *Server) serveSwaggerUI(c echo.Context) error {
	data := struct {
		SpecURL string
	}{
		SpecURL: "/api/v1/openapi.yaml",
	}

	var buf strings.Builder
	if err := swaggerUITemplate.Execute(&buf, data); err != nil {
		return err
	}
	return c.HTML(http.StatusOK, buf.String())
}

// OpenAPISpec returns the embedded OpenAPI specification.
func OpenAPISpec() []byte {
	return openapiSpec
}
