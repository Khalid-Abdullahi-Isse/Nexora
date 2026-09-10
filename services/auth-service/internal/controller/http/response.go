package http

import "time"

// HealthResponse preserves the health endpoint's public response format.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// SuccessResponse wraps successful API responses.
type SuccessResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

// ErrorResponse wraps a public error, never a raw database or Go error.
type ErrorResponse struct {
	Success bool     `json:"success"`
	Error   APIError `json:"error"`
}

// APIError contains a stable code and a client-safe message.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// UserResponse deliberately excludes all credential fields.
type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
