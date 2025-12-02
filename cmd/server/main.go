// @title AI Email Box API
// @version 1.0
// @description Backend API for AI Email Box
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@example.com
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

package main

import (
	"aiemailbox-be/config"
	"aiemailbox-be/internal/database"
	"aiemailbox-be/internal/handlers"
	"aiemailbox-be/internal/middleware"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
	"log"

	"github.com/gin-gonic/gin"

	_ "aiemailbox-be/docs"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Connect to MongoDB
	mongodb, err := database.NewMongoDB(cfg.MongoDBURI, cfg.MongoDBDatabase)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer mongodb.Disconnect()

	// Initialize repositories
	userRepo := repository.NewUserRepository(mongodb.Database)
	// emailRepo := repository.NewEmailRepository(mongodb.Database) // Not used for Gmail track

	// Initialize services
	gmailService := services.NewGmailService(cfg)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(cfg, userRepo)
	emailHandler := handlers.NewEmailHandler(gmailService, userRepo)

	// Initialize Gin
	r := gin.Default()

	// Apply CORS middleware
	r.Use(middleware.CORS(cfg))

	// Public routes
	public := r.Group("/api")
	{
		// Health check
		public.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":   "ok",
				"message":  "AI Email Box API is running",
				"database": "MongoDB connected",
			})
		})

		// Auth routes
		auth := public.Group("/auth")
		{
			auth.POST("/signup", authHandler.Signup)
			auth.POST("/login", authHandler.Login)
			auth.POST("/google", authHandler.GoogleAuth)
			auth.POST("/refresh", authHandler.RefreshToken)
		}
	}

	// Protected routes
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware(cfg))
	{
		// Auth protected routes
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/auth/me", authHandler.GetMe)

		// Email routes
		protected.GET("/mailboxes", emailHandler.GetMailboxes)
		protected.GET("/mailboxes/:mailboxId/emails", emailHandler.GetEmails)
		protected.GET("/emails/:emailId", emailHandler.GetEmailDetail)
	}

	// Start server
	log.Printf("Server starting on port %s", cfg.Port)
	log.Printf("Connected to MongoDB: %s", cfg.MongoDBDatabase)
	// Swagger route
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
