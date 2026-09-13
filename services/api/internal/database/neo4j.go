package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jClient wraps the Neo4j driver
type Neo4jClient struct {
	driver neo4j.DriverWithContext
	config Neo4jConfig
}

// Neo4jConfig holds Neo4j connection configuration
type Neo4jConfig struct {
	URI      string
	Username string
	Password string
	Database string
}

// NewNeo4jClient creates a new Neo4j client
func NewNeo4jClient(config Neo4jConfig) (*Neo4jClient, error) {
	auth := neo4j.BasicAuth(config.Username, config.Password, "")

	driver, err := neo4j.NewDriverWithContext(config.URI, auth, func(c *neo4j.Config) {
		c.MaxConnectionPoolSize = 50
		c.MaxConnectionLifetime = 30 * time.Minute
		c.ConnectionAcquisitionTimeout = 30 * time.Second
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Neo4j driver: %w", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, fmt.Errorf("failed to verify Neo4j connectivity: %w", err)
	}

	log.Println("Connected to Neo4j successfully")

	return &Neo4jClient{
		driver: driver,
		config: config,
	}, nil
}

// Close closes the Neo4j driver
func (c *Neo4jClient) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

// GetDriver returns the underlying Neo4j driver
func (c *Neo4jClient) GetDriver() neo4j.DriverWithContext {
	return c.driver
}

// Session creates a new session
func (c *Neo4jClient) Session(ctx context.Context, mode neo4j.AccessMode) neo4j.SessionWithContext {
	sessionConfig := neo4j.SessionConfig{
		AccessMode: mode,
	}
	if c.config.Database != "" {
		sessionConfig.DatabaseName = c.config.Database
	}
	return c.driver.NewSession(ctx, sessionConfig)
}

// ReadSession creates a new read-only session
func (c *Neo4jClient) ReadSession(ctx context.Context) neo4j.SessionWithContext {
	return c.Session(ctx, neo4j.AccessModeRead)
}

// WriteSession creates a new write session
func (c *Neo4jClient) WriteSession(ctx context.Context) neo4j.SessionWithContext {
	return c.Session(ctx, neo4j.AccessModeWrite)
}

// ExecuteRead executes a read transaction
func (c *Neo4jClient) ExecuteRead(ctx context.Context, work func(tx neo4j.ManagedTransaction) (interface{}, error)) (interface{}, error) {
	session := c.ReadSession(ctx)
	defer session.Close(ctx)

	return session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return work(tx)
	})
}

// ExecuteWrite executes a write transaction
func (c *Neo4jClient) ExecuteWrite(ctx context.Context, work func(tx neo4j.ManagedTransaction) (interface{}, error)) (interface{}, error) {
	session := c.WriteSession(ctx)
	defer session.Close(ctx)

	return session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return work(tx)
	})
}

// InitializeSchema creates indexes and constraints
func (c *Neo4jClient) InitializeSchema(ctx context.Context) error {
	session := c.WriteSession(ctx)
	defer session.Close(ctx)

	// Create constraints and indexes
	queries := []string{
		// Node indexes
		"CREATE INDEX node_type_idx IF NOT EXISTS FOR (n:Node) ON (n.nodeType)",
		"CREATE INDEX node_source_idx IF NOT EXISTS FOR (n:Node) ON (n.sourceType, n.sourceId)",
		"CREATE INDEX person_name_idx IF NOT EXISTS FOR (n:Person) ON (n.name)",
		"CREATE INDEX person_aadhaar_idx IF NOT EXISTS FOR (n:Person) ON (n.aadhaar)",
		"CREATE INDEX phone_number_idx IF NOT EXISTS FOR (n:Phone) ON (n.number)",
		"CREATE INDEX vehicle_reg_idx IF NOT EXISTS FOR (n:Vehicle) ON (n.registrationNumber)",
		"CREATE INDEX fir_number_idx IF NOT EXISTS FOR (n:FIR) ON (n.firNumber)",
		"CREATE INDEX case_number_idx IF NOT EXISTS FOR (n:Case) ON (n.caseNumber)",

		// Fulltext search indexes
		"CREATE FULLTEXT INDEX person_search_idx IF NOT EXISTS FOR (n:Person) ON EACH [n.name, n.alias, n.address]",
		"CREATE FULLTEXT INDEX location_search_idx IF NOT EXISTS FOR (n:Location) ON EACH [n.name, n.address, n.description]",

		// Relationship indexes
		"CREATE INDEX rel_type_idx IF NOT EXISTS FOR ()-[r:RELATED_TO]-() ON (r.relationType)",
	}

	for _, query := range queries {
		_, err := session.Run(ctx, query, nil)
		if err != nil {
			log.Printf("Warning: Failed to execute schema query: %v", err)
		}
	}

	log.Println("Neo4j schema initialized")
	return nil
}

// HealthCheck checks if Neo4j is healthy
func (c *Neo4jClient) HealthCheck(ctx context.Context) error {
	return c.driver.VerifyConnectivity(ctx)
}

// Stats returns Neo4j database statistics
func (c *Neo4jClient) Stats(ctx context.Context) (map[string]interface{}, error) {
	result, err := c.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// Get node count
		nodeResult, err := tx.Run(ctx, "MATCH (n) RETURN count(n) as nodeCount", nil)
		if err != nil {
			return nil, err
		}
		nodeRecord, err := nodeResult.Single(ctx)
		if err != nil {
			return nil, err
		}
		nodeCount, _ := nodeRecord.Get("nodeCount")

		// Get relationship count
		relResult, err := tx.Run(ctx, "MATCH ()-[r]->() RETURN count(r) as relCount", nil)
		if err != nil {
			return nil, err
		}
		relRecord, err := relResult.Single(ctx)
		if err != nil {
			return nil, err
		}
		relCount, _ := relRecord.Get("relCount")

		// Get node labels
		labelResult, err := tx.Run(ctx, "CALL db.labels() YIELD label RETURN collect(label) as labels", nil)
		if err != nil {
			return nil, err
		}
		labelRecord, err := labelResult.Single(ctx)
		if err != nil {
			return nil, err
		}
		labels, _ := labelRecord.Get("labels")

		// Get relationship types
		relTypeResult, err := tx.Run(ctx, "CALL db.relationshipTypes() YIELD relationshipType RETURN collect(relationshipType) as types", nil)
		if err != nil {
			return nil, err
		}
		relTypeRecord, err := relTypeResult.Single(ctx)
		if err != nil {
			return nil, err
		}
		relTypes, _ := relTypeRecord.Get("types")

		return map[string]interface{}{
			"nodeCount":         nodeCount,
			"relationshipCount": relCount,
			"labels":            labels,
			"relationshipTypes": relTypes,
		}, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(map[string]interface{}), nil
}
