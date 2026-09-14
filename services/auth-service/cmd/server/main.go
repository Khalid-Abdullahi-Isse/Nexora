package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	authservice "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/controller/http"
	authdb "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/database/postgres"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/authn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/dbconn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/httpsecurity"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/redisconn"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/server"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	verifier, err := authn.LoadVerifier("auth-service")
	if err != nil {
		return err
	}
	mint, err := service.LoadSigner()
	if err != nil {
		return err
	}
	probe, e := mint(authn.Principal{UserID: uuid.NewString(), Roles: []string{"user"}, SessionID: uuid.NewString(), AuthTime: time.Now().Unix()}, time.Now())
	if e != nil {
		return fmt.Errorf("signing key self-check failed")
	}
	if _, e = verifier.Verify(probe); e != nil {
		return fmt.Errorf("signing and verification keys do not match")
	}
	origins, err := httpsecurity.LoadOrigins()
	if err != nil {
		return err
	}
	ttl := 7 * 24 * time.Hour
	if raw := os.Getenv("REFRESH_TOKEN_TTL"); raw != "" {
		ttl, err = time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("invalid REFRESH_TOKEN_TTL")
		}
	}
	rateConfig, err := ratelimit.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	redisClient, err := redisconn.Connect(ctx, cfg.Environment.RedisAddr, "auth-service")
	if err != nil {
		return err
	}
	defer redisconn.Close(redisClient, "auth-service")
	if len(os.Args) == 2 && os.Args[1] == "--check-redis" {
		return redisconn.Verify(ctx, redisClient, "auth-service")
	}
	limiter, err := ratelimit.New(redisClient, "auth-service", rateConfig.Timeout)
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
	controller := authservice.BuildController(db)
	auth, err := service.NewAuthService(authdb.New(db), mint, ttl)
	if err != nil {
		return err
	}
	controller.ConfigureSecurity(httpcontroller.Security{Roles: service.NewRoleService(authdb.New(db)), Auth: auth, Verifier: verifier, Origins: origins, SecureCookie: os.Getenv("AUTH_COOKIE_INSECURE") != "true", Limiter: limiter, RateConfig: rateConfig})
	controller.HealthCheck = redisconn.Health(redisClient, "auth-service", sqlDB.PingContext)
	var setupErr error
	router := httpcontroller.NewRouter(controller, func(r *gin.Engine) {
		r.Use(origins.CORS())
		if err := ratelimit.Install(r, limiter, rateConfig, "auth-service"); err != nil {
			setupErr = err
		}
	})
	if setupErr != nil {
		return setupErr
	}
	return server.Run("0.0.0.0:"+cfg.Port, router)
}
