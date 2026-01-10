package middleware

import (
	"aiemailbox-be/config"

	"github.com/gin-gonic/gin"
)

func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", cfg.FrontendURL)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		
		// PWA Caching Support: Allow service workers to cache responses
		// Set appropriate cache control headers for GET requests
		if c.Request.Method == "GET" {
			// Cache GET requests for 5 minutes (300 seconds)
			// Service Worker will use NetworkFirst strategy
			c.Writer.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
		} else {
			// Don't cache non-GET requests
			c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
