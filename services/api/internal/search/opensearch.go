package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// OpenSearchClient handles connections to OpenSearch
type OpenSearchClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// SearchResult represents a search result
type SearchResult struct {
	ID         string                 `json:"id"`
	Index      string                 `json:"index"`
	Score      float64                `json:"score"`
	Source     map[string]interface{} `json:"source"`
	Highlights map[string][]string    `json:"highlights,omitempty"`
}

// SearchResponse represents the search response
type SearchResponse struct {
	Total    int64          `json:"total"`
	MaxScore float64        `json:"max_score"`
	Hits     []SearchResult `json:"hits"`
	Took     int64          `json:"took"`
}

// SearchRequest represents a search query
type SearchRequest struct {
	Query       string            `json:"query"`
	Index       string            `json:"index,omitempty"`       // Optional: specific index to search
	Filters     map[string]string `json:"filters,omitempty"`     // Optional: field filters
	From        int               `json:"from,omitempty"`        // Pagination offset
	Size        int               `json:"size,omitempty"`        // Page size
	SortBy      string            `json:"sort_by,omitempty"`     // Sort field
	SortOrder   string            `json:"sort_order,omitempty"`  // asc or desc
	Highlight   bool              `json:"highlight,omitempty"`   // Enable highlighting
	FuzzySearch bool              `json:"fuzzy_search,omitempty"` // Enable fuzzy matching
}

// IndexDocument represents a document to be indexed
type IndexDocument struct {
	Index      string                 `json:"index"`
	ID         string                 `json:"id"`
	Document   map[string]interface{} `json:"document"`
	RefreshNow bool                   `json:"refresh_now,omitempty"`
}

// NewOpenSearchClient creates a new OpenSearch client
func NewOpenSearchClient() (*OpenSearchClient, error) {
	baseURL := os.Getenv("OPENSEARCH_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9200"
	}

	username := os.Getenv("OPENSEARCH_USERNAME")
	password := os.Getenv("OPENSEARCH_PASSWORD")

	client := &OpenSearchClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Verify connection
	if err := client.Ping(context.Background()); err != nil {
		log.Printf("Warning: OpenSearch connection failed: %v", err)
		// Return client anyway for graceful degradation
	}

	return client, nil
}

// Ping checks if OpenSearch is reachable
func (c *OpenSearchClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL, nil)
	if err != nil {
		return err
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opensearch returned status %d", resp.StatusCode)
	}

	return nil
}

// Search performs a search query
func (c *OpenSearchClient) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	// Build the query
	query := c.buildQuery(req)

	// Determine index
	index := "_all"
	if req.Index != "" {
		index = req.Index
	}

	url := fmt.Sprintf("%s/%s/_search", c.baseURL, index)

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var osResp map[string]interface{}
	if err := json.Unmarshal(respBody, &osResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return c.parseSearchResponse(osResp)
}

// buildQuery constructs an OpenSearch query
func (c *OpenSearchClient) buildQuery(req SearchRequest) map[string]interface{} {
	query := make(map[string]interface{})

	// Build the main query
	var mainQuery map[string]interface{}

	if req.FuzzySearch {
		// Use multi_match with fuzziness
		mainQuery = map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     req.Query,
				"fields":    []string{"*"},
				"fuzziness": "AUTO",
				"operator":  "or",
			},
		}
	} else {
		// Use query_string for more flexible search
		mainQuery = map[string]interface{}{
			"query_string": map[string]interface{}{
				"query":            req.Query,
				"default_operator": "OR",
				"allow_leading_wildcard": true,
			},
		}
	}

	// Apply filters if provided
	if len(req.Filters) > 0 {
		filters := make([]map[string]interface{}, 0, len(req.Filters))
		for field, value := range req.Filters {
			filters = append(filters, map[string]interface{}{
				"term": map[string]interface{}{
					field: value,
				},
			})
		}

		query["query"] = map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   mainQuery,
				"filter": filters,
			},
		}
	} else {
		query["query"] = mainQuery
	}

	// Pagination
	if req.From > 0 {
		query["from"] = req.From
	}
	if req.Size > 0 {
		query["size"] = req.Size
	} else {
		query["size"] = 20 // Default page size
	}

	// Sorting
	if req.SortBy != "" {
		order := "desc"
		if req.SortOrder == "asc" {
			order = "asc"
		}
		query["sort"] = []map[string]interface{}{
			{req.SortBy: map[string]interface{}{"order": order}},
		}
	}

	// Highlighting
	if req.Highlight {
		query["highlight"] = map[string]interface{}{
			"fields": map[string]interface{}{
				"*": map[string]interface{}{},
			},
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
		}
	}

	return query
}

// parseSearchResponse converts OpenSearch response to our format
func (c *OpenSearchClient) parseSearchResponse(osResp map[string]interface{}) (*SearchResponse, error) {
	response := &SearchResponse{}

	// Extract timing
	if took, ok := osResp["took"].(float64); ok {
		response.Took = int64(took)
	}

	// Extract hits
	hits, ok := osResp["hits"].(map[string]interface{})
	if !ok {
		return response, nil
	}

	// Total hits
	if total, ok := hits["total"].(map[string]interface{}); ok {
		if value, ok := total["value"].(float64); ok {
			response.Total = int64(value)
		}
	}

	// Max score
	if maxScore, ok := hits["max_score"].(float64); ok {
		response.MaxScore = maxScore
	}

	// Individual hits
	hitList, ok := hits["hits"].([]interface{})
	if !ok {
		return response, nil
	}

	response.Hits = make([]SearchResult, 0, len(hitList))
	for _, hit := range hitList {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		result := SearchResult{}

		if id, ok := hitMap["_id"].(string); ok {
			result.ID = id
		}
		if index, ok := hitMap["_index"].(string); ok {
			result.Index = index
		}
		if score, ok := hitMap["_score"].(float64); ok {
			result.Score = score
		}
		if source, ok := hitMap["_source"].(map[string]interface{}); ok {
			result.Source = source
		}

		// Extract highlights
		if highlight, ok := hitMap["highlight"].(map[string]interface{}); ok {
			result.Highlights = make(map[string][]string)
			for field, snippets := range highlight {
				if snippetList, ok := snippets.([]interface{}); ok {
					result.Highlights[field] = make([]string, len(snippetList))
					for i, s := range snippetList {
						if str, ok := s.(string); ok {
							result.Highlights[field][i] = str
						}
					}
				}
			}
		}

		response.Hits = append(response.Hits, result)
	}

	return response, nil
}

// Index adds or updates a document in the index
func (c *OpenSearchClient) Index(ctx context.Context, doc IndexDocument) error {
	url := fmt.Sprintf("%s/%s/_doc/%s", c.baseURL, doc.Index, doc.ID)
	if doc.RefreshNow {
		url += "?refresh=true"
	}

	body, err := json.Marshal(doc.Document)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("index request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Delete removes a document from the index
func (c *OpenSearchClient) Delete(ctx context.Context, index, id string) error {
	url := fmt.Sprintf("%s/%s/_doc/%s", c.baseURL, index, id)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed with status %d", resp.StatusCode)
	}

	return nil
}

// CreateIndex creates a new index with mappings
func (c *OpenSearchClient) CreateIndex(ctx context.Context, index string, mappings map[string]interface{}) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, index)

	body, err := json.Marshal(map[string]interface{}{
		"settings": map[string]interface{}{
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"indian_analyzer": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "standard",
						"filter":    []string{"lowercase", "asciifolding"},
					},
				},
			},
		},
		"mappings": mappings,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal mappings: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create index request failed: %w", err)
	}
	defer resp.Body.Close()

	// Index already exists is not an error
	if resp.StatusCode == http.StatusBadRequest {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Suggest provides search suggestions/autocomplete
func (c *OpenSearchClient) Suggest(ctx context.Context, index, field, text string, size int) ([]string, error) {
	query := map[string]interface{}{
		"suggest": map[string]interface{}{
			"suggestions": map[string]interface{}{
				"prefix": text,
				"completion": map[string]interface{}{
					"field": field,
					"size":  size,
					"fuzzy": map[string]interface{}{
						"fuzziness": 2,
					},
				},
			},
		},
	}

	url := fmt.Sprintf("%s/%s/_search", c.baseURL, index)

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal suggest query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("suggest request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var osResp map[string]interface{}
	if err := json.Unmarshal(respBody, &osResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	suggestions := []string{}
	if suggest, ok := osResp["suggest"].(map[string]interface{}); ok {
		if suggestionList, ok := suggest["suggestions"].([]interface{}); ok {
			for _, s := range suggestionList {
				if sMap, ok := s.(map[string]interface{}); ok {
					if options, ok := sMap["options"].([]interface{}); ok {
						for _, opt := range options {
							if optMap, ok := opt.(map[string]interface{}); ok {
								if text, ok := optMap["text"].(string); ok {
									suggestions = append(suggestions, text)
								}
							}
						}
					}
				}
			}
		}
	}

	return suggestions, nil
}

// BulkIndex indexes multiple documents at once
func (c *OpenSearchClient) BulkIndex(ctx context.Context, docs []IndexDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		// Action line
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": doc.Index,
				"_id":    doc.ID,
			},
		}
		actionLine, _ := json.Marshal(action)
		buf.Write(actionLine)
		buf.WriteByte('\n')

		// Document line
		docLine, _ := json.Marshal(doc.Document)
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	url := fmt.Sprintf("%s/_bulk", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-ndjson")
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bulk index request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bulk index failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
