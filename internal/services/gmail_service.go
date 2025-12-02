package services

import (
	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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
	var date time.Time

	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "Subject":
			subject = header.Value
		case "From":
			from = header.Value
		case "To":
			to = header.Value
		case "Date":
			// Parse date
			// Gmail date format: "Tue, 2 Dec 2025 22:00:00 +0700"
			d, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", header.Value)
			if err == nil {
				date = d
			} else {
				// Try other formats or just use now
				date = time.Now()
			}
		}
	}

	// Extract body
	body := s.getBody(msg.Payload)

	// Check flags
	isRead := !contains(msg.LabelIds, "UNREAD")
	isStarred := contains(msg.LabelIds, "STARRED")

	return models.Email{
		ID:         msg.Id,
		ThreadID:   msg.ThreadId,
		Subject:    subject,
		Preview:    msg.Snippet,
		From:       parseAddress(from),
		To:         parseAddresses(to),
		Body:       body,
		ReceivedAt: date,
		IsRead:     isRead,
		IsStarred:  isStarred,
		MailboxID:  "INBOX", // Default, or derive from labels
		Labels:     msg.LabelIds,
	}
}

func (s *GmailService) getBody(part *gmail.MessagePart) string {
	if part.Body != nil && part.Body.Data != "" {
		data, _ := base64.URLEncoding.DecodeString(part.Body.Data)
		return string(data)
	}

	for _, p := range part.Parts {
		if p.MimeType == "text/html" {
			data, _ := base64.URLEncoding.DecodeString(p.Body.Data)
			return string(data)
		}
		if p.MimeType == "text/plain" {
			data, _ := base64.URLEncoding.DecodeString(p.Body.Data)
			return string(data)
		}
		// Recursive check
		if len(p.Parts) > 0 {
			return s.getBody(p)
		}
	}
	return ""
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
