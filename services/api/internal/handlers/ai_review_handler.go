package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AIReviewHandler struct {
	service *services.AIReviewService
}

func NewAIReviewHandler(service *services.AIReviewService) *AIReviewHandler {
	return &AIReviewHandler{service: service}
}

// GetReviewQueue returns pending AI decisions for review
func (h *AIReviewHandler) GetReviewQueue(c *gin.Context) {
	// Parse filters
	decisionType := c.Query("type")
	priority := c.Query("priority")
	assignedToStr := c.Query("assigned_to")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filters := services.QueueFilter{
		Page:     page,
		PageSize: pageSize,
	}

	if decisionType != "" {
		dt := models.AIDecisionType(decisionType)
		filters.Type = &dt
	}
	if priority != "" {
		p := models.AIDecisionPriority(priority)
		filters.Priority = &p
	}
	if assignedToStr != "" {
		if assignedTo, err := uuid.Parse(assignedToStr); err == nil {
			filters.AssignedTo = &assignedTo
		}
	}

	queue, err := h.service.GetReviewQueue(c.Request.Context(), filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "queue_fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"decisions": queue.Items,
		"pagination": gin.H{
			"page":        queue.Page,
			"page_size":   queue.PageSize,
			"total":       queue.Total,
			"total_pages": queue.TotalPages,
		},
	})
}

// GetDecision returns a specific AI decision
func (h *AIReviewHandler) GetDecision(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid decision ID format",
			Code:    400,
		})
		return
	}

	decision, err := h.service.GetDecision(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Decision not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, decision)
}

// ReviewDecision handles human review of an AI decision
func (h *AIReviewHandler) ReviewDecision(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid decision ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Status         string `json:"status" binding:"required"`
		HumanDecision  string `json:"humanDecision"`
		ReviewNotes    string `json:"reviewNotes"`
		OverrideReason string `json:"overrideReason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		string(models.AIDecisionStatusApproved):   true,
		string(models.AIDecisionStatusRejected):   true,
		string(models.AIDecisionStatusOverridden): true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_status",
			Message: "Status must be APPROVED, REJECTED, or OVERRIDDEN",
			Code:    400,
		})
		return
	}

	// Get reviewer ID from context
	reviewerID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	uid := reviewerID.(uuid.UUID)

	review := services.ReviewRequest{
		Status:         models.AIDecisionStatus(req.Status),
		HumanDecision:  req.HumanDecision,
		Notes:          req.ReviewNotes,
		OverrideReason: req.OverrideReason,
	}

	decision, err := h.service.ReviewDecision(c.Request.Context(), id, review, uid)
	if errors.Is(err, services.ErrAlreadyReviewed) {
		// Two officers can open the same queue. The second one is told what
		// happened rather than silently overwriting the first one's decision.
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "already_reviewed",
			Message: "This suggestion has already been reviewed by another officer.",
			Code:    409,
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "review_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, decision)
}

// AssignDecision assigns a decision to a reviewer
func (h *AIReviewHandler) AssignDecision(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid decision ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		ReviewerID string `json:"reviewerId" binding:"required"`
		DueBy      string `json:"dueBy"`
		Notes      string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	reviewerID, err := uuid.Parse(req.ReviewerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_reviewer_id",
			Message: "Invalid reviewer ID format",
			Code:    400,
		})
		return
	}

	// Get assigner ID from context
	assignerID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	uid := assignerID.(uuid.UUID)

	var dueBy *time.Time
	if req.DueBy != "" {
		t, err := time.Parse(time.RFC3339, req.DueBy)
		if err == nil {
			dueBy = &t
		}
	}

	err = h.service.AssignDecision(c.Request.Context(), id, reviewerID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "assignment_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"decisionId": id,
		"reviewerId": reviewerID,
		"assignedBy": uid,
		"dueBy":      dueBy,
		"message":    "Decision assigned",
	})
}

// SubmitFeedback submits feedback on an AI decision
func (h *AIReviewHandler) SubmitFeedback(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid decision ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		FeedbackType string `json:"feedbackType" binding:"required"`
		CorrectValue string `json:"correctValue"`
		Comments     string `json:"comments"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Validate feedback type
	validTypes := map[string]bool{
		"CORRECT":           true,
		"INCORRECT":         true,
		"PARTIALLY_CORRECT": true,
	}
	if !validTypes[req.FeedbackType] {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_feedback_type",
			Message: "Feedback type must be CORRECT, INCORRECT, or PARTIALLY_CORRECT",
			Code:    400,
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	uid := userID.(uuid.UUID)

	err = h.service.SubmitFeedback(c.Request.Context(), id, req.FeedbackType, req.CorrectValue, req.Comments, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "feedback_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"decisionId": id,
		"message":    "Feedback recorded",
	})
}

// GetModelConfigs returns all AI model configurations
func (h *AIReviewHandler) GetModelConfigs(c *gin.Context) {
	configs, err := h.service.GetAllModelConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configs": configs,
	})
}

// GetModelConfig returns a specific model configuration
func (h *AIReviewHandler) GetModelConfig(c *gin.Context) {
	modelName := c.Param("modelName")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_model_name",
			Message: "Model name is required",
			Code:    400,
		})
		return
	}

	config, err := h.service.GetModelConfig(c.Request.Context(), modelName)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Model configuration not found",
			Code:    404,
		})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateModelConfig updates a model configuration
func (h *AIReviewHandler) UpdateModelConfig(c *gin.Context) {
	modelName := c.Param("modelName")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_model_name",
			Message: "Model name is required",
			Code:    400,
		})
		return
	}

	var req services.AIModelConfigUpdate

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	if req.ConfidenceThreshold != nil && (*req.ConfidenceThreshold <= 0 || *req.ConfidenceThreshold > 1) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_threshold",
			Message: "The confidence threshold is a fraction between 0 and 1.",
			Code:    400,
		})
		return
	}

	updaterID, _ := c.Get("userID")
	updatedBy, _ := updaterID.(uuid.UUID)

	config, err := h.service.ApplyModelConfigUpdate(c.Request.Context(), modelName, req, updatedBy)
	if err != nil {
		// Switching a model on without a passing evaluation is a refusal, not
		// a server fault, and the officer should be told which rule stopped it.
		if isRuleRefusal(err) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "model_not_measured",
				Message: refusalMessage(err),
				Code:    409,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, config)
}

// RegisterModel adds a model to the registry, switched off.
func (h *AIReviewHandler) RegisterModel(c *gin.Context) {
	var req struct {
		ModelName           string  `json:"modelName" binding:"required"`
		ModelVersion        string  `json:"modelVersion" binding:"required"`
		DecisionType        string  `json:"decisionType" binding:"required"`
		Module              string  `json:"module"`
		Task                string  `json:"task"`
		EndpointEnv         string  `json:"endpointEnv"`
		Licence             string  `json:"licence"`
		SourceURL           string  `json:"sourceUrl"`
		ConfidenceThreshold float64 `json:"confidenceThreshold" binding:"required"`
		ReviewTimeout       int     `json:"reviewTimeout"`
		MaxQueueSize        int     `json:"maxQueueSize"`
		Description         string  `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	if req.ConfidenceThreshold <= 0 || req.ConfidenceThreshold > 1 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_threshold",
			Message: "The confidence threshold is a fraction between 0 and 1.",
			Code:    400,
		})
		return
	}

	actorID, _ := c.Get("userID")
	registeredBy, _ := actorID.(uuid.UUID)

	config, err := h.service.RegisterModel(c.Request.Context(), &models.AIModelConfig{
		ModelName:           req.ModelName,
		ModelVersion:        req.ModelVersion,
		DecisionType:        models.AIDecisionType(req.DecisionType),
		Module:              req.Module,
		Task:                req.Task,
		EndpointEnv:         req.EndpointEnv,
		Licence:             req.Licence,
		SourceURL:           req.SourceURL,
		ConfidenceThreshold: req.ConfidenceThreshold,
		ReviewTimeout:       req.ReviewTimeout,
		MaxQueueSize:        req.MaxQueueSize,
		Description:         req.Description,
	}, registeredBy)
	if err != nil {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "registration_failed",
			Message: refusalMessage(err),
			Code:    409,
		})
		return
	}

	c.JSON(http.StatusCreated, config)
}

// RecordEvaluation appends a measurement of one model version.
func (h *AIReviewHandler) RecordEvaluation(c *gin.Context) {
	var req struct {
		ModelName     string  `json:"modelName" binding:"required"`
		ModelVersion  string  `json:"modelVersion" binding:"required"`
		Dataset       string  `json:"dataset" binding:"required"`
		DatasetSize   int     `json:"datasetSize" binding:"required"`
		DatasetSHA256 string  `json:"datasetSha256"`
		Metric        string  `json:"metric" binding:"required"`
		Threshold     float64 `json:"threshold" binding:"required"`
		Measured      float64 `json:"measured"`
		Limitations   string  `json:"limitations"`
		Notes         string  `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	if req.Threshold <= 0 || req.Threshold > 1 || req.Measured < 0 || req.Measured > 1 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_measurement",
			Message: "The threshold and the measured value are fractions between 0 and 1.",
			Code:    400,
		})
		return
	}

	actorID, _ := c.Get("userID")
	runBy, _ := actorID.(uuid.UUID)

	evaluation, err := h.service.RecordEvaluation(c.Request.Context(), &models.AIModelEvaluation{
		ModelName:     req.ModelName,
		ModelVersion:  req.ModelVersion,
		Dataset:       req.Dataset,
		DatasetSize:   req.DatasetSize,
		DatasetSHA256: req.DatasetSHA256,
		Metric:        req.Metric,
		Threshold:     req.Threshold,
		Measured:      req.Measured,
		Limitations:   req.Limitations,
		Notes:         req.Notes,
	}, runBy)
	if err != nil {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "evaluation_failed",
			Message: refusalMessage(err),
			Code:    409,
		})
		return
	}

	c.JSON(http.StatusCreated, evaluation)
}

// ListEvaluations returns the measurements recorded for a model.
func (h *AIReviewHandler) ListEvaluations(c *gin.Context) {
	evaluations, err := h.service.ListEvaluations(c.Request.Context(), c.Query("model_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": evaluations})
}

// ListModuleSwitches reports every AI module and whether it is on.
func (h *AIReviewHandler) ListModuleSwitches(c *gin.Context) {
	switches, err := h.service.ModuleSwitches(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": switches})
}

// isRuleRefusal reports whether the database turned the write down because of
// a stated rule rather than failing.
func isRuleRefusal(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	for _, marker := range []string{
		"has not passed an evaluation",
		"cannot be enabled without a version",
		"is retired and cannot be enabled",
		"is not in the registry",
		"is switched off",
		"module",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// refusalMessage strips the SQLSTATE noise off a database refusal so the
// officer reads the rule, not the plumbing.
func refusalMessage(err error) string {
	text := err.Error()
	if i := strings.Index(text, "ERROR: "); i >= 0 {
		text = text[i+len("ERROR: "):]
	}
	if i := strings.Index(text, " (SQLSTATE"); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// GetStats returns AI decision statistics
func (h *AIReviewHandler) GetStats(c *gin.Context) {
	// Parse date range
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")

	var startDate, endDate *time.Time
	if startStr != "" {
		t, err := time.Parse("2006-01-02", startStr)
		if err == nil {
			startDate = &t
		}
	}
	if endStr != "" {
		t, err := time.Parse("2006-01-02", endStr)
		if err == nil {
			endDate = &t
		}
	}

	stats, err := h.service.GetStatsByDateRange(c.Request.Context(), startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "stats_fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetAcceptance reports what officers did with each model's suggestions,
// grouped by station, by language or by suggestion type. A model officers keep
// rejecting has to be visible before anyone can decide to switch it off.
func (h *AIReviewHandler) GetAcceptance(c *gin.Context) {
	modelName := c.Query("model_name")

	groupBy := c.DefaultQuery("group_by", "station")
	switch groupBy {
	case "station", "language", "type", "model":
	default:
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_group_by",
			Message: "Group by station, language, type or model.",
			Code:    400,
		})
		return
	}

	// Default to the last 30 days, and read the end date as the whole day.
	startDate := time.Now().AddDate(0, 0, -30)
	endDate := time.Now().AddDate(0, 0, 1)
	if t, err := time.Parse("2006-01-02", c.Query("start_date")); err == nil {
		startDate = t
	}
	if t, err := time.Parse("2006-01-02", c.Query("end_date")); err == nil {
		endDate = t.AddDate(0, 0, 1)
	}

	rows, err := h.service.Acceptance(c.Request.Context(), groupBy, modelName, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "metrics_fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    rows,
		"groupBy": groupBy,
		"start":   startDate.Format("2006-01-02"),
		"end":     endDate.AddDate(0, 0, -1).Format("2006-01-02"),
	})
}

// GetMyAssignments returns decisions assigned to the current user
func (h *AIReviewHandler) GetMyAssignments(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	uid := userID.(uuid.UUID)
	status := c.DefaultQuery("status", "PENDING")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filters := services.QueueFilter{
		AssignedTo: &uid,
		Page:       page,
		PageSize:   pageSize,
	}

	// Filter by status
	if status != "" {
		s := models.AIDecisionStatus(status)
		filters.Status = &s
	}

	queue, err := h.service.GetReviewQueue(c.Request.Context(), filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": queue.Items,
		"pagination": gin.H{
			"page":        queue.Page,
			"page_size":   queue.PageSize,
			"total":       queue.Total,
			"total_pages": queue.TotalPages,
		},
	})
}

// BulkReview handles bulk review of multiple decisions
func (h *AIReviewHandler) BulkReview(c *gin.Context) {
	var req struct {
		DecisionIDs []string `json:"decisionIds" binding:"required"`
		Status      string   `json:"status" binding:"required"`
		ReviewNotes string   `json:"reviewNotes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		string(models.AIDecisionStatusApproved): true,
		string(models.AIDecisionStatusRejected): true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_status",
			Message: "Bulk review status must be APPROVED or REJECTED",
			Code:    400,
		})
		return
	}

	// Get reviewer ID from context
	reviewerID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	uid := reviewerID.(uuid.UUID)

	results := make([]gin.H, 0, len(req.DecisionIDs))
	successCount := 0
	failCount := 0

	for _, idStr := range req.DecisionIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			results = append(results, gin.H{
				"id":      idStr,
				"success": false,
				"error":   "Invalid ID format",
			})
			failCount++
			continue
		}

		review := services.ReviewRequest{
			Status: models.AIDecisionStatus(req.Status),
			Notes:  req.ReviewNotes,
		}

		_, err = h.service.ReviewDecision(c.Request.Context(), id, review, uid)
		if err != nil {
			results = append(results, gin.H{
				"id":      idStr,
				"success": false,
				"error":   err.Error(),
			})
			failCount++
		} else {
			results = append(results, gin.H{
				"id":      idStr,
				"success": true,
			})
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results":      results,
		"successCount": successCount,
		"failCount":    failCount,
		"total":        len(req.DecisionIDs),
	})
}

// GetDecisionHistory returns the history of a decision
func (h *AIReviewHandler) GetDecisionHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid decision ID format",
			Code:    400,
		})
		return
	}

	history, err := h.service.GetDecisionHistory(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "history_fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"history": history,
	})
}

// ExpireDecisions manually triggers expiration of old decisions
func (h *AIReviewHandler) ExpireDecisions(c *gin.Context) {
	count, err := h.service.ExpireOldDecisions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "expiration_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"expiredCount": count,
		"message":      "Successfully expired old decisions",
	})
}
