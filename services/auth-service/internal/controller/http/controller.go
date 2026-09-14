package http

import (
	"errors"
	"net/http"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	security    *Security
	HealthCheck gin.HandlerFunc
	users       *service.UserService
}

func NewController(users *service.UserService) *Controller { return &Controller{users: users} }

func (c *Controller) Health(ctx *gin.Context) {
	if c != nil && c.HealthCheck != nil {
		c.HealthCheck(ctx)
		return
	}
	ctx.JSON(http.StatusOK, HealthResponse{Status: "ok", Service: "auth-service"})
}

func (c *Controller) Register(ctx *gin.Context) {
	var req RegisterRequest
	if err := httpsecurity.Decode(ctx, &req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Error: APIError{Code: "VALIDATION_ERROR", Message: "Invalid email or password"}})
		return
	}
	user, err := c.users.CreateUser(ctx.Request.Context(), service.CreateUserInput{Email: req.Email, Password: req.Password})
	if err != nil {
		status, code, message := http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to create account"
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			status, code, message = http.StatusBadRequest, "VALIDATION_ERROR", "Invalid email or password"
		case errors.Is(err, service.ErrEmailExists):
			status, code, message = http.StatusConflict, "EMAIL_EXISTS", "Email already exists"
		}
		ctx.JSON(status, ErrorResponse{Error: APIError{Code: code, Message: message}})
		return
	}
	ctx.JSON(http.StatusCreated, SuccessResponse{Success: true, Data: UserResponse{ID: user.ID, Email: user.Email, Status: string(user.Status), CreatedAt: user.CreatedAt}})
}
