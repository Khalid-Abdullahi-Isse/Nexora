package http

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type SuccessResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

type ErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
