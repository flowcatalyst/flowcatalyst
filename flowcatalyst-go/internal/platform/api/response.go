package api

import (
	"encoding/json"
	"net/http"

	"go.flowcatalyst.tech/internal/platform/common"
)

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// PagedResponse represents a paginated response
type PagedResponse[T any] struct {
	Data       []T   `json:"data"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalItems int64 `json:"totalItems"`
	TotalPages int   `json:"totalPages"`
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// WriteError writes a JSON error response
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
	})
}

// WriteErrorWithDetails writes a JSON error response with details
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message, details string) {
	WriteJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
		Details: details,
	})
}

// WriteBadRequest writes a 400 error
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "bad_request", message)
}

// WriteUnauthorized writes a 401 error
func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, "unauthorized", message)
}

// WriteForbidden writes a 403 error
func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, "forbidden", message)
}

// WriteNotFound writes a 404 error
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, "not_found", message)
}

// WriteConflict writes a 409 error
func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, "conflict", message)
}

// WriteInternalError writes a 500 error
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, "internal_error", message)
}

// WriteUseCaseError writes an error response based on UseCase error kind
func WriteUseCaseError(w http.ResponseWriter, err *common.UseCaseError) {
	switch err.Kind {
	case common.ErrorKindValidation:
		WriteError(w, http.StatusBadRequest, err.Code, err.Message)
	case common.ErrorKindNotFound:
		WriteError(w, http.StatusNotFound, err.Code, err.Message)
	case common.ErrorKindBusinessRule:
		WriteError(w, http.StatusConflict, err.Code, err.Message)
	case common.ErrorKindConcurrency:
		WriteError(w, http.StatusConflict, err.Code, err.Message)
	case common.ErrorKindUnauthorized:
		WriteError(w, http.StatusForbidden, err.Code, err.Message)
	default:
		WriteError(w, http.StatusInternalServerError, err.Code, err.Message)
	}
}

// WriteUseCaseResult writes a successful use case result or error
func WriteUseCaseResult[T any](w http.ResponseWriter, result common.Result[T], successStatus int) {
	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}
	WriteJSON(w, successStatus, result.Value())
}

// DecodeJSON decodes JSON from a request body
func DecodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// NewPagedResponse creates a new paged response
func NewPagedResponse[T any](data []T, page, pageSize int, totalItems int64) PagedResponse[T] {
	totalPages := int(totalItems) / pageSize
	if int(totalItems)%pageSize > 0 {
		totalPages++
	}

	return PagedResponse[T]{
		Data:       data,
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}
