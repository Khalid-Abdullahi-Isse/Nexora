package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/controller/http"
	websocketcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/controller/websocket"
	postgresdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/database/postgres"
	redisdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/database/redis"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/dbconn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/redisconn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/server"
	"github.com/gin-gonic/gin"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	verifier, err := authn.LoadVerifier("notification-service")
	if err != nil {
		return err
	}
	origins, err := httpsecurity.LoadOrigins()
	if err != nil {
		return err
	}
	rateConfig, err := ratelimit.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	redisClient, err := redisconn.Connect(ctx, cfg.Environment.RedisAddr, "notification-service")
	if err != nil {
		return err
	}
	defer redisconn.Close(redisClient, "notification-service")
	if len(os.Args) == 2 && os.Args[1] == "--check-redis" {
		return redisconn.Verify(ctx, redisClient, "notification-service")
	}
	limiter, err := ratelimit.New(redisClient, "notification-service", rateConfig.Timeout)
	if err != nil {
		return err
	}

	db, err := dbconn.Open(cfg.Environment)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	postgres := postgresdatabase.New(db)
	redis := redisdatabase.New(redisClient)
	application := service.New(postgres, redis)

	controller := httpcontroller.NewController(application)
	controller.Verifier = verifier
	controller.HealthCheck = redisconn.Health(redisClient, "notification-service", sqlDB.PingContext)
	var setupErr error
	router := httpcontroller.NewRouter(controller, func(r *gin.Engine) {
		r.Use(origins.CORS())
		if err := ratelimit.Install(r, limiter, rateConfig, "notification-service"); err != nil {
			setupErr = err
		}
	})
	if setupErr != nil {
		return setupErr
	}
	_ = websocketcontroller.NewController(application)

	fmt.Printf("Notification Service listening on port %s\n", cfg.Port)
	return server.Run("0.0.0.0:"+cfg.Port, router)
}
