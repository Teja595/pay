package main

import (
	"log"
	"time"

	"payment-reconciliation-backend/internal/config"
	"payment-reconciliation-backend/internal/models"
	"payment-reconciliation-backend/internal/routes"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system env")
	}

	db := config.InitDB()

	db.AutoMigrate(
		&models.Invoice{},
		&models.BankTransaction{},
		&models.ReconciliationBatch{},
		&models.MatchAuditLog{},
	)

	r := gin.Default()
	// CORS config
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	routes.RegisterRoutes(r, db)

	r.Run(":8080")
}
