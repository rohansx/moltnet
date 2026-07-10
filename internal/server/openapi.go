package server

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openapiJSON []byte

// handleOpenAPI serves the embedded OpenAPI 3.1 description of the REST API, so
// clients can generate bindings. The spec is baked into the binary.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=3600")
	_, _ = w.Write(openapiJSON)
}
