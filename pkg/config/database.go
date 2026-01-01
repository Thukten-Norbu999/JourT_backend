package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"fmt"
)

var DB *gorm.DB

func ConnectDB() *gorm.DB {
	// Try to load .env file, but don't fail if it doesn't exist (for production deployments)
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables directly")
	}

	// Check if DATABASE_URL is provided (Render's preferred method)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		log.Println("Using DATABASE_URL for connection")

		// Configure GORM with optimized settings for production
		config := &gorm.Config{
			PrepareStmt:                              true, // Cache prepared statements
			DisableForeignKeyConstraintWhenMigrating: true, // Faster migrations
		}

		db, err := gorm.Open(postgres.Open(databaseURL), config)
		if err != nil {
			log.Fatalf("Failed to connect to database using DATABASE_URL: %v", err)
		}

		// Configure connection pool for better performance
		sqlDB, err := db.DB()
		if err != nil {
			log.Fatalf("Failed to get underlying sql.DB: %v", err)
		}

		// Optimize connection pool settings
		sqlDB.SetMaxIdleConns(5)            // Reduced for free tier
		sqlDB.SetMaxOpenConns(10)           // Reduced for free tier
		sqlDB.SetConnMaxLifetime(time.Hour) // Connections expire after 1 hour

		log.Println("Connected to database successfully using DATABASE_URL")
		return db
	}

	// Fallback to individual environment variables
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	// Set default sslmode if not provided
	if sslmode == "" {
		sslmode = "require"
	}

	if host == "" || port == "" || user == "" || password == "" || dbname == "" {
		log.Println("❌ Missing database configuration!")
		log.Println("📋 Please set one of the following:")
		log.Println("   Option 1: DATABASE_URL (recommended for Render)")
		log.Println("   Option 2: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME")
		log.Println("")
		log.Println("🔧 For Render deployment, add a PostgreSQL database service")
		log.Println("   and it will automatically provide DATABASE_URL")
		log.Fatal("Database configuration required")
	}

	// Correct DSN formatting
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)

	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	DB = db
	fmt.Println("✅ Secure database connection established!")

	return DB
}
