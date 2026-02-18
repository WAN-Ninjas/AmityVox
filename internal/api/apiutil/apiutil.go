// Package apiutil provides shared JSON response helpers for the AmityVox REST
// API. All sub-packages under internal/api import this package instead of
// duplicating writeJSON / writeError / writeNoContent in every handler file.
package apiutil

import (
	"encoding/json"
	"net/http"
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
