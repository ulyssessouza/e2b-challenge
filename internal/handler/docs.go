package handler

import (
	"embed"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Public, dependency-free API documentation: a hand-written OpenAPI 3 spec
// plus a Swagger UI page that loads the UI distribution from a CDN and
// points it at /openapi.json. Docs describe the contract; they grant no
// access, so they are served outside the JWT group.

//go:embed assets/openapi.json assets/swagger-ui.html
var docsFS embed.FS

func mustAsset(name string) []byte {
	b, err := docsFS.ReadFile(name)
	if err != nil {
		panic("handler: missing embedded doc asset: " + err.Error())
	}
	return b
}

var (
	openapiJSON   = mustAsset("assets/openapi.json")
	swaggerUIHTML = mustAsset("assets/swagger-ui.html")
)

// OpenAPI serves the raw OpenAPI 3 specification.
func OpenAPI(c echo.Context) error {
	return c.JSONBlob(http.StatusOK, openapiJSON)
}

// SwaggerUI serves the interactive documentation page.
func SwaggerUI(c echo.Context) error {
	return c.HTMLBlob(http.StatusOK, swaggerUIHTML)
}
