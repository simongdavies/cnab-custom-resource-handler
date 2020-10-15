package models

type Operation struct {
	Id         string                 `json:"id"`
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	Error      *OperationError        `json:"error,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type OperationError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
