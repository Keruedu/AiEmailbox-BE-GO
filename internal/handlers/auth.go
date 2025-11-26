package handlers

import (
	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/utils"
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

type AuthHandler struct {
	cfg      *config.Config
	userRepo *repository.UserRepository
}

func NewAuthHandler(cfg *config.Config, userRepo *repository.UserRepository) *AuthHandler {
	return &AuthHandler{
		cfg:      cfg,
		userRepo: userRepo,
	}
}

// Signup handles email/password registration
func (h *AuthHandler) Signup(c *gin.Context) {
	var req models.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if user already exists
	existingUser, err := h.userRepo.FindByEmail(ctx, req.Email)
	if err == nil && existingUser != nil {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "user_exists",
			Message: "User with this email already exists",
		})
		return
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to process password",
		})
		return
	}

	// Create user
	user := &models.User{
		Email:    req.Email,
		Password: hashedPassword,
		Name:     req.Name,
		Provider: "email",
	}

	if err := h.userRepo.Create(ctx, user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create user",
		})
		return
	}

	// Generate tokens
	accessToken, err := utils.GenerateAccessToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTAccessExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := utils.GenerateRefreshToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTRefreshExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token
	if err := h.userRepo.UpdateRefreshToken(ctx, user.ID.Hex(), refreshToken); err != nil {
		// Log the actual error for debugging
		println("Signup - UpdateRefreshToken error:", err.Error(), "UserID:", user.ID.Hex())
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to store refresh token",
		})
		return
	}

	c.JSON(http.StatusCreated, models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// Login handles email/password authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user
	user, err := h.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "invalid_credentials",
				Message: "Invalid email or password",
			})
			return
		}
		// Log the actual error for debugging
		println("FindByEmail error:", err.Error())
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to find user",
		})
		return
	}

	if user.Provider != "email" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_credentials",
			Message: "Please use " + user.Provider + " to sign in",
		})
		return
	}

	// Check password
	if err := utils.CheckPassword(user.Password, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_credentials",
			Message: "Invalid email or password",
		})
		return
	}

	// Generate tokens
	accessToken, err := utils.GenerateAccessToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTAccessExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := utils.GenerateRefreshToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTRefreshExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Update refresh token
	if err := h.userRepo.UpdateRefreshToken(ctx, user.ID.Hex(), refreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// GoogleAuth handles Google OAuth authentication
func (h *AuthHandler) GoogleAuth(c *gin.Context) {
	var req models.GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: err.Error(),
		})
		return
	}

	// Verify Google token
	oauth2Service, err := oauth2.NewService(context.Background(), option.WithAPIKey(h.cfg.GoogleClientID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "google_auth_error",
			Message: "Failed to initialize Google auth service",
		})
		return
	}

	tokenInfo, err := oauth2Service.Tokeninfo().IdToken(req.Token).Do()
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_google_token",
			Message: "Failed to verify Google token",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if user exists with Google ID
	user, err := h.userRepo.FindByGoogleID(ctx, tokenInfo.UserId)
	if err != nil && err != mongo.ErrNoDocuments {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to find user",
		})
		return
	}

	// If user doesn't exist, create new user
	if user == nil {
		// Check if email already exists with different provider
		existingUser, _ := h.userRepo.FindByEmail(ctx, tokenInfo.Email)
		if existingUser != nil {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "email_exists",
				Message: "Email already registered with different provider",
			})
			return
		}

		user = &models.User{
			Email:    tokenInfo.Email,
			Name:     tokenInfo.Email,
			Provider: "google",
			GoogleID: tokenInfo.UserId,
		}

		if err := h.userRepo.Create(ctx, user); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "server_error",
				Message: "Failed to create user",
			})
			return
		}
	}

	// Generate tokens
	accessToken, err := utils.GenerateAccessToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTAccessExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := utils.GenerateRefreshToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTRefreshExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Update refresh token
	if err := h.userRepo.UpdateRefreshToken(ctx, user.ID.Hex(), refreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req models.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("RefreshToken - Bind error:", err.Error())
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: err.Error(),
		})
		return
	}

	println("RefreshToken - Received token:", req.RefreshToken[:20]+"...")

	// Validate refresh token
	claims, err := utils.ValidateToken(req.RefreshToken, h.cfg.JWTSecret)
	if err != nil {
		println("RefreshToken - Token validation error:", err.Error())
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_refresh_token",
			Message: "Invalid or expired refresh token",
		})
		return
	}

	println("RefreshToken - Token validated, UserID:", claims.UserID, "TokenType:", claims.TokenType)

	// Check if it's a refresh token
	if claims.TokenType != "refresh" {
		println("RefreshToken - Wrong token type:", claims.TokenType)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_token_type",
			Message: "Token is not a refresh token",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user and verify stored refresh token
	user, err := h.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		println("RefreshToken - User not found error:", err.Error(), "UserID:", claims.UserID)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_refresh_token",
			Message: "User not found",
		})
		return
	}

	println("RefreshToken - User found:", user.ID.Hex(), "Email:", user.Email)
	println("RefreshToken - Stored token:", user.RefreshToken[:20]+"...")
	println("RefreshToken - Request token:", req.RefreshToken[:20]+"...")
	println("RefreshToken - Tokens match:", user.RefreshToken == req.RefreshToken)

	if user.RefreshToken != req.RefreshToken {
		println("RefreshToken - Token mismatch!")
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_refresh_token",
			Message: "Refresh token not found or revoked",
		})
		return
	}

	// Generate new access token
	accessToken, err := utils.GenerateAccessToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTAccessExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	// Generate new refresh token (rotation)
	newRefreshToken, err := utils.GenerateRefreshToken(user.ID.Hex(), user.Email, h.cfg.JWTSecret, h.cfg.JWTRefreshExpiration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Update refresh token
	if err := h.userRepo.UpdateRefreshToken(ctx, user.ID.Hex(), newRefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken":  accessToken,
		"refreshToken": newRefreshToken,
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Revoke refresh token
	if err := h.userRepo.UpdateRefreshToken(ctx, userID.(string), ""); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to logout",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

// GetMe returns the current user's profile
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, user)
}
