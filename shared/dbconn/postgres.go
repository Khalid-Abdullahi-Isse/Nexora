// Package dbconn configures PostgreSQL without SQL parameter or DSN logging.
package dbconn

import (
	"errors"
	envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net"
	"net/url"
	"os"
	"time"
)

func Open(c envfolder.Config) (*gorm.DB, error) {
	u := url.URL{Scheme: "postgres", User: url.UserPassword(c.PostgresUser, c.PostgresPassword), Host: net.JoinHostPort(c.PostgresHost, c.PostgresPort), Path: "/" + c.PostgresDB}
	q := u.Query()
	mode := os.Getenv("POSTGRES_SSLMODE")
	if mode == "" {
		mode = "disable"
	}
	q.Set("sslmode", mode)
	q.Set("connect_timeout", "5")
	u.RawQuery = q.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	if e != nil {
		return nil, errors.New("PostgreSQL connection failed")
	}
	pool, e := db.DB()
	if e != nil {
		return nil, errors.New("PostgreSQL pool unavailable")
	}
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
