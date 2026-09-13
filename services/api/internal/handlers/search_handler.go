package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/npdms/api/internal/search"
)

// SearchHandler handles search-related HTTP requests
type SearchHandler struct {
	searchClient *search.OpenSearchClient
	indexer      *search.Indexer
}

// NewSearchHandler creates a new search handler
func NewSearchHandler() (*SearchHandler, error) {
	client, err := search.NewOpenSearchClient()
	if err != nil {
		return nil, err
	}

	indexer := search.NewIndexer(client)

	return &SearchHandler{
		searchClient: client,
		indexer:      indexer,
	}, nil
}

// SearchRequest represents an incoming search request
type SearchRequestBody struct {
	Query       string            `json:"query"`
	Index       string            `json:"index,omitempty"`
	Filters     map[string]string `json:"filters,omitempty"`
	Page        int               `json:"page,omitempty"`
	PageSize    int               `json:"page_size,omitempty"`
	SortBy      string            `json:"sort_by,omitempty"`
	SortOrder   string            `json:"sort_order,omitempty"`
	Highlight   bool              `json:"highlight,omitempty"`
	FuzzySearch bool              `json:"fuzzy_search,omitempty"`
}

// SearchResponseBody represents the search response
type SearchResponseBody struct {
	Success bool                 `json:"success"`
	Total   int64                `json:"total"`
	Page    int                  `json:"page"`
	Pages   int                  `json:"pages"`
	Results []search.SearchResult `json:"results"`
	Took    int64                `json:"took_ms"`
}

// Search handles global search requests
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqBody SearchRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if reqBody.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if reqBody.PageSize <= 0 {
		reqBody.PageSize = 20
	}
	if reqBody.Page <= 0 {
		reqBody.Page = 1
	}

	// Build search request
	searchReq := search.SearchRequest{
		Query:       reqBody.Query,
		Index:       reqBody.Index,
		Filters:     reqBody.Filters,
		From:        (reqBody.Page - 1) * reqBody.PageSize,
		Size:        reqBody.PageSize,
		SortBy:      reqBody.SortBy,
		SortOrder:   reqBody.SortOrder,
		Highlight:   reqBody.Highlight,
		FuzzySearch: reqBody.FuzzySearch,
	}

	// Execute search
	result, err := h.searchClient.Search(ctx, searchReq)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate pages
	pages := int(result.Total) / reqBody.PageSize
	if int(result.Total)%reqBody.PageSize > 0 {
		pages++
	}

	response := SearchResponseBody{
		Success: true,
		Total:   result.Total,
		Page:    reqBody.Page,
		Pages:   pages,
		Results: result.Hits,
		Took:    result.Took,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SearchFIRs handles FIR-specific search
func (h *SearchHandler) SearchFIRs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	searchReq := search.SearchRequest{
		Query:       query,
		Index:       search.IndexFIRs,
		From:        (page - 1) * pageSize,
		Size:        pageSize,
		Highlight:   true,
		FuzzySearch: true,
	}

	// Add status filter if provided
	if status := r.URL.Query().Get("status"); status != "" {
		searchReq.Filters = map[string]string{"status": status}
	}

	result, err := h.searchClient.Search(ctx, searchReq)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"total":   result.Total,
		"results": result.Hits,
		"took_ms": result.Took,
	})
}

// SearchCases handles Case-specific search
func (h *SearchHandler) SearchCases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	searchReq := search.SearchRequest{
		Query:       query,
		Index:       search.IndexCases,
		From:        (page - 1) * pageSize,
		Size:        pageSize,
		Highlight:   true,
		FuzzySearch: true,
	}

	result, err := h.searchClient.Search(ctx, searchReq)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"total":   result.Total,
		"results": result.Hits,
		"took_ms": result.Took,
	})
}

// SearchPersons handles Person search (suspects, witnesses, etc.)
func (h *SearchHandler) SearchPersons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	searchReq := search.SearchRequest{
		Query:       query,
		Index:       search.IndexPersons,
		From:        (page - 1) * pageSize,
		Size:        pageSize,
		Highlight:   true,
		FuzzySearch: true,
	}

	// Add role filter if provided
	if role := r.URL.Query().Get("role"); role != "" {
		searchReq.Filters = map[string]string{"role": role}
	}

	result, err := h.searchClient.Search(ctx, searchReq)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"total":   result.Total,
		"results": result.Hits,
		"took_ms": result.Took,
	})
}

// SearchVehicles handles Vehicle search
func (h *SearchHandler) SearchVehicles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	searchReq := search.SearchRequest{
		Query:       query,
		Index:       search.IndexVehicles,
		From:        (page - 1) * pageSize,
		Size:        pageSize,
		Highlight:   true,
		FuzzySearch: true,
	}

	result, err := h.searchClient.Search(ctx, searchReq)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"total":   result.Total,
		"results": result.Hits,
		"took_ms": result.Took,
	})
}

// Suggest provides search suggestions/autocomplete
func (h *SearchHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	index := r.URL.Query().Get("index")
	if index == "" {
		index = search.IndexFIRs // Default to FIRs
	}

	field := r.URL.Query().Get("field")
	if field == "" {
		field = "title.suggest" // Default suggest field
	}

	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 5
	}

	suggestions, err := h.searchClient.Suggest(ctx, index, field, query, size)
	if err != nil {
		http.Error(w, "Suggest failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"suggestions": suggestions,
	})
}

// IndexDocument manually indexes a document (for admin use)
func (h *SearchHandler) IndexDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var doc search.IndexDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if doc.Index == "" || doc.ID == "" || doc.Document == nil {
		http.Error(w, "Index, ID, and Document are required", http.StatusBadRequest)
		return
	}

	if err := h.searchClient.Index(ctx, doc); err != nil {
		http.Error(w, "Indexing failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Document indexed successfully",
	})
}

// DeleteDocument removes a document from the index
func (h *SearchHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	index := r.URL.Query().Get("index")
	id := r.URL.Query().Get("id")

	if index == "" || id == "" {
		http.Error(w, "Index and ID are required", http.StatusBadRequest)
		return
	}

	if err := h.searchClient.Delete(ctx, index, id); err != nil {
		http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Document deleted successfully",
	})
}

// InitializeIndices creates all required search indices
func (h *SearchHandler) InitializeIndices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.indexer.InitializeIndices(ctx); err != nil {
		http.Error(w, "Failed to initialize indices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Search indices initialized successfully",
	})
}

// Health checks OpenSearch connection status
func (h *SearchHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := h.searchClient.Ping(ctx)
	status := "healthy"
	httpStatus := http.StatusOK

	if err != nil {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": "opensearch",
		"status":  status,
		"error":   err,
	})
}

// GetIndexer returns the indexer for use by other handlers
func (h *SearchHandler) GetIndexer() *search.Indexer {
	return h.indexer
}
