package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/npdms/api/internal/models"
)

type GraphRepository struct {
	db *sqlx.DB
}

func NewGraphRepository(db *sqlx.DB) *GraphRepository {
	return &GraphRepository{db: db}
}

// Node Operations
func (r *GraphRepository) CreateNode(ctx context.Context, node *models.GraphNode) error {
	node.ID = uuid.New()
	node.CreatedAt = time.Now()
	node.UpdatedAt = time.Now()

	propsJSON, _ := json.Marshal(node.Properties)

	query := `
		INSERT INTO graph_nodes (
			id, node_type, label, properties, external_id,
			source_type, source_id, risk_score, last_seen,
			is_active, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := r.db.ExecContext(ctx, query,
		node.ID, node.NodeType, node.Label, propsJSON, node.ExternalID,
		node.SourceType, node.SourceID, node.RiskScore, node.LastSeen,
		node.IsActive, node.CreatedBy, node.CreatedAt, node.UpdatedAt,
	)
	return err
}

func (r *GraphRepository) GetNode(ctx context.Context, id uuid.UUID) (*models.GraphNode, error) {
	var node models.GraphNode
	var propsJSON []byte

	query := `SELECT * FROM graph_nodes WHERE id = $1`
	row := r.db.QueryRowContext(ctx, query, id)

	err := row.Scan(
		&node.ID, &node.NodeType, &node.Label, &propsJSON, &node.ExternalID,
		&node.SourceType, &node.SourceID, &node.RiskScore, &node.LastSeen,
		&node.IsActive, &node.CreatedBy, &node.CreatedAt, &node.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("node not found")
		}
		return nil, err
	}

	json.Unmarshal(propsJSON, &node.Properties)
	return &node, nil
}

func (r *GraphRepository) SearchNodes(ctx context.Context, query models.GraphQuery) ([]models.GraphNode, error) {
	var nodes []models.GraphNode

	sqlQuery := `SELECT * FROM graph_nodes WHERE is_active = true`
	args := []interface{}{}
	argIndex := 1

	if query.NodeType != nil {
		sqlQuery += fmt.Sprintf(" AND node_type = $%d", argIndex)
		args = append(args, *query.NodeType)
		argIndex++
	}

	if query.Keyword != "" {
		sqlQuery += fmt.Sprintf(" AND label ILIKE $%d", argIndex)
		args = append(args, "%"+query.Keyword+"%")
		argIndex++
	}

	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	sqlQuery += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var node models.GraphNode
		var propsJSON []byte
		err := rows.Scan(
			&node.ID, &node.NodeType, &node.Label, &propsJSON, &node.ExternalID,
			&node.SourceType, &node.SourceID, &node.RiskScore, &node.LastSeen,
			&node.IsActive, &node.CreatedBy, &node.CreatedAt, &node.UpdatedAt,
		)
		if err != nil {
			continue
		}
		json.Unmarshal(propsJSON, &node.Properties)
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (r *GraphRepository) UpdateNode(ctx context.Context, node *models.GraphNode) error {
	node.UpdatedAt = time.Now()
	propsJSON, _ := json.Marshal(node.Properties)

	query := `
		UPDATE graph_nodes SET
			label = $2, properties = $3, risk_score = $4,
			last_seen = $5, is_active = $6, updated_at = $7
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		node.ID, node.Label, propsJSON, node.RiskScore,
		node.LastSeen, node.IsActive, node.UpdatedAt,
	)
	return err
}

// Edge Operations
func (r *GraphRepository) CreateEdge(ctx context.Context, edge *models.GraphEdge) error {
	edge.ID = uuid.New()
	edge.CreatedAt = time.Now()
	edge.UpdatedAt = time.Now()

	propsJSON, _ := json.Marshal(edge.Properties)

	query := `
		INSERT INTO graph_edges (
			id, from_node_id, to_node_id, relation_type, properties,
			weight, confidence, source, start_date, end_date,
			is_active, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := r.db.ExecContext(ctx, query,
		edge.ID, edge.FromNodeID, edge.ToNodeID, edge.RelationType, propsJSON,
		edge.Weight, edge.Confidence, edge.Source, edge.StartDate, edge.EndDate,
		edge.IsActive, edge.CreatedBy, edge.CreatedAt, edge.UpdatedAt,
	)
	return err
}

func (r *GraphRepository) GetEdgesForNode(ctx context.Context, nodeID uuid.UUID) ([]models.GraphEdge, error) {
	var edges []models.GraphEdge

	query := `
		SELECT * FROM graph_edges
		WHERE (from_node_id = $1 OR to_node_id = $1) AND is_active = true`

	rows, err := r.db.QueryContext(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var edge models.GraphEdge
		var propsJSON []byte
		err := rows.Scan(
			&edge.ID, &edge.FromNodeID, &edge.ToNodeID, &edge.RelationType, &propsJSON,
			&edge.Weight, &edge.Confidence, &edge.Source, &edge.StartDate, &edge.EndDate,
			&edge.IsActive, &edge.CreatedBy, &edge.CreatedAt, &edge.UpdatedAt,
		)
		if err != nil {
			continue
		}
		json.Unmarshal(propsJSON, &edge.Properties)
		edges = append(edges, edge)
	}

	return edges, nil
}

// Network Operations
func (r *GraphRepository) CreateNetwork(ctx context.Context, network *models.CriminalNetwork) error {
	network.ID = uuid.New()
	network.CreatedAt = time.Now()
	network.UpdatedAt = time.Now()

	query := `
		INSERT INTO criminal_networks (
			id, name, alias, type, threat_level, member_count,
			leader_id, operating_areas, crime_types, established_date,
			linked_cases, linked_firs, description, is_active, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err := r.db.ExecContext(ctx, query,
		network.ID, network.Name, network.Alias, network.Type, network.ThreatLevel, network.MemberCount,
		network.LeaderID, pq.Array(network.OperatingAreas), pq.Array(network.CrimeTypes), network.EstablishedDate,
		pq.Array(network.LinkedCases), pq.Array(network.LinkedFIRs), network.Description, network.IsActive,
		network.CreatedBy, network.CreatedAt, network.UpdatedAt,
	)
	return err
}

func (r *GraphRepository) GetNetwork(ctx context.Context, id uuid.UUID) (*models.CriminalNetwork, error) {
	var network models.CriminalNetwork
	query := `SELECT * FROM criminal_networks WHERE id = $1`
	err := r.db.GetContext(ctx, &network, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("network not found")
		}
		return nil, err
	}
	return &network, nil
}

func (r *GraphRepository) ListNetworks(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CriminalNetwork, int64, error) {
	var networks []models.CriminalNetwork
	var total int64

	baseQuery := `FROM criminal_networks WHERE is_active = true`
	args := []interface{}{}
	argIndex := 1

	if threatLevel, ok := filters["threat_level"].(string); ok && threatLevel != "" {
		baseQuery += fmt.Sprintf(" AND threat_level = $%d", argIndex)
		args = append(args, threatLevel)
		argIndex++
	}

	if networkType, ok := filters["type"].(string); ok && networkType != "" {
		baseQuery += fmt.Sprintf(" AND type = $%d", argIndex)
		args = append(args, networkType)
		argIndex++
	}

	countQuery := "SELECT COUNT(*) " + baseQuery
	r.db.GetContext(ctx, &total, countQuery, args...)

	offset := (page - 1) * pageSize
	selectQuery := `SELECT * ` + baseQuery + fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, pageSize, offset)

	r.db.SelectContext(ctx, &networks, selectQuery, args...)

	return networks, total, nil
}

func (r *GraphRepository) AddNetworkMember(ctx context.Context, member *models.NetworkMember) error {
	member.ID = uuid.New()
	member.CreatedAt = time.Now()

	query := `
		INSERT INTO network_members (
			id, network_id, person_id, role, joined_date,
			left_date, is_active, notes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.db.ExecContext(ctx, query,
		member.ID, member.NetworkID, member.PersonID, member.Role, member.JoinedDate,
		member.LeftDate, member.IsActive, member.Notes, member.CreatedAt,
	)
	return err
}

func (r *GraphRepository) GetNetworkMembers(ctx context.Context, networkID uuid.UUID) ([]models.NetworkMember, error) {
	var members []models.NetworkMember
	query := `
		SELECT nm.*, gn.label as person_name
		FROM network_members nm
		JOIN graph_nodes gn ON nm.person_id = gn.id
		WHERE nm.network_id = $1 AND nm.is_active = true
		ORDER BY nm.role, nm.joined_date`
	err := r.db.SelectContext(ctx, &members, query, networkID)
	return members, err
}

// Graph Analysis
func (r *GraphRepository) GetConnectedNodes(ctx context.Context, nodeID uuid.UUID, depth int) (*models.GraphQueryResult, error) {
	result := &models.GraphQueryResult{
		Nodes: []models.GraphNode{},
		Edges: []models.GraphEdge{},
	}

	// Get the starting node
	startNode, err := r.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	result.Nodes = append(result.Nodes, *startNode)

	// BFS to find connected nodes up to specified depth
	visited := make(map[uuid.UUID]bool)
	visited[nodeID] = true
	currentLevel := []uuid.UUID{nodeID}

	for d := 0; d < depth && len(currentLevel) > 0; d++ {
		nextLevel := []uuid.UUID{}

		for _, nid := range currentLevel {
			edges, _ := r.GetEdgesForNode(ctx, nid)
			for _, edge := range edges {
				result.Edges = append(result.Edges, edge)

				connectedID := edge.ToNodeID
				if edge.ToNodeID == nid {
					connectedID = edge.FromNodeID
				}

				if !visited[connectedID] {
					visited[connectedID] = true
					nextLevel = append(nextLevel, connectedID)

					node, _ := r.GetNode(ctx, connectedID)
					if node != nil {
						result.Nodes = append(result.Nodes, *node)
					}
				}
			}
		}

		currentLevel = nextLevel
	}

	return result, nil
}

func (r *GraphRepository) GetGraphStatistics(ctx context.Context) (*models.GraphStatistics, error) {
	stats := &models.GraphStatistics{
		NodesByType:      make(map[string]int64),
		EdgesByType:      make(map[string]int64),
		TopConnectedNodes: []models.TopNode{},
		HighRiskNodes:    []models.GraphNode{},
	}

	r.db.GetContext(ctx, &stats.TotalNodes, "SELECT COUNT(*) FROM graph_nodes WHERE is_active = true")
	r.db.GetContext(ctx, &stats.TotalEdges, "SELECT COUNT(*) FROM graph_edges WHERE is_active = true")
	r.db.GetContext(ctx, &stats.TotalNetworks, "SELECT COUNT(*) FROM criminal_networks WHERE is_active = true")

	// Nodes by type
	rows, _ := r.db.QueryContext(ctx, "SELECT node_type, COUNT(*) FROM graph_nodes WHERE is_active = true GROUP BY node_type")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var count int64
			rows.Scan(&t, &count)
			stats.NodesByType[t] = count
		}
	}

	// Edges by type
	rows2, _ := r.db.QueryContext(ctx, "SELECT relation_type, COUNT(*) FROM graph_edges WHERE is_active = true GROUP BY relation_type")
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var t string
			var count int64
			rows2.Scan(&t, &count)
			stats.EdgesByType[t] = count
		}
	}

	// High risk nodes
	var highRisk []models.GraphNode
	r.db.SelectContext(ctx, &highRisk, `
		SELECT * FROM graph_nodes
		WHERE is_active = true AND risk_score >= 0.7
		ORDER BY risk_score DESC LIMIT 10`)
	stats.HighRiskNodes = highRisk

	return stats, nil
}

// Pattern Detection
func (r *GraphRepository) SavePatternMatch(ctx context.Context, pattern *models.PatternMatch) error {
	pattern.ID = uuid.New()
	pattern.DetectedAt = time.Now()

	query := `
		INSERT INTO pattern_matches (
			id, pattern_type, confidence, node_ids, edge_ids, description, detected_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	nodeIDs := make([]string, len(pattern.Nodes))
	for i, n := range pattern.Nodes {
		nodeIDs[i] = n.ID.String()
	}
	edgeIDs := make([]string, len(pattern.Edges))
	for i, e := range pattern.Edges {
		edgeIDs[i] = e.ID.String()
	}

	_, err := r.db.ExecContext(ctx, query,
		pattern.ID, pattern.PatternType, pattern.Confidence,
		pq.Array(nodeIDs), pq.Array(edgeIDs), pattern.Description,
		pattern.DetectedAt, time.Now(),
	)
	return err
}
