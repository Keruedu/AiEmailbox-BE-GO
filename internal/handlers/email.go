package handlers

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
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
