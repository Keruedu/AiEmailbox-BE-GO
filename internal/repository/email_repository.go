package repository

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/utils"
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EmailRepository struct {
	emailCollection   *mongo.Collection
	mailboxCollection *mongo.Collection
}

func NewEmailRepository(db *mongo.Database) *EmailRepository {
	r := &EmailRepository{
		emailCollection:   db.Collection("emails"),
		mailboxCollection: db.Collection("mailboxes"),
	}

	// Ensure indexes for faster Kanban queries
	ctx := context.Background()
	idxView := r.emailCollection.Indexes()
	// index on status
	_, _ = idxView.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "status", Value: 1}},
		Options: options.Index().SetName("idx_status"),
	})
	// index on snoozedUntil
	_, _ = idxView.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "snoozedUntil", Value: 1}},
		Options: options.Index().SetName("idx_snoozed_until"),
	})

	return r
}

// helper to build ID filter that supports either ObjectID hex or string IDs
func idFilter(emailID string) bson.M {
	if oid, err := primitive.ObjectIDFromHex(emailID); err == nil {
		return bson.M{"_id": oid}
	}
	return bson.M{"_id": emailID}
}

// GetKanban returns emails grouped by status for a specific user. Snoozed emails are excluded.
func (r *EmailRepository) GetKanban(ctx context.Context, userID string) (map[string][]models.Email, error) {
	// fetch all emails (include snoozed so frontend can render a Snoozed column)
	// fetch all emails (include snoozed so frontend can render a Snoozed column)
	filter := bson.M{
		"userId":    userID,
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "receivedAt", Value: -1}})

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[string][]models.Email)
	for cursor.Next(ctx) {
		var e models.Email
		if err := cursor.Decode(&e); err != nil {
			return nil, err
		}
		key := string(e.Status)
		if key == "" {
			key = string(models.StatusInbox)
		}
		result[key] = append(result[key], e)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// SearchEmails searches for emails matching the query string in subject, sender, or summary.
func (r *EmailRepository) SearchEmails(ctx context.Context, userID string, query string) ([]models.Email, error) {
	// Fuzzy search using regex with case insensitivity
	// We search in: subject, from.name, from.email, summary, body
	// Use relaxed regex for accent insensitivity
	pattern := utils.GenerateRelaxedRegex(query)
	regex := bson.M{"$regex": pattern, "$options": "i"}
	filter := bson.M{
		"userId": userID,
		"$or": []bson.M{
			{"subject": regex},
			{"from.name": regex},
			{"from.email": regex},
			{"summary": regex},
			{"body": regex},
		},
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "receivedAt", Value: -1}})
	findOptions.SetLimit(50) // Limit results for performance

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var emails []models.Email
	if err = cursor.All(ctx, &emails); err != nil {
		return nil, err
	}

	return emails, nil
}

// UpdateStatus updates the workflow status for an email
func (r *EmailRepository) UpdateStatus(ctx context.Context, emailID string, status string) error {
	filter := idFilter(emailID)
	update := bson.M{"$set": bson.M{"status": status}}
	// if moving out of snoozed, clear snoozedUntil
	if status != string(models.StatusSnoozed) {
		update = bson.M{"$set": bson.M{"status": status}, "$unset": bson.M{"snoozedUntil": ""}}
	}
	_, err := r.emailCollection.UpdateOne(ctx, filter, update)
	return err
}

// SetSnooze sets an email to snoozed with a snoozedUntil time
func (r *EmailRepository) SetSnooze(ctx context.Context, emailID string, until time.Time) error {
	filter := idFilter(emailID)
	update := bson.M{"$set": bson.M{"status": string(models.StatusSnoozed), "snoozedUntil": until}}
	_, err := r.emailCollection.UpdateOne(ctx, filter, update)
	return err
}

// SetSummary stores a generated summary for an email
func (r *EmailRepository) SetSummary(ctx context.Context, emailID string, summary string) error {
	filter := idFilter(emailID)
	update := bson.M{"$set": bson.M{"summary": summary}}
	_, err := r.emailCollection.UpdateOne(ctx, filter, update)
	return err
}

// GetByID returns an email by its ID (supports string IDs and ObjectID hex)
func (r *EmailRepository) GetByID(ctx context.Context, emailID string) (*models.Email, error) {
	filter := idFilter(emailID)
	var email models.Email
	if err := r.emailCollection.FindOne(ctx, filter).Decode(&email); err != nil {
		return nil, err
	}
	return &email, nil
}

// ListSnoozedDue returns snoozed emails that are due (snoozedUntil <= now)
func (r *EmailRepository) ListSnoozedDue(ctx context.Context, now time.Time) ([]models.Email, error) {
	filter := bson.M{"status": string(models.StatusSnoozed), "snoozedUntil": bson.M{"$lte": now}}
	cursor, err := r.emailCollection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var emails []models.Email
	for cursor.Next(ctx) {
		var e models.Email
		if err := cursor.Decode(&e); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return emails, nil
}
func (r *EmailRepository) GetMailboxes(ctx context.Context, userID string) ([]*models.Mailbox, error) {
	cursor, err := r.mailboxCollection.Find(ctx, bson.M{"userId": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var mailboxes []*models.Mailbox
	if err = cursor.All(ctx, &mailboxes); err != nil {
		return nil, err
	}

	return mailboxes, nil
}

func (r *EmailRepository) GetEmails(ctx context.Context, mailboxID string, page, perPage int) ([]*models.Email, int, error) {
	skip := (page - 1) * perPage

	findOptions := options.Find()
	findOptions.SetSkip(int64(skip))
	findOptions.SetLimit(int64(perPage))
	findOptions.SetSort(bson.D{{Key: "receivedAt", Value: -1}})

	filter := bson.M{"mailboxId": mailboxID}

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var emails []*models.Email
	if err = cursor.All(ctx, &emails); err != nil {
		return nil, 0, err
	}

	total, err := r.emailCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return emails, int(total), nil
}

func (r *EmailRepository) GetEmailByID(ctx context.Context, emailID string) (*models.Email, error) {
	oid, err := primitive.ObjectIDFromHex(emailID)
	if err != nil {
		return nil, err
	}

	var email models.Email
	err = r.emailCollection.FindOne(ctx, bson.M{"_id": oid}).Decode(&email)
	if err != nil {
		return nil, err
	}

	return &email, nil
}

func (r *EmailRepository) CreateEmail(ctx context.Context, email *models.Email) error {
	_, err := r.emailCollection.InsertOne(ctx, email)
	return err
}

func (r *EmailRepository) CreateMailbox(ctx context.Context, mailbox *models.Mailbox) error {
	_, err := r.mailboxCollection.InsertOne(ctx, mailbox)
	return err
}

func (r *EmailRepository) GetMailboxByID(ctx context.Context, mailboxID string) (*models.Mailbox, error) {
	var mailbox models.Mailbox
	err := r.mailboxCollection.FindOne(ctx, bson.M{"id": mailboxID}).Decode(&mailbox)
	if err != nil {
		return nil, err
	}
	return &mailbox, nil
}

func (r *EmailRepository) UpdateMailboxUnreadCount(ctx context.Context, mailboxID string, count int) error {
	update := bson.M{
		"$set": bson.M{
			"unreadCount": count,
		},
	}
	_, err := r.mailboxCollection.UpdateOne(ctx, bson.M{"id": mailboxID}, update)
	return err
}

// UpsertEmail updates an existing email or inserts a new one
func (r *EmailRepository) UpsertEmail(ctx context.Context, email *models.Email) error {
	filter := bson.M{"_id": email.ID} // email.ID is now string from Gmail ID
	update := bson.M{"$set": email}
	opts := options.Update().SetUpsert(true)
	_, err := r.emailCollection.UpdateOne(ctx, filter, update, opts)
	return err
}

// ======== Week 4: Semantic Search Methods ========

// SetEmbedding stores the vector embedding for an email
func (r *EmailRepository) SetEmbedding(ctx context.Context, emailID string, embedding []float32) error {
	filter := idFilter(emailID)
	update := bson.M{"$set": bson.M{"embedding": embedding}}
	_, err := r.emailCollection.UpdateOne(ctx, filter, update)
	return err
}

// GetAllWithEmbeddings returns all emails for a user that have embeddings stored
func (r *EmailRepository) GetAllWithEmbeddings(ctx context.Context, userID string) ([]models.Email, error) {
	filter := bson.M{
		"userId":    userID,
		"embedding": bson.M{"$exists": true, "$ne": nil},
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "receivedAt", Value: -1}})

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var emails []models.Email
	if err = cursor.All(ctx, &emails); err != nil {
		return nil, err
	}

	return emails, nil
}

// GetEmailsWithoutEmbedding returns emails that don't have embeddings yet
func (r *EmailRepository) GetEmailsWithoutEmbedding(ctx context.Context, userID string, limit int) ([]models.Email, error) {
	filter := bson.M{
		"userId": userID,
		"$or": []bson.M{
			{"embedding": bson.M{"$exists": false}},
			{"embedding": nil},
			{"embedding": bson.M{"$size": 0}},
		},
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}

	findOptions := options.Find()
	findOptions.SetLimit(int64(limit))
	findOptions.SetSort(bson.D{{Key: "receivedAt", Value: -1}})

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var emails []models.Email
	if err = cursor.All(ctx, &emails); err != nil {
		return nil, err
	}

	return emails, nil
}

// GetUniqueSenders returns unique sender names/emails for a user (for auto-suggestions)
func (r *EmailRepository) GetUniqueSenders(ctx context.Context, userID string, query string, limit int) ([]string, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			"userId":    userID,
			"labels":    bson.M{"$ne": "TRASH"},
			"mailboxId": bson.M{"$ne": "TRASH"},
		}},
		{"$group": bson.M{
			"_id": bson.M{
				"name":  "$from.name",
				"email": "$from.email",
			},
		}},
		{"$limit": 100},
	}

	cursor, err := r.emailCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []string
	queryLower := strings.ToLower(query)
	for cursor.Next(ctx) {
		var doc struct {
			ID struct {
				Name  string `bson:"name"`
				Email string `bson:"email"`
			} `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		// Filter by query prefix
		nameLower := strings.ToLower(doc.ID.Name)
		emailLower := strings.ToLower(doc.ID.Email)
		if strings.Contains(nameLower, queryLower) || strings.Contains(emailLower, queryLower) {
			if doc.ID.Name != "" {
				results = append(results, doc.ID.Name)
			} else {
				results = append(results, doc.ID.Email)
			}
		}

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetSubjectKeywords returns unique subject words for a user (for auto-suggestions)
func (r *EmailRepository) GetSubjectKeywords(ctx context.Context, userID string, query string, limit int) ([]string, error) {
	filter := bson.M{
		"userId":    userID,
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}

	findOptions := options.Find()
	findOptions.SetLimit(200)
	findOptions.SetProjection(bson.M{"subject": 1})

	cursor, err := r.emailCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	wordMap := make(map[string]bool)
	queryLower := strings.ToLower(query)

	for cursor.Next(ctx) {
		var doc struct {
			Subject string `bson:"subject"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		// Extract words from subject
		words := strings.Fields(doc.Subject)
		for _, w := range words {
			w = strings.Trim(w, ".,!?:;\"'()[]{}|")
			if len(w) < 3 {
				continue
			}
			wLower := strings.ToLower(w)
			if strings.HasPrefix(wLower, queryLower) {
				wordMap[w] = true
			}
		}
	}

	var results []string
	for w := range wordMap {
		results = append(results, w)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}
