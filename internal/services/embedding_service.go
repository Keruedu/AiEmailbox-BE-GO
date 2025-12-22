package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aiemailbox-be/config"
)

// EmbeddingService defines the interface for generating embeddings
type EmbeddingService interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	BatchGenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	GetDimension() int
}

// OpenAIEmbeddingService implements EmbeddingService using OpenAI API
type OpenAIEmbeddingService struct {
	apiKey    string
	model     string
	client    *http.Client
	dimension int
}

// GeminiEmbeddingService implements EmbeddingService using Gemini API
type GeminiEmbeddingService struct {
	apiKey    string
	model     string
	client    *http.Client
	dimension int
}

// NewEmbeddingService creates an embedding service based on provider config
func NewEmbeddingService(cfg *config.Config) EmbeddingService {
	provider := strings.ToLower(cfg.EmbeddingProvider)

	switch provider {
	case "gemini":
		return &GeminiEmbeddingService{
			apiKey:    cfg.EmbeddingAPIKey,
			model:     getGeminiModel(cfg.EmbeddingModel),
			client:    &http.Client{Timeout: 30 * time.Second},
			dimension: 768, // Gemini embedding-001 dimension
		}
	case "openai":
		fallthrough
	default:
		return &OpenAIEmbeddingService{
			apiKey:    cfg.EmbeddingAPIKey,
			model:     cfg.EmbeddingModel,
			client:    &http.Client{Timeout: 30 * time.Second},
			dimension: 1536, // text-embedding-ada-002 dimension
		}
	}
}

func getGeminiModel(model string) string {
	// Use text-embedding-004 as default (newer and better)
	if model == "" || model == "text-embedding-ada-002" {
		return "text-embedding-004"
	}
	// Also accept embedding-001 for backwards compatibility
	if model == "embedding-001" {
		return "embedding-001"
	}
	return model
}

// GetDimension returns the embedding dimension
func (s *OpenAIEmbeddingService) GetDimension() int {
	return s.dimension
}

// GenerateEmbedding generates embedding for a single text using OpenAI
func (s *OpenAIEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.apiKey == "" {
		return nil, errors.New("OpenAI API key not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty text for embedding")
	}

	// Truncate to max tokens (roughly 8000 chars for ada-002)
	if len(text) > 8000 {
		text = text[:8000]
	}

	reqBody := map[string]interface{}{
		"model": s.model,
		"input": text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, errors.New("no embedding data in response")
	}

	return result.Data[0].Embedding, nil
}

// BatchGenerateEmbeddings generates embeddings for multiple texts
func (s *OpenAIEmbeddingService) BatchGenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if s.apiKey == "" {
		return nil, errors.New("OpenAI API key not configured")
	}

	if len(texts) == 0 {
		return nil, errors.New("no texts provided")
	}

	// Clean and truncate texts
	cleanTexts := make([]string, 0, len(texts))
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if len(t) > 8000 {
			t = t[:8000]
		}
		cleanTexts = append(cleanTexts, t)
	}

	if len(cleanTexts) == 0 {
		return nil, errors.New("no valid texts provided")
	}

	reqBody := map[string]interface{}{
		"model": s.model,
		"input": cleanTexts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Sort by index and extract embeddings
	embeddings := make([][]float32, len(cleanTexts))
	for _, d := range result.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

// ======== Gemini Embedding Service ========

// GetDimension returns the embedding dimension
func (s *GeminiEmbeddingService) GetDimension() int {
	return s.dimension
}

// GenerateEmbedding generates embedding using Gemini API
func (s *GeminiEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.apiKey == "" {
		return nil, errors.New("Gemini API key not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty text for embedding")
	}

	// Truncate if needed
	if len(text) > 10000 {
		text = text[:10000]
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:embedContent?key=%s", s.model, s.apiKey)

	reqBody := map[string]interface{}{
		"content": map[string]interface{}{
			"parts": []map[string]string{
				{"text": text},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Embedding.Values, nil
}

// BatchGenerateEmbeddings generates embeddings for multiple texts (Gemini uses single requests)
func (s *GeminiEmbeddingService) BatchGenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		emb, err := s.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	return embeddings, nil
}

// ======== Cosine Similarity for Vector Search ========

// CosineSimilarity computes cosine similarity between two vectors
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt32(normA) * sqrt32(normB))
}

func sqrt32(x float32) float32 {
	return float32(sqrtNewton(float64(x)))
}

func sqrtNewton(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
