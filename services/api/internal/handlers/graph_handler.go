package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

type GraphHandler struct {
	service *services.GraphService
}

func NewGraphHandler(service *services.GraphService) *GraphHandler {
	return &GraphHandler{service: service}
}

// Node Operations
func (h *GraphHandler) CreateNode(c *gin.Context) {
	var node models.GraphNode
	if err := c.ShouldBindJSON(&node); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	node.CreatedBy = &uid

	if err := h.service.CreateNode(c.Request.Context(), &node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create node", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, node)
}

func (h *GraphHandler) GetNode(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	node, err := h.service.GetNode(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	c.JSON(http.StatusOK, node)
}

func (h *GraphHandler) SearchNodes(c *gin.Context) {
	query := models.GraphQuery{
		Keyword: c.Query("keyword"),
		Limit:   100,
	}

	if nodeType := c.Query("nodeType"); nodeType != "" {
		nt := models.NodeType(nodeType)
		query.NodeType = &nt
	}

	if limit := c.Query("limit"); limit != "" {
		l, _ := strconv.Atoi(limit)
		query.Limit = l
	}

	nodes, err := h.service.SearchNodes(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search nodes"})
		return
	}

	c.JSON(http.StatusOK, nodes)
}

func (h *GraphHandler) UpdateNode(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var node models.GraphNode
	if err := c.ShouldBindJSON(&node); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	node.ID = id
	if err := h.service.UpdateNode(c.Request.Context(), &node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	c.JSON(http.StatusOK, node)
}

// Edge Operations
func (h *GraphHandler) CreateEdge(c *gin.Context) {
	var edge models.GraphEdge
	if err := c.ShouldBindJSON(&edge); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	edge.CreatedBy = &uid

	if err := h.service.CreateEdge(c.Request.Context(), &edge); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create edge", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, edge)
}

func (h *GraphHandler) GetConnections(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	depth, _ := strconv.Atoi(c.DefaultQuery("depth", "2"))

	result, err := h.service.GetConnectedNodes(c.Request.Context(), nodeID, depth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get connections"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Network Operations
func (h *GraphHandler) CreateNetwork(c *gin.Context) {
	var network models.CriminalNetwork
	if err := c.ShouldBindJSON(&network); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	uid := userID.(uuid.UUID)
	network.CreatedBy = &uid

	if err := h.service.CreateNetwork(c.Request.Context(), &network); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create network", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, network)
}

func (h *GraphHandler) GetNetwork(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	network, err := h.service.GetNetwork(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Network not found"})
		return
	}

	c.JSON(http.StatusOK, network)
}

func (h *GraphHandler) ListNetworks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := map[string]interface{}{
		"threat_level": c.Query("threat_level"),
		"type":         c.Query("type"),
	}

	networks, total, err := h.service.ListNetworks(c.Request.Context(), filters, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list networks"})
		return
	}

	totalPages := (int(total) + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       networks,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *GraphHandler) AddNetworkMember(c *gin.Context) {
	networkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid network ID"})
		return
	}

	var member models.NetworkMember
	if err := c.ShouldBindJSON(&member); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	member.NetworkID = networkID

	if err := h.service.AddNetworkMember(c.Request.Context(), &member); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	c.JSON(http.StatusCreated, member)
}

func (h *GraphHandler) GetNetworkMembers(c *gin.Context) {
	networkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid network ID"})
		return
	}

	members, err := h.service.GetNetworkMembers(c.Request.Context(), networkID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get members"})
		return
	}

	c.JSON(http.StatusOK, members)
}

// Analytics
func (h *GraphHandler) GetStatistics(c *gin.Context) {
	stats, err := h.service.GetStatistics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Graph visualization data
func (h *GraphHandler) GetVisualizationData(c *gin.Context) {
	// Get nodes
	nodes, err := h.service.SearchNodes(c.Request.Context(), models.GraphQuery{Limit: 500})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get nodes"})
		return
	}

	// Convert to visualization format
	visNodes := make([]map[string]interface{}, len(nodes))
	for i, n := range nodes {
		visNodes[i] = map[string]interface{}{
			"id":       n.ID.String(),
			"label":    n.Label,
			"type":     n.NodeType,
			"risk":     n.RiskScore,
			"isActive": n.IsActive,
		}
	}

	// Get all edges for these nodes
	allEdges := []map[string]interface{}{}
	nodeSet := make(map[uuid.UUID]bool)
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}

	for _, n := range nodes {
		edges, _ := h.service.GetEdgesForNode(c.Request.Context(), n.ID)
		for _, e := range edges {
			// Only include edges where both nodes are in our set
			if nodeSet[e.FromNodeID] && nodeSet[e.ToNodeID] {
				allEdges = append(allEdges, map[string]interface{}{
					"id":       e.ID.String(),
					"source":   e.FromNodeID.String(),
					"target":   e.ToNodeID.String(),
					"type":     e.RelationType,
					"weight":   e.Weight,
					"isActive": e.IsActive,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"nodes": visNodes,
		"edges": allEdges,
	})
}
