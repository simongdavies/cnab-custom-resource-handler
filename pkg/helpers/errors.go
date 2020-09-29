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
