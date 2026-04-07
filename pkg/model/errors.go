package model

import "fmt"

// ErrCode is a machine-parseable error code for agent consumers
type ErrCode string

const (
	ErrTransportUnavailable ErrCode = "transport_unavailable"
	ErrTransportTimeout     ErrCode = "transport_timeout"
	ErrNotFound             ErrCode = "not_found"
	ErrPermissionDenied     ErrCode = "permission_denied"
	ErrAuthRequired         ErrCode = "auth_required"
	ErrAuthExpired          ErrCode = "auth_expired"
	ErrUnsupported          ErrCode = "unsupported_operation"
	ErrCorruptedData        ErrCode = "corrupted_data"
	ErrConflict             ErrCode = "conflict"
	ErrIO                   ErrCode = "io_error"
	ErrInvalidArgs          ErrCode = "invalid_args"
	ErrUnknownCommand       ErrCode = "unknown_command"
)

// CLIError is the structured error type returned as JSON to agent consumers
type CLIError struct {
	Message   string  `json:"error"`
	Code      ErrCode `json:"code"`
	Transport string  `json:"transport,omitempty"`
}

func (e *CLIError) Error() string {
	if e.Transport != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Transport, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewCLIError creates a structured error
func NewCLIError(code ErrCode, transport, msg string) *CLIError {
	return &CLIError{Code: code, Transport: transport, Message: msg}
}
