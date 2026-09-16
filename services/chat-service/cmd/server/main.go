package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/http"
	websocketcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/controller/websocket"
	postgresdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/chat-service/internal/service"
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

	verifier, err := authn.LoadVerifier("chat-service")
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
	redisClient, err := redisconn.Connect(ctx, cfg.Environment.RedisAddr, "chat-service")
	if err != nil {
		return err
	}
	defer redisconn.Close(redisClient, "chat-service")
	if len(os.Args) == 2 && os.Args[1] == "--check-redis" {
		return redisconn.Verify(ctx, redisClient, "chat-service")
	}
	limiter, err := ratelimit.New(redisClient, "chat-service", rateConfig.Timeout)
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
	rt, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	hub := websocketcontroller.NewHub(rt.MaxConnections)
	bus, err := websocketcontroller.NewBus(ctx, redisClient, hub)
	if err != nil {
		return err
	}
	defer bus.Close()
	defer hub.Close()
	application := service.New(postgres, bus)
	application.MaxLength = rt.MaxLength

	controller := httpcontroller.NewController(application)
	controller.Verifier = verifier
	var draining atomic.Bool
	controller.HealthCheck = func(c *gin.Context) {
		probe, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if draining.Load() || sqlDB.PingContext(probe) != nil {
			c.JSON(503, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	}
	controller.WebSocket = &websocketcontroller.Controller{Service: application, Hub: hub, Config: rt, Origins: origins, Limiter: limiter}
	identity := func(c *gin.Context) string { p, _ := authn.FromContext(c); return p.UserID }
	controller.Limit = limiter.Middleware(ratelimit.Policy{Name: "chat-events", Requests: rt.EventRequests, Window: time.Minute, FailClosed: true}, identity)
	controller.ConnectionLimit = limiter.Middleware(rateConfig.WebSocket, identity)
	var setupErr error
	router := httpcontroller.NewRouter(controller, func(r *gin.Engine) {
		r.Use(origins.CORS())
		r.Use(func(c *gin.Context) {
			if draining.Load() && c.FullPath() != "/health" {
				authn.Deny(c, 503, "UNAVAILABLE", "Service is shutting down")
				return
			}
			c.Next()
		})
		if err := ratelimit.Install(r, limiter, rateConfig, "chat-service"); err != nil {
			setupErr = err
		}
	})
	if setupErr != nil {
		return setupErr
	}

	fmt.Printf("Chat Service listening on port %s\n", cfg.Port)
	return server.Run("0.0.0.0:"+cfg.Port, router, func() { draining.Store(true); hub.Close() })
}
