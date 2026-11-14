// Package httpapi implements the HTTP handlers for openapi/openapi.yaml.
//
// openapi.gen.go is generated from the contract (make gen-go) and must not be
// edited by hand. This file only fills in the behavior behind each operation.
package httpapi

import (
	"context"
	"net/http"
	"time"
)

// readyTimeout bounds the database check. Without it a stopped database makes
// /readyz hang for the driver's 30 s server-selection timeout, and the
// orchestrator cannot tell "slow" from "down".
const readyTimeout = 2 * time.Second

// Pinger reports whether a dependency is reachable. *mongodb.Client satisfies
// it, and tests use a fake, so handlers never need a real database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server implements StrictServerInterface.
type Server struct {
	db Pinger
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns a Server that checks db for readiness.
func NewServer(db Pinger) *Server { return &Server{db: db} }

// NewHandler wires the strict server into a net/http mux.
func NewHandler(s *Server) http.Handler {
	return HandlerFromMux(NewStrictHandler(s, nil), http.NewServeMux())
}

// GetHealthz answers liveness. It never touches the database: a database
// outage must not make the orchestrator restart a healthy process.
func (s *Server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: Ok}, nil
}

// GetReadyz answers readiness: 200 if MongoDB answers, 503 otherwise. The
// underlying error is not sent to the client because it can leak internals.
func (s *Server) GetReadyz(ctx context.Context, _ GetReadyzRequestObject) (GetReadyzResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		return GetReadyz503ApplicationProblemPlusJSONResponse{
			Title:  "Servicio no disponible",
			Status: http.StatusServiceUnavailable,
			Code:   "database_unavailable",
		}, nil
	}
	return GetReadyz200JSONResponse{Status: Ok}, nil
}
