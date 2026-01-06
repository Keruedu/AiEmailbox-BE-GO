package services

import (
	"aiemailbox-be/internal/repository"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SummaryService provides summary generation for emails.
type SummaryService interface {
	SummarizeText(ctx context.Context, text string) (string, error)
	SummarizeAndSave(ctx context.Context, emailID string) (string, error)
}

// LocalSummaryService implements SummaryService with a local extractor and optional OpenAI provider.
type LocalSummaryService struct {
	repo     *repository.EmailRepository
	apiKey   string
	provider string
	model    string
	client   *http.Client
}

// NewSummaryService creates a new summary service. If apiKey is empty, it runs purely local extractor.
func NewSummaryService(repo *repository.EmailRepository, apiKey, provider, model string) SummaryService {
	return &LocalSummaryService{
		repo:     repo,
		apiKey:   apiKey,
		provider: strings.ToLower(provider),
		model:    model,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SummarizeAndSave fetches an email by id, generates a summary and saves it to DB.
func (s *LocalSummaryService) SummarizeAndSave(ctx context.Context, emailID string) (string, error) {
	email, err := s.repo.GetByID(ctx, emailID)
	if err != nil {
		return "", err
	}
	// Use body if available, otherwise preview
	text := strings.TrimSpace(email.Body)
	if text == "" {
		text = strings.TrimSpace(email.Preview)
	}

	// Clean HTML
	text = stripHTML(text)

	summary, err := s.SummarizeText(ctx, text)
	if err != nil {
		return "", err
	}
	if err := s.repo.SetSummary(ctx, emailID, summary); err != nil {
		return "", err
	}
	return summary, nil
}

// SummarizeText returns a summary for given text. If an API key is present and provider is supported, it will call the provider.
func (s *LocalSummaryService) SummarizeText(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	// If API key present, attempt provider call
	if s.apiKey != "" {
		if s.provider == "gemini" {
			summ, err := s.callGemini(ctx, text)
			if err == nil && strings.TrimSpace(summ) != "" {
				return summ, nil
			}
			fmt.Printf("Gemini summary failed, falling back: %v\n", err)
		} else if s.provider == "" || s.provider == "openai" {
			summ, err := s.callOpenAI(ctx, text)
			if err == nil && strings.TrimSpace(summ) != "" {
				return summ, nil
			}
		}
	}

	// Local extractive summarizer (free)
	return extractiveSummary(text, 2, 300), nil
}

// callGemini calls Google Gemini API. Returns summary or error.
func (s *LocalSummaryService) callGemini(ctx context.Context, text string) (string, error) {
	// Use configured model or default to gemini-1.5-flash for speed and efficiency
	model := s.model
	if model == "" {
		model = "gemini-1.5-flash"
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, s.apiKey)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": "Summarize the following email in 2-3 concise sentences:\n\n" + text},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.2,
			"maxOutputTokens": 200,
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}

	if len(parsed.Candidates) > 0 && len(parsed.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text), nil
	}
	return "", errors.New("no content in Gemini response")
}

// callOpenAI calls OpenAI Chat Completions API (simple implementation). Returns summary or error.
func (s *LocalSummaryService) callOpenAI(ctx context.Context, text string) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	model := s.model
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []message{
			{Role: "system", Content: "You are a concise email summarizer. Return a short summary (2-3 sentences)."},
			{Role: "user", Content: "Summarize the following email:\n\n" + text},
		},
		"max_tokens":  200,
		"temperature": 0.2,
	}

	b, _ := json.Marshal(reqBody)

	// simple retry
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.apiKey)

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = errors.New(resp.Status)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			lastErr = err
			continue
		}
		if len(parsed.Choices) > 0 {
			return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
		}
		lastErr = errors.New("no choices in response")
	}
	return "", lastErr
}

// ===== Extractive summarizer (simple, free) =====

var sentenceSplitRE = regexp.MustCompile(`(?m)([^.!?\n]+[.!?]?)`)

func extractiveSummary(text string, topSentences int, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// split into sentences
	matches := sentenceSplitRE.FindAllString(text, -1)
	if len(matches) == 0 {
		// fallback: truncate
		if len(text) > maxChars {
			return text[:maxChars]
		}
		return text
	}

	// build frequency map
	freq := map[string]float64{}
	totalWords := 0
	wordRE := regexp.MustCompile(`[A-Za-z0-9']+`)
	for _, s := range matches {
		for _, w := range wordRE.FindAllString(strings.ToLower(s), -1) {
			if len(w) <= 2 {
				continue
			}
			freq[w] += 1
			totalWords++
		}
	}
	if totalWords == 0 {
		// fallback
		out := strings.Join(matches, " ")
		if len(out) > maxChars {
			return out[:maxChars]
		}
		return out
	}

	// score sentences
	type sscore struct {
		idx   int
		score float64
		text  string
	}
	var scores []sscore
	for i, s := range matches {
		sc := 0.0
		words := wordRE.FindAllString(strings.ToLower(s), -1)
		for _, w := range words {
			sc += freq[w]
		}
		if len(words) > 0 {
			sc = sc / float64(len(words))
		}
		scores = append(scores, sscore{idx: i, score: sc, text: strings.TrimSpace(s)})
	}

	// pick top sentences
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if topSentences > len(scores) {
		topSentences = len(scores)
	}
	chosen := scores[:topSentences]
	// restore original order
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].idx < chosen[j].idx })

	var parts []string
	outLen := 0
	for _, c := range chosen {
		if outLen+len(c.text) > maxChars && outLen > 0 {
			break
		}
		parts = append(parts, c.text)
		outLen += len(c.text)
	}
	result := strings.Join(parts, " ")
	if len(result) > maxChars {
		result = result[:maxChars]
	}
	return strings.TrimSpace(result)
}

// stripHTML removes HTML tags, scripts, and styles from text
func stripHTML(input string) string {
	original := input
	// Remove script tags and content
	reScript := regexp.MustCompile(`(?si)<script[^>]*>.*?</script>`)
	input = reScript.ReplaceAllString(input, " ")

	// Remove style tags and content
	reStyle := regexp.MustCompile(`(?si)<style[^>]*>.*?</style>`)
	input = reStyle.ReplaceAllString(input, " ")

	// Remove comments
	reComment := regexp.MustCompile(`(?s)<!--.*?-->`)
	input = reComment.ReplaceAllString(input, " ")

	// Remove HTML tags
	reTags := regexp.MustCompile(`<[^>]*>`)
	input = reTags.ReplaceAllString(input, " ")

	// Decode all HTML entities
	input = html.UnescapeString(input)

	// Remove zero-width characters and other invisible formatting
	reInvisible := regexp.MustCompile(`[\x{200B}-\x{200D}\x{FEFF}]`)
	input = reInvisible.ReplaceAllString(input, "")

	// Collapse whitespace
	reSpace := regexp.MustCompile(`\s+`)
	input = reSpace.ReplaceAllString(input, " ")

	result := strings.TrimSpace(input)

	// Debug logging
	if len(original) > 50 {
		fmt.Printf("[DEBUG] stripHTML input (start): %s...\n", original[:50])
	} else {
		fmt.Printf("[DEBUG] stripHTML input: %s\n", original)
	}
	if len(result) > 50 {
		fmt.Printf("[DEBUG] stripHTML output (start): %s...\n", result[:50])
	} else {
		fmt.Printf("[DEBUG] stripHTML output: %s\n", result)
	}

	return result
}
