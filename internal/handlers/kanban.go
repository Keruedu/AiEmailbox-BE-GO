package handlers

import (
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type KanbanHandler struct {
	repo    *repository.EmailRepository
	summary services.SummaryService
}

func NewKanbanHandler(repo *repository.EmailRepository, summary services.SummaryService) *KanbanHandler {
	return &KanbanHandler{repo: repo, summary: summary}
}

// Card represents the Kanban card shape returned to the client
type Card struct {
	ID           string     `json:"id"`
	Sender       string     `json:"sender"`
	Subject      string     `json:"subject"`
	Summary      string     `json:"summary"`
	GmailURL     string     `json:"gmail_url"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

// GET /api/kanban
func (h *KanbanHandler) GetKanban(c *gin.Context) {
	ctx := c.Request.Context()
	board, err := h.repo.GetKanban(ctx)
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
				GmailURL:     e.GmailURL,
				SnoozedUntil: e.SnoozedUntil,
			}
			resp[status] = append(resp[status], card)
		}
	}
	c.JSON(http.StatusOK, gin.H{"columns": resp})
}

// POST /api/kanban/move
func (h *KanbanHandler) Move(c *gin.Context) {
	var body struct {
		EmailID  string `json:"email_id" binding:"required"`
		ToStatus string `json:"to_status" binding:"required"`
	}
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
func (h *KanbanHandler) Snooze(c *gin.Context) {
	var body struct {
		EmailID string `json:"email_id" binding:"required"`
		Until   string `json:"until" binding:"required"`
	}
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
func (h *KanbanHandler) Summarize(c *gin.Context) {
	var body struct {
		EmailID string `json:"email_id" binding:"required"`
	}
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
