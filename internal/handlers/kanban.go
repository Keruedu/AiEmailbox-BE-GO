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

	type ColMeta struct {
		Key   string `json:"key"`
		Label string `json:"label"`
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
