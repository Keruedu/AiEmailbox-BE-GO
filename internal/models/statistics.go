package models

// EmailStatusStats - count of emails by workflow status
type EmailStatusStats struct {
	Status string `json:"status" bson:"_id"`
	Count  int    `json:"count" bson:"count"`
}

// EmailTrendPoint - count of emails received on a specific date
type EmailTrendPoint struct {
	Date  string `json:"date" bson:"_id"`  // YYYY-MM-DD format
	Count int    `json:"count" bson:"count"`
}

// TopSender - represents a top email sender with count
type TopSender struct {
	Name  string `json:"name" bson:"name"`
	Email string `json:"email" bson:"email"`
	Count int    `json:"count" bson:"count"`
}

// DailyActivity - email activity by day of week and hour
type DailyActivity struct {
	DayOfWeek int `json:"dayOfWeek" bson:"dayOfWeek"` // 0=Sunday, 6=Saturday
	Hour      int `json:"hour" bson:"hour"`           // 0-23
	Count     int `json:"count" bson:"count"`
}

// StatisticsResponse - complete statistics response for the dashboard
type StatisticsResponse struct {
	StatusStats   []EmailStatusStats `json:"statusStats"`
	EmailTrend    []EmailTrendPoint  `json:"emailTrend"`
	TopSenders    []TopSender        `json:"topSenders"`
	DailyActivity []DailyActivity    `json:"dailyActivity"`
	TotalEmails   int                `json:"totalEmails"`
	UnreadCount   int                `json:"unreadCount"`
	StarredCount  int                `json:"starredCount"`
	Period        string             `json:"period"` // "7d", "30d", "90d"
}
