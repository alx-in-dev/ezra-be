package httputil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- NewBadRequest ---

func Test_NewBadRequest_Status(t *testing.T) {
	err := NewBadRequest("invalid_input", "bad request")
	assert.Equal(t, http.StatusBadRequest, err.Status)
}

func Test_NewBadRequest_Code(t *testing.T) {
	err := NewBadRequest("invalid_input", "bad request")
	assert.Equal(t, "invalid_input", err.Code)
}

func Test_NewBadRequest_Error(t *testing.T) {
	err := NewBadRequest("invalid_input", "bad request")
	assert.Equal(t, "bad request", err.Error())
}

// --- NewNotFound ---

func Test_NewNotFound_Status(t *testing.T) {
	err := NewNotFound("not_found", "resource not found")
	assert.Equal(t, http.StatusNotFound, err.Status)
}

func Test_NewNotFound_Error(t *testing.T) {
	err := NewNotFound("not_found", "resource not found")
	assert.Equal(t, "resource not found", err.Error())
}

// --- NewUnauthorized ---

func Test_NewUnauthorized_Status(t *testing.T) {
	err := NewUnauthorized("auth_required", "unauthorized")
	assert.Equal(t, http.StatusUnauthorized, err.Status)
}

func Test_NewUnauthorized_Error(t *testing.T) {
	err := NewUnauthorized("auth_required", "unauthorized")
	assert.Equal(t, "unauthorized", err.Error())
}

// --- NewForbidden ---

func Test_NewForbidden_Status(t *testing.T) {
	err := NewForbidden("access_denied", "forbidden")
	assert.Equal(t, http.StatusForbidden, err.Status)
}

func Test_NewForbidden_Error(t *testing.T) {
	err := NewForbidden("access_denied", "forbidden")
	assert.Equal(t, "forbidden", err.Error())
}

// --- NewConflict ---

func Test_NewConflict_Status(t *testing.T) {
	err := NewConflict("duplicate", "conflict")
	assert.Equal(t, http.StatusConflict, err.Status)
}

func Test_NewConflict_Error(t *testing.T) {
	err := NewConflict("duplicate", "conflict")
	assert.Equal(t, "conflict", err.Error())
}

// --- NewInternal ---

func Test_NewInternal_Status(t *testing.T) {
	err := NewInternal("something broke")
	assert.Equal(t, http.StatusInternalServerError, err.Status)
}

func Test_NewInternal_Code(t *testing.T) {
	err := NewInternal("something broke")
	assert.Equal(t, "internal_error", err.Code)
}

func Test_NewInternal_Error(t *testing.T) {
	err := NewInternal("something broke")
	assert.Equal(t, "something broke", err.Error())
}
