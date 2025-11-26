package repository

import (
	"aiemailbox-be/internal/models"
	"context"

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
	return &EmailRepository{
		emailCollection:   db.Collection("emails"),
		mailboxCollection: db.Collection("mailboxes"),
	}
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
