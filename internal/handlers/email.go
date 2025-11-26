package handlers

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type EmailHandler struct {
	emailRepo *repository.EmailRepository
}

func NewEmailHandler(emailRepo *repository.EmailRepository) *EmailHandler {
	return &EmailHandler{
		emailRepo: emailRepo,
	}
}

// GetMailboxes returns all mailboxes for the authenticated user
func (h *EmailHandler) GetMailboxes(c *gin.Context) {
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

	mailboxes, err := h.emailRepo.GetMailboxes(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to load mailboxes",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"mailboxes": mailboxes,
	})
}

// GetEmails returns emails for a specific mailbox with pagination
func (h *EmailHandler) GetEmails(c *gin.Context) {
	_, exists := c.Get("userID")
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	emails, total, err := h.emailRepo.GetEmails(ctx, mailboxID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to load emails",
		})
		return
	}

	c.JSON(http.StatusOK, models.EmailListResponse{
		Emails:      emails,
		Total:       total,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page*perPage < total,
	})
}

// GetEmailDetail returns detailed information about a specific email
func (h *EmailHandler) GetEmailDetail(c *gin.Context) {
	_, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
		return
	}

	emailID := c.Param("emailId")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	email, err := h.emailRepo.GetEmailByID(ctx, emailID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "email_not_found",
				Message: "Email not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to load email",
		})
		return
	}

	c.JSON(http.StatusOK, email)
}
