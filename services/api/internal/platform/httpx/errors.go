// Package httpx holds the HTTP plumbing shared by all modules: the one error envelope,
// request IDs and access logs. Business rules never live here.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/reqctx"
)

// envelope is the REQ-API-011 error format.
type envelope struct {
	Error body `json:"error"`
}

type body struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []detail `json:"details,omitempty"`
	RequestID string   `json:"request_id"`
}

type detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteError maps err to the error envelope. An *apperr.Error keeps its status and code;
// anything else becomes a 500 whose message is hidden from the client and logged.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	reqID := reqctx.RequestID(ctx)

	e, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(ctx, "request failed", "err", err, "request_id", reqID)
		e = &apperr.Error{Status: http.StatusInternalServerError, Code: "internal", Message: "Something went wrong. Quote the request_id to support."}
	}

	out := envelope{Error: body{Code: e.Code, Message: e.Message, RequestID: reqID}}
	for _, d := range e.Details {
		out.Error.Details = append(out.Error.Details, detail{Field: d.Field, Message: d.Message})
	}
	writeJSON(w, e.Status, out)
}

// BadRequest is for errors raised before a handler runs (bad JSON, bad path or query values).
func BadRequest(w http.ResponseWriter, r *http.Request, err error) {
	WriteError(w, r, apperr.Validation("request.invalid", "The request could not be read: "+err.Error()))
}

// WriteJSON writes a success body with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent; the best we can do is log.
		slog.Error("write response", "err", err)
	}
}
