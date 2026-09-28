package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

// duckDuckGoClient implements domain.SearchClient using the DuckDuckGo Instant Answer API.
type duckDuckGoClient struct {
	httpClient *http.Client
}

// NewDuckDuckGoClient creates a new DuckDuckGo search client (no API key required).
func NewDuckDuckGoClient() domain.SearchClient {
	return &duckDuckGoClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ddgAPIResponse is the partial response structure from DuckDuckGo's JSON API.
type ddgAPIResponse struct {
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	AbstractSource string `json:"AbstractSource"`
	Answer         string `json:"Answer"`
	Definition     string `json:"Definition"`
	DefinitionURL  string `json:"DefinitionURL"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
}

// Search performs a web search using DuckDuckGo's Instant Answer API.
func (c *duckDuckGoClient) Search(ctx context.Context, query string, maxResults int) ([]domain.SearchResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.duckduckgo.com/?q=%s&format=json&no_redirect=1&no_html=1&skip_disambig=1",
		url.QueryEscape(query),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo create request: %w", err)
	}
	req.Header.Set("User-Agent", "my-assistant-gw/1.0 (personal assistant)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo read body: %w", err)
	}

	var ddgResp ddgAPIResponse
	if err := json.Unmarshal(body, &ddgResp); err != nil {
		return nil, fmt.Errorf("duckduckgo unmarshal response: %w", err)
	}

	var results []domain.SearchResult

	// Add abstract if available
	if ddgResp.AbstractText != "" {
		results = append(results, domain.SearchResult{
			Title:   ddgResp.AbstractSource,
			URL:     ddgResp.AbstractURL,
			Snippet: ddgResp.AbstractText,
		})
	}

	// Add instant answer
	if ddgResp.Answer != "" {
		results = append(results, domain.SearchResult{
			Title:   "Instant Answer",
			URL:     "",
			Snippet: ddgResp.Answer,
		})
	}

	// Add definition
	if ddgResp.Definition != "" {
		results = append(results, domain.SearchResult{
			Title:   "Definition",
			URL:     ddgResp.DefinitionURL,
			Snippet: ddgResp.Definition,
		})
	}

	// Add related topics
	for _, topic := range ddgResp.RelatedTopics {
		if len(results) >= maxResults {
			break
		}
		if topic.Text != "" {
			results = append(results, domain.SearchResult{
				Title:   extractTitle(topic.Text),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
	}

	return results, nil
}

// extractTitle extracts a short title from the topic text (first sentence or first 60 chars).
func extractTitle(text string) string {
	for i, ch := range text {
		if ch == '.' || ch == '-' {
			if i > 3 {
				t := text[:i]
				if len(t) > 60 {
					return t[:60] + "..."
				}
				return t
			}
		}
	}
	if len(text) > 60 {
		return text[:60] + "..."
	}
	return text
}
