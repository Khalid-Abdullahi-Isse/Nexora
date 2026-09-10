//go:build dependencies

package notificationservice

import (
	_ "github.com/go-playground/validator/v10"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/google/uuid"
	_ "github.com/gorilla/websocket"
	_ "github.com/joho/godotenv"
	_ "github.com/redis/go-redis/v9"
	_ "golang.org/x/crypto/bcrypt"
	_ "gorm.io/driver/postgres"
	_ "gorm.io/gorm"
)
