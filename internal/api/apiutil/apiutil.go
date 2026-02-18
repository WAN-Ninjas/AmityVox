// Package apiutil provides shared JSON response helpers for the AmityVox REST
// API. All sub-packages under internal/api import this package instead of
// duplicating writeJSON / writeError / writeNoContent in every handler file.
package apiutil

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrorResponse is the standard error envelope returned by the API.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error code and human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SuccessResponse is the standard success envelope returned by the API.
type SuccessResponse struct {
	Data interface{} `json:"data"`
}

// WriteJSON writes a JSON response with the given status code and data wrapped
// in the standard success envelope {"data": ...}.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(SuccessResponse{Data: data})
}

// WriteJSONRaw writes a JSON response with the given status code without
// wrapping in the success envelope. Useful for responses that define their own
// structure.
func WriteJSONRaw(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a JSON error response with the given status code, error
// code, and message using the standard error envelope
// {"error": {"code": ..., "message": ...}}.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	})
}

// WriteNoContent writes a 204 No Content response with no body.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// DecodeJSON reads JSON from the request body into dst. On failure it writes a
// 400 error response and returns false so the caller can return early.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return false
	}
	return true
}

// InternalError logs the error and writes a generic 500 response. The msg
// parameter is used both as the log message and the user-facing message.
func InternalError(w http.ResponseWriter, logger *slog.Logger, msg string, err error) {
	logger.Error(msg, slog.String("error", err.Error()))
	WriteError(w, http.StatusInternalServerError, "internal_error", msg)
}

// WithTx runs fn inside a database transaction. It begins a transaction, calls
// fn, and commits if fn returns nil. If fn returns an error or panics, the
// transaction is rolled back. Post-commit work (event publishing, writing the
// HTTP response) should happen after WithTx returns nil.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
