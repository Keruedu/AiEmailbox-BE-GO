package handlers

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"net/http"

	"github.com/gin-gonic/gin"
)

type StatisticsHandler struct {
	repo *repository.StatisticsRepository
}

func NewStatisticsHandler(repo *repository.StatisticsRepository) *StatisticsHandler {
	return &StatisticsHandler{repo: repo}
}

// GetStatistics godoc
// @Summary Get email statistics for dashboard
// @Description Returns comprehensive email statistics including status distribution, trends, top senders, and activity heatmap
// @Tags statistics
// @Security ApiKeyAuth
// @Param period query string false "Time period: 7d, 30d, 90d" default(30d)
// @Success 200 {object} models.StatisticsResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /statistics [get]
func (h *StatisticsHandler) GetStatistics(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse period parameter
	period := c.DefaultQuery("period", "30d")
	var days int
	switch period {
	case "7d":
		days = 7
	case "90d":
		days = 90
	default:
		days = 30
		period = "30d"
	}

	ctx := c.Request.Context()
	userIDStr := userID.(string)

	// Get status stats
	statusStats, err := h.repo.GetEmailsByStatus(ctx, userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get status stats: " + err.Error()})
		return
	}

	// Get email trend
	emailTrend, err := h.repo.GetEmailTrend(ctx, userIDStr, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get email trend: " + err.Error()})
		return
	}

	// Get top senders (limit 10)
	topSenders, err := h.repo.GetTopSenders(ctx, userIDStr, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get top senders: " + err.Error()})
		return
	}

	// Get daily activity
	dailyActivity, err := h.repo.GetDailyActivity(ctx, userIDStr, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get daily activity: " + err.Error()})
		return
	}

	// Get total and unread counts
	total, unread, starred, err := h.repo.GetTotalAndUnread(ctx, userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get counts: " + err.Error()})
		return
	}

	// Build response
	response := models.StatisticsResponse{
		StatusStats:   statusStats,
		EmailTrend:    emailTrend,
		TopSenders:    topSenders,
		DailyActivity: dailyActivity,
		TotalEmails:   total,
		UnreadCount:   unread,
		StarredCount:  starred,
		Period:        period,
	}

	c.JSON(http.StatusOK, response)
}
