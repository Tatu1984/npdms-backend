-- Graph Intelligence Tables Migration

-- Node Type Enum
CREATE TYPE graph_node_type AS ENUM (
    'PERSON', 'ORGANIZATION', 'LOCATION', 'VEHICLE', 'PHONE',
    'EMAIL', 'BANK_ACCOUNT', 'CRYPTO_WALLET', 'CASE', 'FIR', 'EVIDENCE', 'WEAPON'
);

-- Relation Type Enum
CREATE TYPE relation_type AS ENUM (
    'ASSOCIATE', 'FAMILY_MEMBER', 'EMPLOYER', 'EMPLOYEE', 'OWNS',
    'RESIDES', 'FREQUENTS', 'COMMUNICATES', 'TRANSACTS', 'SUSPECTED_OF',
    'WITNESSED_BY', 'LINKED_TO', 'CO_ACCUSED', 'GANG_MEMBER'
);

-- Graph Nodes Table
CREATE TABLE graph_nodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    node_type graph_node_type NOT NULL,
    label VARCHAR(500) NOT NULL,
    properties JSONB DEFAULT '{}',
    external_id VARCHAR(255),
    source_type VARCHAR(50), -- FIR, CASE, INTELLIGENCE, OSINT
    source_id UUID,
    risk_score DECIMAL(5, 2),
    last_seen TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Graph Edges Table
CREATE TABLE graph_edges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    from_node_id UUID REFERENCES graph_nodes(id) ON DELETE CASCADE,
    to_node_id UUID REFERENCES graph_nodes(id) ON DELETE CASCADE,
    relation_type relation_type NOT NULL,
    properties JSONB DEFAULT '{}',
    weight DECIMAL(5, 2) DEFAULT 1.0,
    confidence DECIMAL(5, 2) DEFAULT 1.0,
    source VARCHAR(255),
    start_date TIMESTAMP WITH TIME ZONE,
    end_date TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Criminal Networks Table
CREATE TABLE criminal_networks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    alias VARCHAR(255),
    type VARCHAR(50) NOT NULL, -- GANG, SYNDICATE, CELL, NETWORK
    threat_level VARCHAR(20) DEFAULT 'MEDIUM', -- LOW, MEDIUM, HIGH, CRITICAL
    member_count INTEGER DEFAULT 0,
    leader_id UUID REFERENCES graph_nodes(id),
    operating_areas TEXT[],
    crime_types TEXT[],
    established_date DATE,
    linked_cases UUID[],
    linked_firs UUID[],
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Network Members Table
CREATE TABLE network_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    network_id UUID REFERENCES criminal_networks(id) ON DELETE CASCADE,
    person_id UUID REFERENCES graph_nodes(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL, -- LEADER, LIEUTENANT, MEMBER, ASSOCIATE
    joined_date DATE,
    left_date DATE,
    is_active BOOLEAN DEFAULT true,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(network_id, person_id)
);

-- Pattern Matches Table (for detected patterns)
CREATE TABLE pattern_matches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pattern_type VARCHAR(100) NOT NULL, -- MONEY_LAUNDERING, ORGANIZED_CRIME, FRAUD_RING
    confidence DECIMAL(5, 2) NOT NULL,
    node_ids UUID[] NOT NULL,
    edge_ids UUID[] NOT NULL,
    description TEXT,
    detected_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    reviewed BOOLEAN DEFAULT false,
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMP WITH TIME ZONE,
    is_valid BOOLEAN,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_graph_nodes_type ON graph_nodes(node_type);
CREATE INDEX idx_graph_nodes_label ON graph_nodes USING gin(to_tsvector('english', label));
CREATE INDEX idx_graph_nodes_properties ON graph_nodes USING gin(properties);
CREATE INDEX idx_graph_nodes_source ON graph_nodes(source_type, source_id);
CREATE INDEX idx_graph_nodes_active ON graph_nodes(is_active) WHERE is_active = true;

CREATE INDEX idx_graph_edges_from ON graph_edges(from_node_id);
CREATE INDEX idx_graph_edges_to ON graph_edges(to_node_id);
CREATE INDEX idx_graph_edges_type ON graph_edges(relation_type);
CREATE INDEX idx_graph_edges_active ON graph_edges(is_active) WHERE is_active = true;

CREATE INDEX idx_criminal_networks_type ON criminal_networks(type);
CREATE INDEX idx_criminal_networks_threat ON criminal_networks(threat_level);
CREATE INDEX idx_criminal_networks_active ON criminal_networks(is_active) WHERE is_active = true;

CREATE INDEX idx_network_members_network ON network_members(network_id);
CREATE INDEX idx_network_members_person ON network_members(person_id);

CREATE INDEX idx_pattern_matches_type ON pattern_matches(pattern_type);
CREATE INDEX idx_pattern_matches_reviewed ON pattern_matches(reviewed) WHERE reviewed = false;

-- Triggers
CREATE TRIGGER update_graph_nodes_updated_at BEFORE UPDATE ON graph_nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_graph_edges_updated_at BEFORE UPDATE ON graph_edges
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_criminal_networks_updated_at BEFORE UPDATE ON criminal_networks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Function to update network member count
CREATE OR REPLACE FUNCTION update_network_member_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE criminal_networks SET member_count = member_count + 1 WHERE id = NEW.network_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE criminal_networks SET member_count = member_count - 1 WHERE id = OLD.network_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_member_count
AFTER INSERT OR DELETE ON network_members
FOR EACH ROW EXECUTE FUNCTION update_network_member_count();
