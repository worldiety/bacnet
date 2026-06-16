package errors

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidDeviceInstance indicates that a device instance is outside the BACnet range.
	ErrInvalidDeviceInstance = errors.New("invalid device instance")

	// ErrInvalidObjectType indicates that an object type is outside the BACnet identifier range.
	ErrInvalidObjectType = errors.New("invalid object type")

	// ErrInvalidObjectInstance indicates that an object instance is outside the BACnet range.
	ErrInvalidObjectInstance = errors.New("invalid object instance")

	// ErrInvalidMACAddress indicates that a BACnet MAC address is malformed.
	ErrInvalidMACAddress = errors.New("invalid MAC address")
)

// ValidationError describes an invalid value supplied to a BACnet helper.
type ValidationError struct {
	Field string
	Value any
	Cause error
}

func NewValidationError(field string, value any, cause error) *ValidationError {
	return &ValidationError{
		Field: field,
		Value: value,
		Cause: cause,
	}
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Field == "" {
		return fmt.Sprintf("validation failed: %v", e.Cause)
	}

	return fmt.Sprintf("invalid %s (%v): %v", e.Field, e.Value, e.Cause)
}

// Unwrap returns the underlying sentinel error.
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
