// Package api implements the HTTP contract defined in api/openapi/openapi.yaml.
// Routes and request/response types come from the generated package gen.
package api

import (
	"context"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
)

// Server implements gen.StrictServerInterface. Module handlers are added here
// as the modules are built.
type Server struct{}

// NewServer returns a Server with no dependencies yet.
func NewServer() *Server {
	return &Server{}
}

// GetHealth reports that the process is up. Readiness checks (database, queue)
// are added once those dependencies exist.
func (s *Server) GetHealth(_ context.Context, _ gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	return gen.GetHealth200JSONResponse{Status: "ok"}, nil
}
