package helpers

import (
	"net/http"

	"github.com/go-chi/render"
)

type RequestError struct {
	HTTPStatusCode int    `json:"statuscode"`
	Status         string `json:"status"`
	Message        string `json:"error,omitempty"`
}
type ErrorResponse struct {
	*RequestError `json:"ErrorResponse"`
}

func (e *ErrorResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrorInternalServerErrorFromError(err error) *ErrorResponse {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 500,
			Status:         "Internal Server Error",
			Message:        err.Error(),
		},
	}
}

func ErrorConflict(message string) render.Renderer {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 409,
			Status:         "Conflict",
			Message:        message,
		},
	}
}

func ErrorNotFound() render.Renderer {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 404,
			Status:         "Resource Not Found",
		},
	}
}

func ErrorInternalServerError(message string) *ErrorResponse {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 500,
			Status:         "Internal Server Error",
			Message:        message,
		},
	}
}

func ErrorInvalidRequestFromError(err error) render.Renderer {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 400,
			Status:         "Invalid Request",
			Message:        err.Error(),
		},
	}
}

func ErrorInvalidRequest(message string) render.Renderer {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 400,
			Status:         "Invalid Request",
			Message:        message,
		},
	}
}
