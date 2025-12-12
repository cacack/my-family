package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// APIError represents a standardized API error response.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error codes.
const (
	CodeBadRequest     = "BAD_REQUEST"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeInternalError  = "INTERNAL_ERROR"
	CodeValidation     = "VALIDATION_ERROR"
)

// customErrorHandler handles errors and returns consistent JSON responses.
func customErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var apiErr APIError
	code := http.StatusInternalServerError

	switch {
	case errors.Is(err, command.ErrPersonNotFound):
		code = http.StatusNotFound
		apiErr = APIError{
			Code:    CodeNotFound,
			Message: "Person not found",
		}
	case errors.Is(err, query.ErrNotFound):
		code = http.StatusNotFound
		apiErr = APIError{
			Code:    CodeNotFound,
			Message: "Resource not found",
		}
	case errors.Is(err, repository.ErrConcurrencyConflict):
		code = http.StatusConflict
		apiErr = APIError{
			Code:    CodeConflict,
			Message: "Resource was modified by another request. Please reload and try again.",
		}
	case errors.Is(err, command.ErrPersonHasFamilies):
		code = http.StatusConflict
		apiErr = APIError{
			Code:    CodeConflict,
			Message: "Person is linked to families and cannot be deleted",
		}
	case errors.Is(err, command.ErrFamilyNotFound):
		code = http.StatusNotFound
		apiErr = APIError{
			Code:    CodeNotFound,
			Message: "Family not found",
		}
	case errors.Is(err, command.ErrFamilyHasChildren):
		code = http.StatusConflict
		apiErr = APIError{
			Code:    CodeConflict,
			Message: "Family has children and cannot be deleted",
		}
	case errors.Is(err, command.ErrChildAlreadyLinked):
		code = http.StatusConflict
		apiErr = APIError{
			Code:    CodeConflict,
			Message: "Child is already linked to a family",
		}
	case errors.Is(err, command.ErrChildNotInFamily):
		code = http.StatusBadRequest
		apiErr = APIError{
			Code:    CodeBadRequest,
			Message: "Child is not in this family",
		}
	case errors.Is(err, command.ErrCircularAncestry):
		code = http.StatusConflict
		apiErr = APIError{
			Code:    CodeConflict,
			Message: "Circular ancestry detected - this would create an impossible family tree",
		}
	case errors.Is(err, command.ErrInvalidInput):
		code = http.StatusBadRequest
		apiErr = APIError{
			Code:    CodeValidation,
			Message: err.Error(),
		}
	default:
		// Check if it's an Echo HTTP error
		var he *echo.HTTPError
		if errors.As(err, &he) {
			code = he.Code
			if msg, ok := he.Message.(string); ok {
				apiErr = APIError{
					Code:    httpStatusToCode(code),
					Message: msg,
				}
			} else {
				apiErr = APIError{
					Code:    httpStatusToCode(code),
					Message: http.StatusText(code),
				}
			}
		} else {
			// Unknown error - return generic internal error
			apiErr = APIError{
				Code:    CodeInternalError,
				Message: "An unexpected error occurred",
			}
			// Log the actual error for debugging
			c.Logger().Error(err)
		}
	}

	c.JSON(code, apiErr)
}

// httpStatusToCode converts HTTP status to error code.
func httpStatusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	default:
		return CodeInternalError
	}
}

// NewAPIError creates a new API error with the given code and message.
func NewAPIError(code, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
	}
}

// WithDetails adds details to the error.
func (e *APIError) WithDetails(details map[string]any) *APIError {
	e.Details = details
	return e
}
