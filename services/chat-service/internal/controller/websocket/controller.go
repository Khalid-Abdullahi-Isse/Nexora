// Package websocket contains the chat WebSocket controller.
package websocket

// Service is the business-layer boundary available to WebSocket controllers.
// Message operations will be added when WebSocket behavior is implemented.
type Service interface{}

// Controller translates WebSocket communication to service calls.
type Controller struct {
	service Service
}

func NewController(service Service) *Controller {
	return &Controller{service: service}
}
