package models

// KanbanColumn represents a custom Kanban column for a user
type KanbanColumn struct {
	ID         string `json:"id" bson:"_id,omitempty"`
	UserID     string `json:"userId" bson:"userId"`
	Key        string `json:"key" bson:"key"`               // internal key (e.g., "inbox", "todo", "custom_1")
	Label      string `json:"label" bson:"label"`           // display name
	Order      int    `json:"order" bson:"order"`           // column order
	GmailLabel string `json:"gmailLabel" bson:"gmailLabel"` // mapped Gmail label (e.g., "STARRED", "IMPORTANT")
	Color      string `json:"color,omitempty" bson:"color,omitempty"`
	IsDefault  bool   `json:"isDefault" bson:"isDefault"` // true for system columns
}

// KanbanConfig represents the complete Kanban configuration for a user
type KanbanConfig struct {
	UserID  string         `json:"userId" bson:"userId"`
	Columns []KanbanColumn `json:"columns" bson:"columns"`
}

// CreateColumnRequest is the request payload for creating a new column
type CreateColumnRequest struct {
	Label      string `json:"label" binding:"required"`
	GmailLabel string `json:"gmailLabel"`
	Color      string `json:"color"`
}

// UpdateColumnRequest is the request payload for updating a column
type UpdateColumnRequest struct {
	Label      string `json:"label"`
	GmailLabel string `json:"gmailLabel"`
	Color      string `json:"color"`
	Order      *int   `json:"order"`
}

// ReorderColumnsRequest is the request for reordering columns
type ReorderColumnsRequest struct {
	ColumnIDs []string `json:"columnIds" binding:"required"`
}

// GmailLabel represents a Gmail label
type GmailLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "system" or "user"
}
