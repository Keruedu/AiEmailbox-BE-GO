package handlers

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"
	"aiemailbox-be/internal/utils"
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sahilm/fuzzy"
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
	perPage, _ := strconv.Atoi(c.DefaultQuery("perPage", "50"))

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
			// Pre-process candidates for fuzzy search (Sanitize HTML once)

			var searchableItems []SearchableEmail
			for _, list := range kanbanMap {
				for i := range list {
					// Use pointer to avoid copying big structs
					e := &list[i]
					// Combine fields and sanitize
					rawText := e.Subject + " " + e.From.Name + " " + e.From.Email + " " + e.Body
					cleanText := utils.SanitizeHTML(rawText)
					searchableItems = append(searchableItems, SearchableEmail{
						Original:   e,
						SearchText: cleanText,
					})
				}
			}

			// Use sahilm/fuzzy for search
			src := &EmailSource{items: searchableItems}
			matches := fuzzy.FindFrom(query, src)

			for _, match := range matches {
				// Debug logging to help tune threshold
				// fmt.Printf("Query: %s, Match: %s, Score: %d\n", query, searchableItems[match.Index].SearchText, match.Score)

				// Threshold: Match score must be at least the query length.
				// This filters out very weak/scattered matches.
				if match.Score < len(query) {
					continue
				}

				if match.Index < len(searchableItems) {
					email := searchableItems[match.Index].Original
					emailMap[email.ID] = *email
				}
			}
		}
	}

	// Convert map to slice
	finalEmails := make([]*models.Email, 0, len(emailMap))
	for _, e := range emailMap {
		val := e // copy
		// Sanitize Preview and Summary for display
		val.Preview = utils.SanitizeHTML(val.Preview)
		val.Summary = utils.SanitizeHTML(val.Summary)
		// Clear body to reduce payload and force detail fetch (which ensures full content is loaded on click)
		val.Body = ""

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

	// Sort by ReceivedAt descending (newest first)
	sort.Slice(finalEmails, func(i, j int) bool {
		// Newest first means i > j (i is 'after' j)
		return finalEmails[i].ReceivedAt.After(finalEmails[j].ReceivedAt)
	})

	// Sort by ReceivedAt descending (newest first)
	sort.Slice(finalEmails, func(i, j int) bool {
		// Newest first means i > j (i is 'after' j)
		return finalEmails[i].ReceivedAt.After(finalEmails[j].ReceivedAt)
	})

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

// Helper for fuzzy search
type SearchableEmail struct {
	Original   *models.Email
	SearchText string
}

type EmailSource struct {
	items []SearchableEmail
}

func (s *EmailSource) String(i int) string {
	return s.items[i].SearchText
}

func (s *EmailSource) Len() int {
	return len(s.items)
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
		To       []string `json:"to"`
		Cc       []string `json:"cc,omitempty"`
		Bcc      []string `json:"bcc,omitempty"`
		Subject  string   `json:"subject"`
		Body     string   `json:"body"`
		ThreadID string   `json:"threadId,omitempty"`
	}

	// Check Content-Type to determine how to parse
	contentType := c.ContentType()
	var attachments []*models.Attachment
	
	if contentType == "multipart/form-data" || c.Request.MultipartForm != nil {
		// Parse multipart form
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_request",
				Message: "Failed to parse multipart form: " + err.Error(),
			})
			return
		}
		
		// Get JSON fields from form
		toJSON := c.PostForm("to")
		ccJSON := c.PostForm("cc")
		bccJSON := c.PostForm("bcc")
		req.Subject = c.PostForm("subject")
		req.Body = c.PostForm("body")
		req.ThreadID = c.PostForm("threadId")
		
		// Parse JSON arrays
		if toJSON != "" {
			if err := utils.ParseJSON(toJSON, &req.To); err != nil {
				c.JSON(http.StatusBadRequest, models.ErrorResponse{
					Error:   "invalid_request",
					Message: "Invalid 'to' field",
				})
				return
			}
		}
		if ccJSON != "" {
			if err := utils.ParseJSON(ccJSON, &req.Cc); err != nil {
				req.Cc = []string{}
			}
		}
		if bccJSON != "" {
			if err := utils.ParseJSON(bccJSON, &req.Bcc); err != nil {
				req.Bcc = []string{}
			}
		}
		
		// Get files
		form := c.Request.MultipartForm
		if form != nil && form.File != nil {
			files := form.File["attachments"]
			for _, fileHeader := range files {
				file, err := fileHeader.Open()
				if err != nil {
					continue
				}
				defer file.Close()
				
				// Read file content
				content := make([]byte, fileHeader.Size)
				_, err = file.Read(content)
				if err != nil {
					continue
				}
				
				attachments = append(attachments, &models.Attachment{
					Filename:    fileHeader.Filename,
					MimeType:    fileHeader.Header.Get("Content-Type"),
					Size:        fileHeader.Size,
					Data:        content,
				})
			}
		}
	} else {
		// Parse JSON body
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_request",
				Message: "Invalid request body",
			})
			return
		}
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

	// Convert string arrays to EmailAddress arrays
	toAddresses := make([]models.EmailAddress, len(req.To))
	for i, to := range req.To {
		toAddresses[i] = models.EmailAddress{Email: to}
	}
	
	ccAddresses := make([]models.EmailAddress, len(req.Cc))
	for i, cc := range req.Cc {
		ccAddresses[i] = models.EmailAddress{Email: cc}
	}
	
	bccAddresses := make([]models.EmailAddress, len(req.Bcc))
	for i, bcc := range req.Bcc {
		bccAddresses[i] = models.EmailAddress{Email: bcc}
	}

	email := &models.Email{
		To:          toAddresses,
		Cc:          ccAddresses,
		Bcc:         bccAddresses,
		Subject:     req.Subject,
		Body:        req.Body,
		ThreadID:    req.ThreadID,
		Attachments: attachments,
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
	// Use the same SendEmail logic
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
