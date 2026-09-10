package main

import (
	"fmt"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/gin-gonic/gin"
	"log"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/http"
	websocketcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/websocket"
	postgresdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/database/postgres"
	redisdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/database/redis"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	rateConfig, err := ratelimit.Load()
	if err != nil {
		log.Fatal(err)
	}
	rateRedis, err := ratelimit.Client(cfg.Environment.RedisAddr, rateConfig.Timeout)
	if err != nil {
		log.Fatal("invalid rate limit Redis configuration")
	}
	defer rateRedis.Close()
	limiter, err := ratelimit.New(rateRedis, "chat-service", rateConfig.Timeout)
	if err != nil {
		log.Fatal(err)
	}

	postgres := postgresdatabase.New(nil)
	redis := redisdatabase.New(nil)
	application := service.New(postgres, redis)

	controller := httpcontroller.NewController(application)
	router := httpcontroller.NewRouter(controller, func(r *gin.Engine) {
		if err := ratelimit.Install(r, limiter, rateConfig, "chat-service"); err != nil {
			log.Fatal(err)
		}
	})
	_ = websocketcontroller.NewController(application)

	fmt.Printf("Chat Service listening on port %s\n", cfg.Port)
	if err := router.Run("0.0.0.0:" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
