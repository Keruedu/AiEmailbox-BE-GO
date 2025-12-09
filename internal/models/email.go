package models

import (
	"time"
)

// EmailStatus represents the workflow status for Kanban
type EmailStatus string

const (
	StatusInbox      EmailStatus = "inbox"
	StatusTodo       EmailStatus = "todo"
	StatusInProgress EmailStatus = "in_progress"
	StatusDone       EmailStatus = "done"
	StatusSnoozed    EmailStatus = "snoozed"
)

type Mailbox struct {
	ID          string `json:"id" bson:"id"`
	UserID      string `json:"userId" bson:"userId"`
	Name        string `json:"name" bson:"name"`
	Icon        string `json:"icon" bson:"icon"`
	UnreadCount int    `json:"unreadCount" bson:"unreadCount"`
	Type        string `json:"type" bson:"type"` // "system" or "custom"
	TotalCount  int    `json:"totalCount" bson:"totalCount"`
}

type Email struct {
	ID        string         `json:"id" bson:"_id,omitempty"` // Changed to string for Gmail ID
	ThreadID  string         `json:"threadId" bson:"threadId"`
	MailboxID string         `json:"mailboxId" bson:"mailboxId"`
	UserID    string         `json:"userId" bson:"userId"`
	From      EmailAddress   `json:"from" bson:"from"`
	To        []EmailAddress `json:"to" bson:"to"`
	Cc        []EmailAddress `json:"cc,omitempty" bson:"cc,omitempty"`
	Bcc       []EmailAddress `json:"bcc,omitempty" bson:"bcc,omitempty"`
	Subject   string         `json:"subject" bson:"subject"`
	Preview   string         `json:"preview" bson:"preview"`
	Body      string         `json:"body" bson:"body"`
	// Workflow fields for Kanban
	Status         EmailStatus  `json:"status" bson:"status"`
	SnoozedUntil   *time.Time   `json:"snoozedUntil,omitempty" bson:"snoozedUntil,omitempty"`
	Summary        string       `json:"summary,omitempty" bson:"summary,omitempty"`
	GmailURL       string       `json:"gmailUrl,omitempty" bson:"gmailUrl,omitempty"`
	IsRead         bool         `json:"isRead" bson:"isRead"`
	IsStarred      bool         `json:"isStarred" bson:"isStarred"`
	HasAttachments bool         `json:"hasAttachments" bson:"hasAttachments"`
	Attachments    []Attachment `json:"attachments,omitempty" bson:"attachments,omitempty"`
	Labels         []string     `json:"labels,omitempty" bson:"labels,omitempty"`
	ReceivedAt     time.Time    `json:"receivedAt" bson:"receivedAt"`
	CreatedAt      time.Time    `json:"createdAt" bson:"createdAt"`
}

type EmailAddress struct {
	Name  string `json:"name" bson:"name"`
	Email string `json:"email" bson:"email"`
}

type Attachment struct {
	ID       string `json:"id" bson:"id"`
	Filename string `json:"filename" bson:"filename"`
	Size     int64  `json:"size" bson:"size"`
	MimeType string `json:"mimeType" bson:"mimeType"`
	URL      string `json:"url" bson:"url"`
}

type EmailListResponse struct {
	Emails      []*Email `json:"emails"`
	Total       int      `json:"total"`
	Page        int      `json:"page"`
	PerPage     int      `json:"perPage"`
	HasNextPage bool     `json:"hasNextPage"`
}

type MailboxesResponse struct {
	Mailboxes []Mailbox `json:"mailboxes"`
}
