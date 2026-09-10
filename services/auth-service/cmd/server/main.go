package main

import (
	authservice "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/config"
	httpcontroller "github.com/Khalid-Abdullahi-Isse/social-media-backend/services/auth-service/internal/controller/http"
	"github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/ratelimit"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net"
	"net/url"
	"os"
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
	limiter, err := ratelimit.New(rateRedis, "auth-service", rateConfig.Timeout)
	if err != nil {
		log.Fatal(err)
	}

	env := cfg.Environment
	dsn := url.URL{Scheme: "postgres", User: url.UserPassword(env.PostgresUser, env.PostgresPassword), Host: net.JoinHostPort(env.PostgresHost, env.PostgresPort), Path: "/" + env.PostgresDB}
	sslmode := os.Getenv("POSTGRES_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}
	query := dsn.Query()
	query.Set("sslmode", sslmode)
	query.Set("connect_timeout", "5")
	dsn.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		log.Fatal("connect PostgreSQL: ", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	controller := authservice.BuildController(db)
	router := httpcontroller.NewRouter(controller, func(r *gin.Engine) {
		if err := ratelimit.Install(r, limiter, rateConfig, "auth-service"); err != nil {
			log.Fatal(err)
		}
	})
	if err := router.Run("0.0.0.0:" + cfg.Port); err != nil {
		log.Print(err)
	}
}
