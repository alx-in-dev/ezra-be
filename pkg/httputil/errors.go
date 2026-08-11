package httputil

import "net/http"

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewBadRequest(code, message string) *AppError {
	return &AppError{Code: code, Message: message, Status: http.StatusBadRequest}
}

func NewNotFound(code, message string) *AppError {
	return &AppError{Code: code, Message: message, Status: http.StatusNotFound}
}

func NewUnauthorized(code, message string) *AppError {
	return &AppError{Code: code, Message: message, Status: http.StatusUnauthorized}
}

func NewForbidden(code, message string) *AppError {
	return &AppError{Code: code, Message: message, Status: http.StatusForbidden}
}

func NewConflict(code, message string) *AppError {
	return &AppError{Code: code, Message: message, Status: http.StatusConflict}
}

func NewInternal(message string) *AppError {
	return &AppError{Code: "internal_error", Message: message, Status: http.StatusInternalServerError}
}
