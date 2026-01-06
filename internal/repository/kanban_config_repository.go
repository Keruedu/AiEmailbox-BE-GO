package repository

import (
	"aiemailbox-be/internal/models"
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// KanbanConfigRepository handles Kanban column configuration persistence
type KanbanConfigRepository struct {
	collection *mongo.Collection
}

// NewKanbanConfigRepository creates a new repository
func NewKanbanConfigRepository(db *mongo.Database) *KanbanConfigRepository {
	r := &KanbanConfigRepository{
		collection: db.Collection("kanban_columns"),
	}

	// Ensure indexes
	ctx := context.Background()
	idxView := r.collection.Indexes()
	_, _ = idxView.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}},
		Options: options.Index().SetName("idx_user_id"),
	})
	_, _ = idxView.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "order", Value: 1}},
		Options: options.Index().SetName("idx_user_order"),
	})

	return r
}

// GetColumns returns all columns for a user, ordered by 'order' field
func (r *KanbanConfigRepository) GetColumns(ctx context.Context, userID string) ([]models.KanbanColumn, error) {
	filter := bson.M{"userId": userID}
	findOptions := options.Find().SetSort(bson.D{{Key: "order", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var columns []models.KanbanColumn
	if err = cursor.All(ctx, &columns); err != nil {
		return nil, err
	}

	return columns, nil
}

// GetColumnByID returns a single column by ID
func (r *KanbanConfigRepository) GetColumnByID(ctx context.Context, columnID string) (*models.KanbanColumn, error) {
	filter := r.idFilter(columnID)
	var column models.KanbanColumn
	if err := r.collection.FindOne(ctx, filter).Decode(&column); err != nil {
		return nil, err
	}
	return &column, nil
}

// CreateColumn creates a new column
func (r *KanbanConfigRepository) CreateColumn(ctx context.Context, column *models.KanbanColumn) error {
	if column.ID == "" {
		column.ID = primitive.NewObjectID().Hex()
	}
	_, err := r.collection.InsertOne(ctx, column)
	return err
}

// UpdateColumn updates an existing column
func (r *KanbanConfigRepository) UpdateColumn(ctx context.Context, columnID string, updates bson.M) error {
	filter := r.idFilter(columnID)
	update := bson.M{"$set": updates}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

// DeleteColumn deletes a column
func (r *KanbanConfigRepository) DeleteColumn(ctx context.Context, columnID string) error {
	filter := r.idFilter(columnID)
	_, err := r.collection.DeleteOne(ctx, filter)
	return err
}

// GetMaxOrder returns the maximum order value for a user's columns
func (r *KanbanConfigRepository) GetMaxOrder(ctx context.Context, userID string) (int, error) {
	filter := bson.M{"userId": userID}
	findOptions := options.FindOne().SetSort(bson.D{{Key: "order", Value: -1}})

	var column models.KanbanColumn
	err := r.collection.FindOne(ctx, filter, findOptions).Decode(&column)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return -1, nil
		}
		return 0, err
	}
	return column.Order, nil
}

// ReorderColumns updates the order of multiple columns
func (r *KanbanConfigRepository) ReorderColumns(ctx context.Context, userID string, columnIDs []string) error {
	for i, id := range columnIDs {
		filter := bson.M{"userId": userID, "_id": id}
		update := bson.M{"$set": bson.M{"order": i}}
		_, err := r.collection.UpdateOne(ctx, filter, update)
		if err != nil {
			return err
		}
	}
	return nil
}

// InitDefaultColumns creates default columns for a new user
func (r *KanbanConfigRepository) InitDefaultColumns(ctx context.Context, userID string) error {
	// Check if user already has columns
	count, err := r.collection.CountDocuments(ctx, bson.M{"userId": userID})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Already has columns
	}

	// Default columns
	defaults := []models.KanbanColumn{
		{ID: primitive.NewObjectID().Hex(), UserID: userID, Key: "inbox", Label: "Inbox", Order: 0, GmailLabel: "INBOX", IsDefault: true},
		{ID: primitive.NewObjectID().Hex(), UserID: userID, Key: "todo", Label: "To Do", Order: 1, GmailLabel: "STARRED", IsDefault: true},
		{ID: primitive.NewObjectID().Hex(), UserID: userID, Key: "in_progress", Label: "In Progress", Order: 2, GmailLabel: "IMPORTANT", IsDefault: true},
		{ID: primitive.NewObjectID().Hex(), UserID: userID, Key: "done", Label: "Done", Order: 3, GmailLabel: "", IsDefault: true},
		{ID: primitive.NewObjectID().Hex(), UserID: userID, Key: "snoozed", Label: "Snoozed", Order: 4, GmailLabel: "", IsDefault: true},
	}

	docs := make([]interface{}, len(defaults))
	for i := range defaults {
		docs[i] = defaults[i]
	}

	_, err = r.collection.InsertMany(ctx, docs)
	return err
}

// GetColumnByKey returns a column by its key for a user
func (r *KanbanConfigRepository) GetColumnByKey(ctx context.Context, userID, key string) (*models.KanbanColumn, error) {
	filter := bson.M{"userId": userID, "key": key}
	var column models.KanbanColumn
	if err := r.collection.FindOne(ctx, filter).Decode(&column); err != nil {
		return nil, err
	}
	return &column, nil
}

// helper to build ID filter
func (r *KanbanConfigRepository) idFilter(columnID string) bson.M {
	if oid, err := primitive.ObjectIDFromHex(columnID); err == nil {
		return bson.M{"_id": oid}
	}
	return bson.M{"_id": columnID}
}

// generateKey creates a URL-safe key from label
func generateKey(label string) string {
	key := strings.ToLower(label)
	key = strings.ReplaceAll(key, " ", "_")
	// Remove non-alphanumeric characters except underscore
	result := ""
	for _, c := range key {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result += string(c)
		}
	}
	return fmt.Sprintf("custom_%s", result)
}
