package httputil

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, err *AppError) {
	JSON(w, err.Status, map[string]string{
		"code":    err.Code,
		"message": err.Message,
	})
}
