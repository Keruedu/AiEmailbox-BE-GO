package handlers

import (
	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type KanbanHandler struct {
	repo    *repository.EmailRepository
	summary services.SummaryService
	cfg     *config.Config
}

func NewKanbanHandler(repo *repository.EmailRepository, summary services.SummaryService, cfg *config.Config) *KanbanHandler {
	return &KanbanHandler{repo: repo, summary: summary, cfg: cfg}
}

// Card represents the Kanban card shape returned to the client
type Card struct {
	ID           string     `json:"id"`
	Sender       string     `json:"sender"`
	Subject      string     `json:"subject"`
	Summary      string     `json:"summary"`
	Preview      string     `json:"preview"`
	GmailURL     string     `json:"gmail_url"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
	ReceivedAt   time.Time  `json:"received_at"`
}

// ColMeta describes a single column metadata item returned by /api/kanban/meta
type ColMeta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// MoveRequest is the payload for moving a card between columns
type MoveRequest struct {
	EmailID  string `json:"email_id" binding:"required"`
	ToStatus string `json:"to_status" binding:"required"`
}

// SnoozeRequest is the payload for snoozing a card until a given time
type SnoozeRequest struct {
	EmailID string `json:"email_id" binding:"required"`
	Until   string `json:"until" binding:"required"` // RFC3339
}

// SummarizeRequest requests generation of a summary for an email
type SummarizeRequest struct {
	EmailID string `json:"email_id" binding:"required"`
}

// GET /api/kanban
// GetKanban godoc
// @Summary Get Kanban board
// @Description Return kanban columns with cards
// @Tags kanban
// @Security ApiKeyAuth
// @Success 200 {object} map[string][]handlers.Card
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban [get]
func (h *KanbanHandler) GetKanban(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	ctx := c.Request.Context()
	board, err := h.repo.GetKanban(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := map[string][]Card{}
	for status, emails := range board {
		for _, e := range emails {
			sender := e.From.Email
			if e.From.Name != "" {
				sender = e.From.Name
			}
			card := Card{
				ID:           e.ID,
				Sender:       sender,
				Subject:      e.Subject,
				Summary:      e.Summary,
				Preview:      e.Preview,
				GmailURL:     e.GmailURL,
				SnoozedUntil: e.SnoozedUntil,
				ReceivedAt:   e.ReceivedAt,
			}
			resp[status] = append(resp[status], card)
		}
	}
	c.JSON(http.StatusOK, gin.H{"columns": resp})
}

// POST /api/kanban/move
// Move godoc
// @Summary Move a card to another column
// @Tags kanban
// @Security ApiKeyAuth
// @Param payload body handlers.MoveRequest true "Move payload"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/move [post]
func (h *KanbanHandler) Move(c *gin.Context) {
	var body MoveRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	if err := h.repo.UpdateStatus(ctx, body.EmailID, body.ToStatus); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/kanban/snooze
// Snooze godoc
// @Summary Snooze a card until a given time
// @Tags kanban
// @Security ApiKeyAuth
// @Param payload body handlers.SnoozeRequest true "Snooze payload"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/snooze [post]
func (h *KanbanHandler) Snooze(c *gin.Context) {
	var body SnoozeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	until, err := time.Parse(time.RFC3339, body.Until)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time format, use RFC3339"})
		return
	}
	ctx := c.Request.Context()
	if err := h.repo.SetSnooze(ctx, body.EmailID, until); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/kanban/summarize
// Summarize godoc
// @Summary Generate summary for an email
// @Tags kanban
// @Security ApiKeyAuth
// @Param payload body handlers.SummarizeRequest true "Summarize payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /kanban/summarize [post]
func (h *KanbanHandler) Summarize(c *gin.Context) {
	var body SummarizeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	summary, err := h.summary.SummarizeAndSave(ctx, body.EmailID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "summary": summary})
}

// GET /api/kanban/meta
// Returns ordered columns with keys and labels for frontend to render
func (h *KanbanHandler) Meta(c *gin.Context) {
	cols := h.cfg.KanbanColumns
	// helper to normalize label -> key
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "_")
		return s
	}

	// map some common labels to canonical status keys
	canonical := map[string]string{
		"inbox":       string(models.StatusInbox),
		"to do":       string(models.StatusTodo),
		"todo":        string(models.StatusTodo),
		"in progress": string(models.StatusInProgress),
		"in_progress": string(models.StatusInProgress),
		"done":        string(models.StatusDone),
		"snoozed":     string(models.StatusSnoozed),
	}

	var out []ColMeta
	for _, l := range cols {
		norm := strings.ToLower(strings.TrimSpace(l))
		key, ok := canonical[norm]
		if !ok {
			// fallback: normalized slug
			key = normalize(l)
		}
		out = append(out, ColMeta{Key: key, Label: l})
	}

	c.JSON(http.StatusOK, gin.H{"columns": out})
}
