package repository

import (
	"aiemailbox-be/internal/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type StatisticsRepository struct {
	emailCollection *mongo.Collection
}

func NewStatisticsRepository(db *mongo.Database) *StatisticsRepository {
	return &StatisticsRepository{
		emailCollection: db.Collection("emails"),
	}
}

// GetEmailsByStatus aggregates email count by workflow status
func (r *StatisticsRepository) GetEmailsByStatus(ctx context.Context, userID string) ([]models.EmailStatusStats, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			"userId":    userID,
			"labels":    bson.M{"$ne": "TRASH"},
			"mailboxId": bson.M{"$ne": "TRASH"},
		}},
		{"$group": bson.M{
			"_id":   "$status",
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"count": -1}},
	}

	cursor, err := r.emailCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.EmailStatusStats
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	// Map empty status to "inbox"
	for i := range results {
		if results[i].Status == "" {
			results[i].Status = "inbox"
		}
	}

	return results, nil
}

// GetEmailTrend aggregates emails by date for the last N days
func (r *StatisticsRepository) GetEmailTrend(ctx context.Context, userID string, days int) ([]models.EmailTrendPoint, error) {
	startDate := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{"$match": bson.M{
			"userId":     userID,
			"receivedAt": bson.M{"$gte": startDate},
			"labels":     bson.M{"$ne": "TRASH"},
			"mailboxId":  bson.M{"$ne": "TRASH"},
		}},
		{"$group": bson.M{
			"_id": bson.M{
				"$dateToString": bson.M{
					"format": "%Y-%m-%d",
					"date":   "$receivedAt",
				},
			},
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"_id": 1}},
	}

	cursor, err := r.emailCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.EmailTrendPoint
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetTopSenders aggregates top N email senders
func (r *StatisticsRepository) GetTopSenders(ctx context.Context, userID string, limit int) ([]models.TopSender, error) {
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
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"count": -1}},
		{"$limit": limit},
		{"$project": bson.M{
			"name":  "$_id.name",
			"email": "$_id.email",
			"count": 1,
			"_id":   0,
		}},
	}

	cursor, err := r.emailCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.TopSender
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetDailyActivity aggregates email activity by day of week and hour
func (r *StatisticsRepository) GetDailyActivity(ctx context.Context, userID string, days int) ([]models.DailyActivity, error) {
	startDate := time.Now().AddDate(0, 0, -days)

	pipeline := []bson.M{
		{"$match": bson.M{
			"userId":     userID,
			"receivedAt": bson.M{"$gte": startDate},
			"labels":     bson.M{"$ne": "TRASH"},
			"mailboxId":  bson.M{"$ne": "TRASH"},
		}},
		{"$group": bson.M{
			"_id": bson.M{
				"dayOfWeek": bson.M{"$dayOfWeek": "$receivedAt"}, // 1=Sunday in MongoDB
				"hour":      bson.M{"$hour": "$receivedAt"},
			},
			"count": bson.M{"$sum": 1},
		}},
		{"$project": bson.M{
			"dayOfWeek": bson.M{"$subtract": []interface{}{"$_id.dayOfWeek", 1}}, // Convert to 0=Sunday
			"hour":      "$_id.hour",
			"count":     1,
			"_id":       0,
		}},
		{"$sort": bson.D{
			{Key: "dayOfWeek", Value: 1},
			{Key: "hour", Value: 1},
		}},
	}

	cursor, err := r.emailCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.DailyActivity
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetTotalAndUnread returns total email count and unread count
func (r *StatisticsRepository) GetTotalAndUnread(ctx context.Context, userID string) (total int, unread int, starred int, err error) {
	baseFilter := bson.M{
		"userId":    userID,
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}

	// Total count
	totalCount, err := r.emailCollection.CountDocuments(ctx, baseFilter)
	if err != nil {
		return 0, 0, 0, err
	}

	// Unread count
	unreadFilter := bson.M{
		"userId":    userID,
		"isRead":    false,
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}
	unreadCount, err := r.emailCollection.CountDocuments(ctx, unreadFilter)
	if err != nil {
		return 0, 0, 0, err
	}

	// Starred count
	starredFilter := bson.M{
		"userId":    userID,
		"isStarred": true,
		"labels":    bson.M{"$ne": "TRASH"},
		"mailboxId": bson.M{"$ne": "TRASH"},
	}
	starredCount, err := r.emailCollection.CountDocuments(ctx, starredFilter)
	if err != nil {
		return 0, 0, 0, err
	}

	return int(totalCount), int(unreadCount), int(starredCount), nil
}
