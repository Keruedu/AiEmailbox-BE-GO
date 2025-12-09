package repository

import (
	"aiemailbox-be/internal/models"
	"context"
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

// GetKanban returns emails grouped by status. Snoozed emails are excluded.
func (r *EmailRepository) GetKanban(ctx context.Context) (map[string][]models.Email, error) {
	// fetch all emails (include snoozed so frontend can render a Snoozed column)
	filter := bson.M{}
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
