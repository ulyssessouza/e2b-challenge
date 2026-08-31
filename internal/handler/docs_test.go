package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestOpenAPIServesSpec(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/openapi.json", nil), rec)

	if err := OpenAPI(c); err != nil {
		t.Fatalf("OpenAPI returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var spec struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]json.RawMessage
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	if spec.OpenAPI != "3.0.3" || len(spec.Paths) == 0 {
		t.Fatalf("spec missing version/paths: %q, %d paths", spec.OpenAPI, len(spec.Paths))
	}
}

func TestSwaggerUIPage(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/swagger-ui.html", nil), rec)

	if err := SwaggerUI(c); err != nil {
		t.Fatalf("SwaggerUI returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=UTF-8" {
		t.Fatalf("expected text/html; charset=UTF-8, got %q", ct)
	}
}
