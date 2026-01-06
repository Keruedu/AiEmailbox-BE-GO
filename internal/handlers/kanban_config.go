package handlers

import (
	"net/http"
	"strings"

	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// KanbanConfigHandler handles Kanban configuration endpoints
type KanbanConfigHandler struct {
	configRepo   *repository.KanbanConfigRepository
	emailRepo    *repository.EmailRepository
	gmailService *services.GmailService
	cfg          *config.Config
}

// NewKanbanConfigHandler creates a new handler
func NewKanbanConfigHandler(
	configRepo *repository.KanbanConfigRepository,
	emailRepo *repository.EmailRepository,
	gmailService *services.GmailService,
	cfg *config.Config,
) *KanbanConfigHandler {
	return &KanbanConfigHandler{
		configRepo:   configRepo,
		emailRepo:    emailRepo,
		gmailService: gmailService,
		cfg:          cfg,
	}
}

// ========== Column Configuration Endpoints ==========

// GetColumns godoc
// @Summary Get user's Kanban columns
// @Description Get the custom column configuration for the current user
// @Tags kanban-config
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} map[string][]models.KanbanColumn
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/columns [get]
func (h *KanbanConfigHandler) GetColumns(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx := c.Request.Context()

	// Initialize default columns if needed
	if err := h.configRepo.InitDefaultColumns(ctx, userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize columns"})
		return
	}

	columns, err := h.configRepo.GetColumns(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch columns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"columns": columns})
}

// CreateColumn godoc
// @Summary Create a new Kanban column
// @Tags kanban-config
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param payload body models.CreateColumnRequest true "Column data"
// @Success 201 {object} models.KanbanColumn
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/columns [post]
func (h *KanbanConfigHandler) CreateColumn(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.CreateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Get max order
	maxOrder, err := h.configRepo.GetMaxOrder(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get column order"})
		return
	}

	// Generate key from label
	key := h.generateKey(req.Label)

	column := &models.KanbanColumn{
		ID:         primitive.NewObjectID().Hex(),
		UserID:     userID.(string),
		Key:        key,
		Label:      req.Label,
		Order:      maxOrder + 1,
		GmailLabel: req.GmailLabel,
		Color:      req.Color,
		IsDefault:  false,
	}

	if err := h.configRepo.CreateColumn(ctx, column); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create column"})
		return
	}

	c.JSON(http.StatusCreated, column)
}

// UpdateColumn godoc
// @Summary Update a Kanban column
// @Tags kanban-config
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path string true "Column ID"
// @Param payload body models.UpdateColumnRequest true "Update data"
// @Success 200 {object} models.KanbanColumn
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/columns/{id} [put]
func (h *KanbanConfigHandler) UpdateColumn(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	columnID := c.Param("id")
	if columnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Column ID required"})
		return
	}

	var req models.UpdateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Verify column exists and belongs to user
	column, err := h.configRepo.GetColumnByID(ctx, columnID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Column not found"})
		return
	}
	if column.UserID != userID.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Build update map
	updates := make(map[string]interface{})
	if req.Label != "" {
		updates["label"] = req.Label
	}
	if req.GmailLabel != "" {
		updates["gmailLabel"] = req.GmailLabel
	}
	if req.Color != "" {
		updates["color"] = req.Color
	}
	if req.Order != nil {
		updates["order"] = *req.Order
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	if err := h.configRepo.UpdateColumn(ctx, columnID, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update column"})
		return
	}

	// Get updated column
	updatedColumn, _ := h.configRepo.GetColumnByID(ctx, columnID)
	c.JSON(http.StatusOK, updatedColumn)
}

// DeleteColumn godoc
// @Summary Delete a Kanban column
// @Tags kanban-config
// @Security ApiKeyAuth
// @Param id path string true "Column ID"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /kanban/columns/{id} [delete]
func (h *KanbanConfigHandler) DeleteColumn(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	columnID := c.Param("id")
	if columnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Column ID required"})
		return
	}

	ctx := c.Request.Context()

	// Verify column exists and belongs to user
	column, err := h.configRepo.GetColumnByID(ctx, columnID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Column not found"})
		return
	}
	if column.UserID != userID.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Prevent deleting default columns
	if column.IsDefault {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete default column"})
		return
	}

	if err := h.configRepo.DeleteColumn(ctx, columnID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete column"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ReorderColumns godoc
// @Summary Reorder Kanban columns
// @Tags kanban-config
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param payload body models.ReorderColumnsRequest true "Column IDs in new order"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} models.ErrorResponse
// @Router /kanban/columns/reorder [post]
func (h *KanbanConfigHandler) ReorderColumns(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.ReorderColumnsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	if err := h.configRepo.ReorderColumns(ctx, userID.(string), req.ColumnIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder columns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ========== Gmail Labels Endpoints ==========

// GetGmailLabels godoc
// @Summary Get available Gmail labels
// @Tags kanban-config
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} map[string][]models.GmailLabel
// @Failure 500 {object} models.ErrorResponse
// @Router /gmail/labels [get]
func (h *KanbanConfigHandler) GetGmailLabels(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx := c.Request.Context()

	labels, err := h.gmailService.GetLabels(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch Gmail labels: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"labels": labels})
}

// Helper: generate URL-safe key from label
func (h *KanbanConfigHandler) generateKey(label string) string {
	key := strings.ToLower(label)
	key = strings.ReplaceAll(key, " ", "_")
	result := ""
	for _, c := range key {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result += string(c)
		}
	}
	return "custom_" + result
}
