package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{Message: "field required"}
	assert.Equal(t, "field required", err.Error())
}

func TestValidationError_Unwrap(t *testing.T) {
	err := fmt.Errorf("wrap: %w", &ValidationError{Message: "bad input"})
	var ve *ValidationError
	assert.True(t, errors.As(err, &ve))
	assert.Equal(t, "bad input", ve.Message)
}

func TestSentinelErrors(t *testing.T) {
	assert.True(t, errors.Is(fmt.Errorf("wrapped: %w", ErrNotFound), ErrNotFound))
	assert.True(t, errors.Is(fmt.Errorf("wrapped: %w", ErrForbidden), ErrForbidden))
	assert.True(t, errors.Is(fmt.Errorf("wrapped: %w", ErrFileTooLarge), ErrFileTooLarge))
	assert.False(t, errors.Is(ErrNotFound, ErrForbidden))
}
