package helpers

import (
	"net/http"

	"github.com/go-chi/render"
)

type RequestError struct {
	HTTPStatusCode int    `json:"-"`
	Status         string `json:"status"`
	Message        string `json:"error"`
}
type ErrorResponse struct {
	*RequestError `json:"ErrorResponse"`
}

func (e *ErrorResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrorInternalServerErrorFromError(err error) render.Renderer {
	return &ErrorResponse{
		&RequestError{
			HTTPStatusCode: 500,
			Status:         "Internal Server Error",
			Message:        err.Error(),
		},
	}
}

func ErrorInternalServerError(message string) render.Renderer {
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
