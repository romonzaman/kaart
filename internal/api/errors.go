package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/romonzaman/kaart/internal/store"
)

// Error codes returned to clients. These are part of the API contract: the
// frontend switches on them, so treat a rename as a breaking change.
const (
	codeInvalidRequest = "invalid_request"
	codeNotFound       = "not_found"
	codeConflict       = "conflict"
	codeInternal       = "internal"
)

// errorBody is the only error shape Kaart ever returns.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// apiError is an error carrying the status and code to report to the client.
type apiError struct {
	status  int
	code    string
	message string
	// cause is logged but never sent to the client.
	cause error
}

func (e *apiError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.code, e.message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e *apiError) Unwrap() error { return e.cause }

func badRequest(format string, args ...any) *apiError {
	return &apiError{
		status:  http.StatusBadRequest,
		code:    codeInvalidRequest,
		message: fmt.Sprintf(format, args...),
	}
}

func notFoundError(what string) *apiError {
	return &apiError{
		status:  http.StatusNotFound,
		code:    codeNotFound,
		message: what + " not found",
	}
}

func conflictError(format string, args ...any) *apiError {
	return &apiError{
		status:  http.StatusConflict,
		code:    codeConflict,
		message: fmt.Sprintf(format, args...),
	}
}

func internalError(cause error) *apiError {
	return &apiError{
		status:  http.StatusInternalServerError,
		code:    codeInternal,
		message: "something went wrong on the server",
		cause:   cause,
	}
}

// storeError translates a store error into the right client-facing error.
// Anything unrecognised becomes a 500 with the detail confined to the log —
// SQL text must never reach a response body.
func storeError(err error, what string) *apiError {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return notFoundError(what)
	case errors.Is(err, store.ErrConflict):
		return &apiError{
			status:  http.StatusConflict,
			code:    codeConflict,
			message: what + " is in a state that does not allow this operation",
			cause:   err,
		}
	default:
		return internalError(err)
	}
}

// writeError renders err as the standard error body. Non-apiError values are
// treated as internal failures.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		apiErr = internalError(err)
	}

	if apiErr.status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed",
			slog.String("code", apiErr.code),
			slog.String("error", apiErr.Error()),
		)
	} else {
		logger.DebugContext(r.Context(), "request rejected",
			slog.String("code", apiErr.code),
			slog.String("message", apiErr.message),
		)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiErr.status)

	body := errorBody{Error: errorDetail{Code: apiErr.code, Message: apiErr.message}}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.ErrorContext(r.Context(), "writing error response", slog.String("error", err.Error()))
	}
}
