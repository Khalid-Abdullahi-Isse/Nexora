package http

import (
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, controller *Controller) {
	router.GET("/health", controller.Health)
	router.GET("/ready", controller.Ready)
	api := router.Group("/api/v1")
	api.GET("/posts", controller.ListPosts)
	api.GET("/posts/:id", controller.GetPost)
	api.GET("/users/:userId/posts", controller.GetUserPosts)
	var verifier *authn.Verifier
	if controller != nil {
		verifier = controller.Verifier
	}
	protected := api.Group("", verifier.Middleware())
	protected.POST("/posts", controller.CreatePost)
	protected.POST("/posts/:id/likes", controller.LikePost)
	protected.POST("/posts/:id/comments", controller.CommentPost)
	protected.PATCH("/posts/:id", controller.UpdatePost)
	protected.DELETE("/posts/:id", controller.DeletePost)
}
