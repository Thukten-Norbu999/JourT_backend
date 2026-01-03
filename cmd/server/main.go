package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"jourt_backend/api"
	"jourt_backend/internal/models"
	"jourt_backend/pkg/config"

	"github.com/gin-gonic/gin"
)

func main() {
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
		log.Println("🚀 Running in RELEASE mode (production)")
	} else {
		gin.SetMode(gin.DebugMode)
		log.Println("🔧 Running in DEBUG mode (development)")

		log.Println("=== Environment Variables Debug ===")
		log.Printf("JWT_SECRET exists: %t", os.Getenv("JWT_SECRET") != "")
		log.Printf("secret_key exists: %t", os.Getenv("secret_key") != "")
		log.Printf("DB_HOST: %s", os.Getenv("DB_HOST"))
		log.Printf("DB_NAME: %s", os.Getenv("DB_NAME"))
		log.Printf("PORT: %s", os.Getenv("PORT"))
		log.Println("===================================")
	}

	app := gin.New()
	// app.Use(cors.Config{config.SetupCORS()})
	app.Use(config.SetupCORS())

	if ginMode == "release" {
		app.Use(gin.Recovery())
	} else {
		app.Use(gin.Logger(), gin.Recovery())
	}

	log.Println("📡 Connecting to database...")
	db := config.ConnectDB()

	log.Println("🔄 Running database migrations...")
	if err := db.AutoMigrate(
		&models.User{},
		&models.Trade{},
		&models.Journal{},
		&models.Screenshots{},
		&models.ImportJob{},
		&models.Setup{},
		&models.Psychology{},
		&models.Metrics{},
		&models.BacktestRun{},
	); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}
	log.Println("✅ Database migrations completed")

	log.Println("🛣️ Setting up routes...")
	api.SetupRoutes(app, db)

	log.Println("📋 Registered routes:")
	for _, r := range app.Routes() {
		log.Printf("➡️  %-6s %s", r.Method, r.Path)
	}

	// Server start
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      app,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("✅ API listening on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("🛑 Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited cleanly")
}
