package main

import (
	"context"
	"fmt"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/events"
	"log"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/controller/http"
	websocketcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/controller/websocket"
	postgresdatabase "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/notification-service/internal/database/postgres"
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
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
	hub := websocketcontroller.NewHub(cfg.Realtime.MaxConnections)
	defer hub.Close()
	bus, err := websocketcontroller.NewBus(ctx, redisClient, hub)
	if err != nil {
		return err
	}
	defer bus.Close()
	application := service.New(postgres, bus)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	var workers sync.WaitGroup
	consumer := &events.Consumer{Client: redisClient, Processor: application, Stream: cfg.Stream, Group: cfg.Group, RetryIdle: cfg.RetryIdle, MaxAttempts: cfg.MaxAttempts}
	if err = consumer.Ensure(ctx); err != nil {
		return err
	}
	workers.Add(1)
	go func() { defer workers.Done(); consumer.Run(workerCtx) }()
	var draining atomic.Bool
	shutdown := func() { draining.Store(true); stopWorker(); hub.Close(); workers.Wait() }
	defer shutdown()

	controller := httpcontroller.NewController(application)
	controller.Verifier = verifier
	controller.PageSize = cfg.PageSize
	ws := &websocketcontroller.Controller{Hub: hub, Config: cfg.Realtime, Origins: origins}
	controller.WebSocket = ws.Serve
	controller.ConnectionLimit = limiter.Middleware(rateConfig.WebSocket, func(c *gin.Context) string { p, _ := authn.FromContext(c); return p.UserID })
	controller.HealthCheck = redisconn.Health(redisClient, "notification-service", sqlDB.PingContext)
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
		if err := ratelimit.Install(r, limiter, rateConfig, "notification-service"); err != nil {
			setupErr = err
		}
	})
	if setupErr != nil {
		return setupErr
	}

	fmt.Printf("Notification Service listening on port %s\n", cfg.Port)
	return server.Run("0.0.0.0:"+cfg.Port, router, shutdown)
}
