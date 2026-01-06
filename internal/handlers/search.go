package handlers

import (
	"net/http"
	"sort"
	"strings"

	"aiemailbox-be/config"
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"aiemailbox-be/internal/services"

	"github.com/gin-gonic/gin"
)

// SearchHandler handles semantic search and suggestions
type SearchHandler struct {
	repo      *repository.EmailRepository
	embedding services.EmbeddingService
	cfg       *config.Config
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(repo *repository.EmailRepository, embedding services.EmbeddingService, cfg *config.Config) *SearchHandler {
	return &SearchHandler{
		repo:      repo,
		embedding: embedding,
		cfg:       cfg,
	}
}

// ========== Request/Response Types ==========

// SemanticSearchRequest is the payload for semantic search
type SemanticSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

// SearchResult represents a single search result with score
type SearchResult struct {
	Email *models.Email `json:"email"`
	Score float32       `json:"score"`
}

// SemanticSearchResponse is the response for semantic search
type SemanticSearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Total   int            `json:"total"`
}

// Suggestion represents a single search suggestion
type Suggestion struct {
	Text string `json:"text"`
	Type string `json:"type"` // "sender" | "keyword" | "subject"
}

// SuggestionsResponse is the response for search suggestions
type SuggestionsResponse struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// GenerateEmbeddingsRequest is the payload for generating embeddings
type GenerateEmbeddingsRequest struct {
	Limit int `json:"limit"` // Max emails to process (default 50)
}

// ========== Handlers ==========

// SemanticSearch godoc
// @Summary Semantic search for emails
// @Description Search emails using vector similarity (conceptual relevance)
// @Tags search
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param payload body SemanticSearchRequest true "Search query"
// @Success 200 {object} SemanticSearchResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /search/semantic [post]
func (h *SearchHandler) SemanticSearch(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req SemanticSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query cannot be empty"})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	ctx := c.Request.Context()

	// Generate embedding for query
	queryEmbedding, err := h.embedding.GenerateEmbedding(ctx, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate query embedding: " + err.Error()})
		return
	}

	// Get all emails with embeddings for this user
	emails, err := h.repo.GetAllWithEmbeddings(ctx, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch emails: " + err.Error()})
		return
	}

	// Calculate similarity scores
	type scoredEmail struct {
		email *models.Email
		score float32
	}

	var scored []scoredEmail
	for i := range emails {
		if len(emails[i].Embedding) > 0 {
			score := services.CosineSimilarity(queryEmbedding, emails[i].Embedding)
			scored = append(scored, scoredEmail{email: &emails[i], score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Take top results
	if len(scored) > limit {
		scored = scored[:limit]
	}

	// Build response
	results := make([]SearchResult, len(scored))
	for i, s := range scored {
		results[i] = SearchResult{
			Email: s.email,
			Score: s.score,
		}
	}

	c.JSON(http.StatusOK, SemanticSearchResponse{
		Results: results,
		Query:   req.Query,
		Total:   len(results),
	})
}

// GetSuggestions godoc
// @Summary Get search suggestions
// @Description Get auto-complete suggestions based on senders and keywords
// @Tags search
// @Security ApiKeyAuth
// @Produce json
// @Param q query string true "Search query prefix"
// @Success 200 {object} SuggestionsResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /search/suggestions [get]
func (h *SearchHandler) GetSuggestions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, SuggestionsResponse{Suggestions: []Suggestion{}})
		return
	}

	ctx := c.Request.Context()

	var suggestions []Suggestion

	// Get sender suggestions (limit 3)
	senders, err := h.repo.GetUniqueSenders(ctx, userID.(string), query, 3)
	if err == nil {
		for _, s := range senders {
			suggestions = append(suggestions, Suggestion{Text: s, Type: "sender"})
		}
	}

	// Get keyword suggestions (limit 2)
	keywords, err := h.repo.GetSubjectKeywords(ctx, userID.(string), query, 2)
	if err == nil {
		for _, k := range keywords {
			suggestions = append(suggestions, Suggestion{Text: k, Type: "keyword"})
		}
	}

	// Limit total to 5
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	c.JSON(http.StatusOK, SuggestionsResponse{Suggestions: suggestions})
}

// GenerateEmbeddings godoc
// @Summary Generate embeddings for emails
// @Description Batch generate embeddings for emails that don't have them yet
// @Tags search
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param payload body GenerateEmbeddingsRequest true "Request"
// @Success 200 {object} map[string]int
// @Failure 500 {object} models.ErrorResponse
// @Router /search/generate-embeddings [post]
func (h *SearchHandler) GenerateEmbeddings(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req GenerateEmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Use defaults
		req.Limit = 50
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	ctx := c.Request.Context()

	// Get emails without embeddings
	emails, err := h.repo.GetEmailsWithoutEmbedding(ctx, userID.(string), req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch emails: " + err.Error()})
		return
	}

	if len(emails) == 0 {
		c.JSON(http.StatusOK, gin.H{"processed": 0, "message": "All emails already have embeddings"})
		return
	}

	// Generate embeddings for each email
	processed := 0
	failed := 0
	for _, email := range emails {
		// Combine subject and body for embedding
		text := email.Subject + " " + email.Body
		if text == "" {
			text = email.Preview
		}

		embedding, err := h.embedding.GenerateEmbedding(ctx, text)
		if err != nil {
			failed++
			continue
		}

		if err := h.repo.SetEmbedding(ctx, email.ID, embedding); err != nil {
			failed++
			continue
		}

		processed++
	}

	c.JSON(http.StatusOK, gin.H{
		"processed": processed,
		"failed":    failed,
		"remaining": len(emails) - processed - failed,
	})
}
