package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

const tavilyAPIURL = "https://api.tavily.com/search"

type tavilyClient struct {
	httpClient *http.Client
	apiKey     string
}

// NewTavilyClient creates a new Tavily search client.
func NewTavilyClient(apiKey string) domain.SearchClient {
	return &tavilyClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		apiKey: apiKey,
	}
}

type tavilyRequest struct {
	Query         string `json:"query"`
	IncludeAnswer string `json:"include_answer"` // "basic" untuk hemat token/kredit
	SearchDepth   string `json:"search_depth"`   // "basic" (1 kredit) vs "advanced" (2 kredit)
	MaxResults    int    `json:"max_results"`
}

type tavilyResultItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type tavilyAPIResponse struct {
	Query   string             `json:"query"`
	Answer  string             `json:"answer"`
	Results []tavilyResultItem `json:"results"`
}

// Search performs a web search using Tavily API.
func (c *tavilyClient) Search(ctx context.Context, query string, maxResults int) ([]domain.SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}

	payload, err := json.Marshal(tavilyRequest{
		Query:         query,
		IncludeAnswer: "basic", // include direct AI answer
		SearchDepth:   "basic", // hemat kredit (1 kredit per search)
		MaxResults:    maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("tavily marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tavily create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "my-assistant-gw/1.0 (personal assistant)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily status %d: %s", resp.StatusCode, string(body))
	}

	var tavilyResp tavilyAPIResponse
	if err := json.Unmarshal(body, &tavilyResp); err != nil {
		return nil, fmt.Errorf("tavily unmarshal response: %w", err)
	}

	var results []domain.SearchResult

	// 1. Simpan jawaban langsung dari Tavily (jika tersedia)
	if tavilyResp.Answer != "" {
		results = append(results, domain.SearchResult{
			Title:   "Tavily Answer",
			URL:     "",
			Snippet: tavilyResp.Answer,
		})
	}

	// 2. Simpan sumber referensi hasil pencarian
	for _, item := range tavilyResp.Results {
		if len(results) >= maxResults+1 {
			break
		}
		results = append(results, domain.SearchResult{
			Title:   item.Title,
			URL:     item.URL,
			Snippet: item.Content,
		})
	}

	return results, nil
}
