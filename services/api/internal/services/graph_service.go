package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type GraphService struct {
	repo      *repository.GraphRepository
	auditRepo *repository.AuditRepository
}

func NewGraphService(repo *repository.GraphRepository, auditRepo *repository.AuditRepository) *GraphService {
	return &GraphService{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// Node Operations
func (s *GraphService) CreateNode(ctx context.Context, node *models.GraphNode) error {
	return s.repo.CreateNode(ctx, node)
}

func (s *GraphService) GetNode(ctx context.Context, id uuid.UUID) (*models.GraphNode, error) {
	return s.repo.GetNode(ctx, id)
}

func (s *GraphService) SearchNodes(ctx context.Context, query models.GraphQuery) ([]models.GraphNode, error) {
	return s.repo.SearchNodes(ctx, query)
}

func (s *GraphService) UpdateNode(ctx context.Context, node *models.GraphNode) error {
	return s.repo.UpdateNode(ctx, node)
}

// Edge Operations
func (s *GraphService) CreateEdge(ctx context.Context, edge *models.GraphEdge) error {
	return s.repo.CreateEdge(ctx, edge)
}

func (s *GraphService) GetEdgesForNode(ctx context.Context, nodeID uuid.UUID) ([]models.GraphEdge, error) {
	return s.repo.GetEdgesForNode(ctx, nodeID)
}

// Network Operations
func (s *GraphService) CreateNetwork(ctx context.Context, network *models.CriminalNetwork) error {
	return s.repo.CreateNetwork(ctx, network)
}

func (s *GraphService) GetNetwork(ctx context.Context, id uuid.UUID) (*models.CriminalNetwork, error) {
	return s.repo.GetNetwork(ctx, id)
}

func (s *GraphService) ListNetworks(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CriminalNetwork, int64, error) {
	return s.repo.ListNetworks(ctx, filters, page, pageSize)
}

func (s *GraphService) AddNetworkMember(ctx context.Context, member *models.NetworkMember) error {
	return s.repo.AddNetworkMember(ctx, member)
}

func (s *GraphService) GetNetworkMembers(ctx context.Context, networkID uuid.UUID) ([]models.NetworkMember, error) {
	return s.repo.GetNetworkMembers(ctx, networkID)
}

// Analytics
func (s *GraphService) GetConnectedNodes(ctx context.Context, nodeID uuid.UUID, depth int) (*models.GraphQueryResult, error) {
	return s.repo.GetConnectedNodes(ctx, nodeID, depth)
}

func (s *GraphService) GetStatistics(ctx context.Context) (*models.GraphStatistics, error) {
	return s.repo.GetGraphStatistics(ctx)
}

// Pattern Detection
func (s *GraphService) DetectPatterns(ctx context.Context) ([]models.PatternMatch, error) {
	// This would typically use more sophisticated algorithms
	// For now, return empty slice
	return []models.PatternMatch{}, nil
}

func (s *GraphService) SavePatternMatch(ctx context.Context, pattern *models.PatternMatch) error {
	return s.repo.SavePatternMatch(ctx, pattern)
}

// Link Analysis
func (s *GraphService) FindShortestPath(ctx context.Context, fromID, toID uuid.UUID) (*models.PathResult, error) {
	// BFS implementation for shortest path
	result, err := s.repo.GetConnectedNodes(ctx, fromID, 5)
	if err != nil {
		return nil, err
	}

	// Build adjacency map
	adjacency := make(map[uuid.UUID][]uuid.UUID)
	edgeMap := make(map[string]models.GraphEdge)

	for _, edge := range result.Edges {
		adjacency[edge.FromNodeID] = append(adjacency[edge.FromNodeID], edge.ToNodeID)
		adjacency[edge.ToNodeID] = append(adjacency[edge.ToNodeID], edge.FromNodeID)
		edgeMap[edge.FromNodeID.String()+"-"+edge.ToNodeID.String()] = edge
		edgeMap[edge.ToNodeID.String()+"-"+edge.FromNodeID.String()] = edge
	}

	// BFS
	visited := make(map[uuid.UUID]bool)
	parent := make(map[uuid.UUID]uuid.UUID)
	queue := []uuid.UUID{fromID}
	visited[fromID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == toID {
			// Reconstruct path
			path := []models.GraphNode{}
			edges := []models.GraphEdge{}
			curr := toID

			nodeMap := make(map[uuid.UUID]models.GraphNode)
			for _, n := range result.Nodes {
				nodeMap[n.ID] = n
			}

			for curr != fromID {
				if node, ok := nodeMap[curr]; ok {
					path = append([]models.GraphNode{node}, path...)
				}
				prev := parent[curr]
				key := prev.String() + "-" + curr.String()
				if edge, ok := edgeMap[key]; ok {
					edges = append([]models.GraphEdge{edge}, edges...)
				}
				curr = prev
			}

			if node, ok := nodeMap[fromID]; ok {
				path = append([]models.GraphNode{node}, path...)
			}

			return &models.PathResult{
				Path:     path,
				Edges:    edges,
				Distance: len(path) - 1,
			}, nil
		}

		for _, neighbor := range adjacency[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				parent[neighbor] = current
				queue = append(queue, neighbor)
			}
		}
	}

	return nil, nil // No path found
}

// Auto-populate from FIR/Case data
func (s *GraphService) PopulateFromFIR(ctx context.Context, fir *models.FIR, userID uuid.UUID) error {
	// Create node for complainant
	complainantNode := &models.GraphNode{
		NodeType:   models.NodeTypePerson,
		Label:      fir.ComplainantName,
		Properties: map[string]interface{}{
			"phone": fir.ComplainantPhone,
			"role":  "complainant",
		},
		SourceType: "FIR",
		SourceID:   &fir.ID,
		IsActive:   true,
		CreatedBy:  &userID,
	}
	s.repo.CreateNode(ctx, complainantNode)

	// Create node for FIR
	firNode := &models.GraphNode{
		NodeType:   models.NodeTypeFIR,
		Label:      fir.FIRNumber,
		Properties: map[string]interface{}{
			"status":   fir.Status,
			"priority": fir.Priority,
		},
		SourceType: "FIR",
		SourceID:   &fir.ID,
		IsActive:   true,
		CreatedBy:  &userID,
	}
	s.repo.CreateNode(ctx, firNode)

	// Create edge linking complainant to FIR
	edge := &models.GraphEdge{
		FromNodeID:   complainantNode.ID,
		ToNodeID:     firNode.ID,
		RelationType: models.RelationTypeLinkedTo,
		Properties: map[string]interface{}{
			"role": "complainant",
		},
		Weight:     1.0,
		Confidence: 1.0,
		Source:     "FIR",
		IsActive:   true,
		CreatedBy:  &userID,
	}
	s.repo.CreateEdge(ctx, edge)

	return nil
}
