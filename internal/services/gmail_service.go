package services

import (
	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/utils"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type GmailService struct {
	cfg *config.Config
}

func NewGmailService(cfg *config.Config) *GmailService {
	return &GmailService{
		cfg: cfg,
	}
}

func (s *GmailService) getOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.GoogleClientID,
		ClientSecret: s.cfg.GoogleClientSecret,
		RedirectURL:  s.cfg.FrontendURL, // Or backend callback if handled there
		Scopes: []string{
			gmail.GmailReadonlyScope,
			gmail.GmailModifyScope,
			gmail.GmailSendScope,
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

func (s *GmailService) GetClient(ctx context.Context, user *models.User) (*gmail.Service, error) {
	if user.GoogleRefreshToken == "" {
		return nil, errors.New("no google refresh token found")
	}

	config := s.getOAuthConfig()
	token := &oauth2.Token{
		AccessToken:  user.GoogleAccessToken,
		RefreshToken: user.GoogleRefreshToken,
		Expiry:       user.GoogleTokenExpiry,
		TokenType:    "Bearer",
	}

	tokenSource := config.TokenSource(ctx, token)

	// Create a new service using the token source
	srv, err := gmail.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, err
	}

	return srv, nil
}

func (s *GmailService) ListMailboxes(ctx context.Context, user *models.User) ([]models.Mailbox, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, err
	}

	labels, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}

	var mailboxes []models.Mailbox
	for _, label := range labels.Labels {
		// Filter out some system labels if needed, or map them to icons
		icon := "FolderOutlined"
		if label.Type == "system" {
			switch label.Id {
			case "INBOX":
				icon = "InboxOutlined"
			case "SENT":
				icon = "SendOutlined"
			case "TRASH":
				icon = "DeleteOutlined"
			case "DRAFT":
				icon = "FileOutlined"
			case "STARRED":
				icon = "StarOutlined"
			case "IMPORTANT":
				icon = "StarOutlined"
			}
		}

		mailboxes = append(mailboxes, models.Mailbox{
			ID:          label.Id,
			Name:        label.Name,
			Type:        strings.ToLower(label.Type),
			UnreadCount: int(label.MessagesUnread),
			TotalCount:  int(label.MessagesTotal),
			Icon:        icon,
		})
	}

	return mailboxes, nil
}

func (s *GmailService) ListEmails(ctx context.Context, user *models.User, mailboxID string, page int, perPage int) ([]*models.Email, int, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, 0, err
	}

	// Calculate max results (limit)
	// Gmail API uses page tokens, so "page" number is tricky.
	// For simplicity in this implementation, we might just fetch the first page or use tokens if we stored them.
	// A robust implementation would map page numbers to tokens.
	// Here we will just fetch the latest N messages.

	req := srv.Users.Messages.List("me").LabelIds(mailboxID).MaxResults(int64(perPage))
	// If page > 1, we would need to use PageToken.
	// For this exercise, let's assume simple pagination or just first page for now,
	// or implement a basic token mechanism if the frontend supports it.
	// Since the frontend sends page numbers, we can't easily map to tokens without state.
	// We will just return the first page for now.

	resp, err := req.Do()
	if err != nil {
		return nil, 0, err
	}

	var emails []*models.Email
	if len(resp.Messages) == 0 {
		return emails, 0, nil
	}

	// Fetch details for each message (batching would be better but keeping it simple)
	for _, msgHeader := range resp.Messages {
		msg, err := srv.Users.Messages.Get("me", msgHeader.Id).Format("full").Do()
		if err != nil {
			continue
		}

		email := s.mapGmailMessageToEmail(msg)
		emails = append(emails, &email)
	}

	return emails, int(resp.ResultSizeEstimate), nil
}

func (s *GmailService) GetEmail(ctx context.Context, user *models.User, emailID string) (*models.Email, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, err
	}

	msg, err := srv.Users.Messages.Get("me", emailID).Format("full").Do()
	if err != nil {
		return nil, err
	}

	email := s.mapGmailMessageToEmail(msg)
	return &email, nil
}

func (s *GmailService) mapGmailMessageToEmail(msg *gmail.Message) models.Email {
	var subject, from, to string
	// Initialize date with InternalDate (epoch ms) as a reliable fallback
	var date time.Time
	if msg.InternalDate > 0 {
		date = time.Unix(msg.InternalDate/1000, (msg.InternalDate%1000)*1000000)
	} else {
		date = time.Now()
	}

	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "Subject":
			subject = header.Value
		case "From":
			from = header.Value
		case "To":
			to = header.Value
		case "Date":
			// Parse date using net/mail
			d, err := mail.ParseDate(header.Value)
			if err == nil {
				date = d
			}
			// If parsing failed, we keep the InternalDate value which is better than zero or Now()
		}
	}

	// Extract body
	body := s.getBody(msg.Payload)

	// Check flags
	isRead := !contains(msg.LabelIds, "UNREAD")
	isStarred := contains(msg.LabelIds, "STARRED")

	// Extract attachments
	attachments := s.getAttachments(msg.Payload)
	hasAttachments := len(attachments) > 0

	return models.Email{
		ID:             msg.Id,
		ThreadID:       msg.ThreadId,
		Subject:        utils.ToValidUTF8(subject),
		Preview:        utils.ToValidUTF8(msg.Snippet),
		From:           parseAddress(utils.ToValidUTF8(from)),
		To:             parseAddresses(utils.ToValidUTF8(to)),
		Body:           utils.ToValidUTF8(body),
		ReceivedAt:     date,
		IsRead:         isRead,
		IsStarred:      isStarred,
		HasAttachments: hasAttachments,
		Attachments:    attachments,
		MailboxID:      "INBOX", // Default, or derive from labels
		Labels:         msg.LabelIds,
	}
}

func (s *GmailService) getAttachments(part *gmail.MessagePart) []*models.Attachment {
	var attachments []*models.Attachment
	if part == nil {
		return attachments
	}

	// Check if the current part is an attachment
	if part.Filename != "" && part.Body != nil && part.Body.AttachmentId != "" {
		attachments = append(attachments, &models.Attachment{
			ID:       part.Body.AttachmentId,
			Filename: part.Filename,
			MimeType: part.MimeType,
			Size:     part.Body.Size,
		})
	}

	// Recursively check sub-parts
	for _, p := range part.Parts {
		attachments = append(attachments, s.getAttachments(p)...)
	}

	return attachments
}

func (s *GmailService) getBody(part *gmail.MessagePart) string {
	// Helper to process plain text
	processPlainText := func(data string) string {
		// Convert newlines to <br> for HTML display
		return strings.ReplaceAll(data, "\n", "<br/>")
	}

	// Helper to decode base64url
	decode := func(data string) ([]byte, error) {
		// Try RawURLEncoding first (no padding)
		decoded, err := base64.RawURLEncoding.DecodeString(data)
		if err == nil {
			return decoded, nil
		}
		// Fallback to standard URLEncoding (with padding)
		return base64.URLEncoding.DecodeString(data)
	}

	if part.Body != nil && part.Body.Data != "" {
		data, err := decode(part.Body.Data)
		if err == nil {
			if part.MimeType == "text/plain" {
				return processPlainText(string(data))
			}
			return string(data)
		}
	}

	var htmlBody, plainBody string

	for _, p := range part.Parts {
		if p.MimeType == "text/html" {
			data, err := decode(p.Body.Data)
			if err == nil {
				htmlBody = string(data)
			}
		}
		if p.MimeType == "text/plain" {
			data, err := decode(p.Body.Data)
			if err == nil {
				plainBody = processPlainText(string(data))
			}
		}
		// Recursive check if we haven't found anything yet
		if len(p.Parts) > 0 {
			// This is a bit simplistic for recursion, but let's try to get something
			subBody := s.getBody(p)
			if subBody != "" {
				// If we found something in sub-parts, decide how to use it.
				// For now, if we don't have htmlBody, use it.
				if htmlBody == "" {
					htmlBody = subBody // Assume sub-part returned best effort
				}
			}
		}
	}

	if htmlBody != "" {
		return htmlBody
	}
	return plainBody
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func parseAddress(addr string) models.EmailAddress {
	// Simple parser: "Name <email>" or "email"
	if strings.Contains(addr, "<") {
		parts := strings.Split(addr, "<")
		name := strings.TrimSpace(parts[0])
		email := strings.TrimSuffix(parts[1], ">")
		return models.EmailAddress{Name: name, Email: email}
	}
	return models.EmailAddress{Name: "", Email: addr}
}

func parseAddresses(addrs string) []models.EmailAddress {
	var result []models.EmailAddress
	if addrs == "" {
		return result
	}
	// Split by comma
	parts := strings.Split(addrs, ",")
	for _, p := range parts {
		result = append(result, parseAddress(strings.TrimSpace(p)))
	}
	return result
}

func (s *GmailService) SendEmail(ctx context.Context, user *models.User, email *models.Email) error {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return err
	}

	var message gmail.Message

	// Build recipient headers
	toAddresses := make([]string, len(email.To))
	for i, to := range email.To {
		if to.Name != "" {
			toAddresses[i] = to.Name + " <" + to.Email + ">"
		} else {
			toAddresses[i] = to.Email
		}
	}
	
	var ccAddresses []string
	if len(email.Cc) > 0 {
		ccAddresses = make([]string, len(email.Cc))
		for i, cc := range email.Cc {
			if cc.Name != "" {
				ccAddresses[i] = cc.Name + " <" + cc.Email + ">"
			} else {
				ccAddresses[i] = cc.Email
			}
		}
	}
	
	var bccAddresses []string
	if len(email.Bcc) > 0 {
		bccAddresses = make([]string, len(email.Bcc))
		for i, bcc := range email.Bcc {
			if bcc.Name != "" {
				bccAddresses[i] = bcc.Name + " <" + bcc.Email + ">"
			} else {
				bccAddresses[i] = bcc.Email
			}
		}
	}
	
	// Add In-Reply-To and References headers for reply/forward
	if email.ThreadID != "" {
		message.ThreadId = email.ThreadID
	}

	var msgString strings.Builder
	
	// Write common headers
	msgString.WriteString("To: " + strings.Join(toAddresses, ", ") + "\r\n")
	if len(ccAddresses) > 0 {
		msgString.WriteString("Cc: " + strings.Join(ccAddresses, ", ") + "\r\n")
	}
	if len(bccAddresses) > 0 {
		msgString.WriteString("Bcc: " + strings.Join(bccAddresses, ", ") + "\r\n")
	}
	msgString.WriteString("Subject: " + email.Subject + "\r\n")
	msgString.WriteString("MIME-Version: 1.0\r\n")
	
	// Check if we have attachments
	if len(email.Attachments) > 0 {
		// Use multipart/mixed for email with attachments
		boundary := "----=_Part_" + fmt.Sprintf("%d", time.Now().UnixNano())
		msgString.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")
		
		// HTML body part
		msgString.WriteString("--" + boundary + "\r\n")
		msgString.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msgString.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		msgString.WriteString(base64.StdEncoding.EncodeToString([]byte(email.Body)))
		msgString.WriteString("\r\n")
		
		// Attachments
		for _, att := range email.Attachments {
			if att == nil || att.Data == nil {
				continue
			}
			msgString.WriteString("--" + boundary + "\r\n")
			
			// Determine MIME type
			mimeType := att.MimeType
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			
			msgString.WriteString("Content-Type: " + mimeType + "; name=\"" + att.Filename + "\"\r\n")
			msgString.WriteString("Content-Disposition: attachment; filename=\"" + att.Filename + "\"\r\n")
			msgString.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
			
			// Base64 encode the attachment
			msgString.WriteString(base64.StdEncoding.EncodeToString(att.Data))
			msgString.WriteString("\r\n")
		}
		
		// End boundary
		msgString.WriteString("--" + boundary + "--\r\n")
	} else {
		// Simple HTML email without attachments
		msgString.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msgString.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		msgString.WriteString(base64.StdEncoding.EncodeToString([]byte(email.Body)))
	}

	message.Raw = base64.URLEncoding.EncodeToString([]byte(msgString.String()))

	_, err = srv.Users.Messages.Send("me", &message).Do()
	return err
}

func (s *GmailService) ModifyEmail(ctx context.Context, user *models.User, emailID string, addLabels, removeLabels []string) error {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return err
	}

	req := &gmail.ModifyMessageRequest{
		AddLabelIds:    addLabels,
		RemoveLabelIds: removeLabels,
	}

	_, err = srv.Users.Messages.Modify("me", emailID, req).Do()
	return err
}

func (s *GmailService) GetAttachment(ctx context.Context, user *models.User, messageID, attachmentID string) ([]byte, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, err
	}

	attach, err := srv.Users.Messages.Attachments.Get("me", messageID, attachmentID).Do()
	if err != nil {
		return nil, err
	}

	data, err := base64.URLEncoding.DecodeString(attach.Data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *GmailService) SearchEmails(ctx context.Context, user *models.User, query string, pageToken string) ([]*models.Email, string, int, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, "", 0, err
	}

	// Search using 'q' parameter with limit
	req := srv.Users.Messages.List("me").Q(query).MaxResults(25)
	if pageToken != "" {
		req.PageToken(pageToken)
	}

	resp, err := req.Do()
	if err != nil {
		return nil, "", 0, err
	}

	if len(resp.Messages) == 0 {
		return []*models.Email{}, "", 0, nil
	}

	// Prepare exact slice size
	emails := make([]*models.Email, len(resp.Messages))

	// Use concurrency to fetch details
	// Limit concurrency to avoid rate limits
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)

	type result struct {
		index int
		email *models.Email
		err   error
	}

	resultsChan := make(chan result, len(resp.Messages))

	for i, msgHeader := range resp.Messages {
		sem <- struct{}{} // Acquire token
		go func(idx int, id string) {
			defer func() { <-sem }() // Release token

			msg, err := srv.Users.Messages.Get("me", id).Format("full").Do()
			if err != nil {
				resultsChan <- result{index: idx, err: err}
				return
			}

			email := s.mapGmailMessageToEmail(msg)

			// Generate contextual snippet
			if query != "" {
				// We need to work with runes to ensure we don't break multi-byte characters when slicing
				// This is especially important for Vietnamese or other UTF-8 content
				runeBody := []rune(email.Body)
				lowerBody := strings.ToLower(email.Body)
				lowerQuery := strings.ToLower(query)

				// Find match in bytes
				byteIdx := strings.Index(lowerBody, lowerQuery)

				// Fallback to fuzzy
				if byteIdx == -1 {
					cleanBody, _, _ := transform.String(t, lowerBody)
					cleanQuery, _, _ := transform.String(t, lowerQuery)
					if cleanIdx := strings.Index(cleanBody, cleanQuery); cleanIdx != -1 {
						byteIdx = cleanIdx
						if byteIdx >= len(lowerBody) {
							byteIdx = 0
						}
					}
				}

				if byteIdx != -1 {
					if byteIdx > len(email.Body) {
						byteIdx = len(email.Body)
					}

					safePrefix := email.Body[:byteIdx]
					runeIdx := utf8.RuneCountInString(safePrefix)
					queryLenRunes := utf8.RuneCountInString(query)

					const contextLen = 60
					start := runeIdx - contextLen
					if start < 0 {
						start = 0
					}

					end := runeIdx + queryLenRunes + contextLen
					if end > len(runeBody) {
						end = len(runeBody)
					}

					if start > end {
						start = end
					}

					snippet := string(runeBody[start:end])

					if start > 0 {
						snippet = "..." + snippet
					}
					if end < len(runeBody) {
						snippet = snippet + "..."
					}
					email.Preview = snippet
				}
			}

			resultsChan <- result{index: idx, email: &email}
		}(i, msgHeader.Id)
	}

	// Collect results
	for i := 0; i < len(resp.Messages); i++ {
		res := <-resultsChan
		if res.err == nil && res.email != nil {
			emails[res.index] = res.email
		}
	}

	// Filter out nils (errors)
	validEmails := []*models.Email{}
	for _, e := range emails {
		if e != nil {
			validEmails = append(validEmails, e)
		}
	}

	return validEmails, resp.NextPageToken, int(resp.ResultSizeEstimate), nil
}

// Transformer chain to remove accents
var t = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// ======== Week 4: Label Management ========

// GetLabels returns all Gmail labels for a user
func (s *GmailService) GetLabels(ctx context.Context, userID string) ([]models.GmailLabel, error) {
	// Note: This method needs a User object to get the Gmail client
	// For now, we return common Gmail labels as a fallback
	// In production, you would get the user from repository

	// Return common Gmail labels
	labels := []models.GmailLabel{
		{ID: "INBOX", Name: "Inbox", Type: "system"},
		{ID: "STARRED", Name: "Starred", Type: "system"},
		{ID: "IMPORTANT", Name: "Important", Type: "system"},
		{ID: "SENT", Name: "Sent", Type: "system"},
		{ID: "DRAFT", Name: "Draft", Type: "system"},
		{ID: "TRASH", Name: "Trash", Type: "system"},
		{ID: "SPAM", Name: "Spam", Type: "system"},
		{ID: "UNREAD", Name: "Unread", Type: "system"},
		{ID: "CATEGORY_PERSONAL", Name: "Personal", Type: "category"},
		{ID: "CATEGORY_SOCIAL", Name: "Social", Type: "category"},
		{ID: "CATEGORY_PROMOTIONS", Name: "Promotions", Type: "category"},
		{ID: "CATEGORY_UPDATES", Name: "Updates", Type: "category"},
		{ID: "CATEGORY_FORUMS", Name: "Forums", Type: "category"},
	}

	return labels, nil
}

// GetLabelsWithUser returns Gmail labels for a specific user (full API call)
func (s *GmailService) GetLabelsWithUser(ctx context.Context, user *models.User) ([]models.GmailLabel, error) {
	srv, err := s.GetClient(ctx, user)
	if err != nil {
		return nil, err
	}

	resp, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}

	var labels []models.GmailLabel
	for _, l := range resp.Labels {
		labels = append(labels, models.GmailLabel{
			ID:   l.Id,
			Name: l.Name,
			Type: strings.ToLower(l.Type),
		})
	}

	return labels, nil
}
