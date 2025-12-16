package handlers

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
	"aiemailbox-be/internal/utils"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type EmailHandler struct {
	gmailService *services.GmailService
	userRepo     *repository.UserRepository
	emailRepo    *repository.EmailRepository
}

func NewEmailHandler(gmailService *services.GmailService, userRepo *repository.UserRepository, emailRepo *repository.EmailRepository) *EmailHandler {
	return &EmailHandler{
		gmailService: gmailService,
		userRepo:     userRepo,
		emailRepo:    emailRepo,
	}
}

// GetMailboxes returns all mailboxes for the authenticated user
// GetMailboxes godoc
// @Summary      Get mailboxes
// @Description  Returns all mailboxes for the authenticated user
// @Tags         emails
// @Produce      json
// @Success      200  {object}  models.MailboxesResponse
// @Failure      401  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /mailboxes [get]
func (h *EmailHandler) GetMailboxes(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	mailboxes, err := h.gmailService.ListMailboxes(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to load mailboxes: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.MailboxesResponse{
		Mailboxes: mailboxes,
	})
}

// GetEmails returns emails for a specific mailbox with pagination
// GetEmails godoc
// @Summary      List emails
// @Description  Returns emails for a specific mailbox with pagination
// @Tags         emails
// @Produce      json
// @Param        mailboxId   path      string  true  "Mailbox ID"
// @Param        page        query     int     false "Page number"
// @Param        limit       query     int     false "Items per page"
// @Success      200  {object}  models.EmailListResponse
// @Failure      401  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /mailboxes/{mailboxId}/emails [get]
func (h *EmailHandler) GetEmails(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	mailboxID := c.Param("mailboxId")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	emails, total, err := h.gmailService.ListEmails(ctx, user, mailboxID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to load emails: " + err.Error(),
		})
		return
	}

	// Sync emails to database for Kanban
	// Note: In a real app, this should be a background job or separate goroutine
	// but here we do it inline for simplicity to ensure Kanban is populated.
	// We run it in a goroutine so user doesn't wait too long, but context needs to be background then.
	// Actually, let's do it synchronously to ensure data is there if they switch tabs immediately,
	// or use a detached context.
	go func(emails []*models.Email) {
		syncCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		for _, e := range emails {
			// Preserve existing status if exists, else default to Inbox
			existing, err := h.emailRepo.GetByID(syncCtx, e.ID)
			if err == nil && existing != nil {
				e.Status = existing.Status
				e.SnoozedUntil = existing.SnoozedUntil
				e.Summary = existing.Summary
			} else {
				e.Status = models.StatusInbox
			}
			e.UserID = user.ID.Hex()
			_ = h.emailRepo.UpsertEmail(syncCtx, e)
		}
	}(emails)

	c.JSON(http.StatusOK, models.EmailListResponse{
		Emails:      emails,
		Total:       total, // This is estimate
		Page:        page,
		PerPage:     perPage,
		HasNextPage: false, // Simplified for now
	})
}

// SearchEmails searches for emails
// SearchEmails godoc
// @Summary      Search emails
// @Description  Search emails by fuzzy query (subject, sender, summary)
// @Tags         emails
// @Produce      json
// @Param        q           query     string  true  "Search query"
// @Success      200  {object}  []models.Email
// @Failure      401  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /emails/search [get]
func (h *EmailHandler) SearchEmails(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	query := c.Query("q")
	pageToken := c.Query("pageToken")

	if query == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Query parameter 'q' is required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	// 1. Gmail API Search (Primary - Exact/Global)
	gmailEmails, nextPageToken, estimate, err := h.gmailService.SearchEmails(ctx, user, query, pageToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "search_error",
			Message: "Failed to search emails: " + err.Error(),
		})
		return
	}

	// 2. Local MongoDB Search (Secondary - Partial Regex)
	localEmails, err := h.emailRepo.SearchEmails(ctx, user.ID.Hex(), query)
	if err != nil {
		// Log error but continue with Gmail results
		localEmails = []models.Email{}
	}

	// Merge results (Deduplicate by ID)
	emailMap := make(map[string]models.Email)
	for _, e := range gmailEmails {
		emailMap[e.ID] = *e
	}
	for _, e := range localEmails {
		if _, exists := emailMap[e.ID]; !exists {
			emailMap[e.ID] = e
		}
	}

	// 3. Fuzzy Search Fallback (If no results found)
	// Only if generic query (not too short) and no results so far.
	if len(emailMap) == 0 && len(query) > 3 {
		// Fetch all local emails (excluding trash, via GetKanban)
		kanbanMap, err := h.emailRepo.GetKanban(ctx, user.ID.Hex())
		if err == nil {
			const fuzzyThreshold = 3 // Distance threshold
			for _, list := range kanbanMap {
				for _, e := range list {
					// Normalize for fuzzy check
					normQuery := utils.RemoveAccents(query)
					normSubject := utils.RemoveAccents(e.Subject)

					// Simple optimization: check length diff first
					if abs(len(normSubject)-len(normQuery)) > fuzzyThreshold {
						// Check summary next
					} else {
						distSub := utils.Levenshtein(normQuery, normSubject)
						if distSub <= fuzzyThreshold {
							emailMap[e.ID] = e
							continue
						}
					}

					// Also check summary if exists
					if e.Summary != "" {
						normSummary := utils.RemoveAccents(e.Summary)
						if abs(len(normSummary)-len(normQuery)) > fuzzyThreshold {
							continue
						}
						distSum := utils.Levenshtein(normQuery, normSummary)
						if distSum <= fuzzyThreshold {
							emailMap[e.ID] = e
							continue
						}
					}
				}
			}
		}
	}

	// Convert map to slice
	finalEmails := make([]*models.Email, 0, len(emailMap))
	for _, e := range emailMap {
		val := e // copy
		finalEmails = append(finalEmails, &val)
	}

	// Sync valid Gmail results to local DB (as before)
	// We only sync the DIRECT Gmail results to ensure we have the latest data for them.
	// Local results are already local.
	if len(gmailEmails) > 0 {
		go func(emails []*models.Email) {
			syncCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			for _, e := range emails {
				existing, err := h.emailRepo.GetByID(syncCtx, e.ID)
				if err == nil && existing != nil {
					e.Status = existing.Status
					e.SnoozedUntil = existing.SnoozedUntil
					e.Summary = existing.Summary
				} else {
					e.Status = models.StatusInbox
				}
				e.UserID = user.ID.Hex()
				_ = h.emailRepo.UpsertEmail(syncCtx, e)
			}
		}(gmailEmails)
	}

	// Calculate total estimate (max of Gmail estimate or actual count)
	totalEstimate := estimate
	if len(finalEmails) > totalEstimate {
		totalEstimate = len(finalEmails)
	}

	c.JSON(http.StatusOK, gin.H{
		"emails":        finalEmails,
		"nextPageToken": nextPageToken,
		"totalEstimate": totalEstimate,
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetEmailDetail returns detailed information about a specific email
// GetEmailDetail godoc
// @Summary      Get email detail
// @Description  Returns detailed information about a specific email
// @Tags         emails
// @Produce      json
// @Param        emailId   path      string  true  "Email ID"
// @Success      200  {object}  models.Email
// @Failure      401  {object}  models.ErrorResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /emails/{emailId} [get]
func (h *EmailHandler) GetEmailDetail(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	emailID := c.Param("emailId")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	email, err := h.gmailService.GetEmail(ctx, user, emailID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "email_not_found",
				Message: "Email not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to load email: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, email)
}

// SendEmail sends a new email
func (h *EmailHandler) SendEmail(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	email := &models.Email{
		To:      []models.EmailAddress{{Email: req.To}},
		Subject: req.Subject,
		Body:    req.Body,
	}

	if err := h.gmailService.SendEmail(ctx, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to send email: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email sent successfully"})
}

// ReplyEmail replies to an existing email
func (h *EmailHandler) ReplyEmail(c *gin.Context) {
	// For now, this is same as SendEmail but could be enhanced to check thread ID
	h.SendEmail(c)
}

// ModifyEmail modifies email labels (mark read/unread, star, delete)
func (h *EmailHandler) ModifyEmail(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	emailID := c.Param("emailId")
	var req struct {
		AddLabels    []string `json:"addLabels"`
		RemoveLabels []string `json:"removeLabels"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	if err := h.gmailService.ModifyEmail(ctx, user, emailID, req.AddLabels, req.RemoveLabels); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to modify email: " + err.Error(),
		})
		return
	}

	// Sync changes to local database immediately to reflect in Kanban/other views
	// Fetch fresh details from Gmail to get current labels/state
	updatedEmail, err := h.gmailService.GetEmail(ctx, user, emailID)
	if err == nil {
		// Preserve local fields that aren't on Gmail (like custom status, snoozedUntil if any - though modify might affect them)
		// For now, simple Upsert is safe as it overwrites with fresh Gmail data.
		// However, we must ensure we don't lose custom Status if it's not stored in Gmail.
		// Actually, GetEmail from GmailService might return a default Status if not found map?
		// Let's check repository.GetByID first.
		existing, _ := h.emailRepo.GetByID(ctx, emailID)
		if existing != nil {
			updatedEmail.Status = existing.Status
			updatedEmail.SnoozedUntil = existing.SnoozedUntil
			updatedEmail.Summary = existing.Summary
		} else {
			// If not in DB, default?
			updatedEmail.Status = models.StatusInbox
		}
		updatedEmail.UserID = user.ID.Hex()
		_ = h.emailRepo.UpsertEmail(ctx, updatedEmail)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email modified successfully"})
}

// GetAttachment streams an attachment
func (h *EmailHandler) GetAttachment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	// Route: /api/attachments/:id?messageId=...
	attachmentID := c.Param("id")
	messageID := c.Query("messageId")

	if messageID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "messageId query parameter is required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found",
		})
		return
	}

	data, err := h.gmailService.GetAttachment(ctx, user, messageID, attachmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "gmail_error",
			Message: "Failed to get attachment: " + err.Error(),
		})
		return
	}

	// Detect content type or default to octet-stream
	contentType := http.DetectContentType(data)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename=\"attachment\"") // Simplified filename
	c.Data(http.StatusOK, contentType, data)
}
