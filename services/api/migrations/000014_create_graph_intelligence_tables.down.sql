-- Drop Graph Intelligence Tables

DROP TRIGGER IF EXISTS update_member_count ON network_members;
DROP FUNCTION IF EXISTS update_network_member_count();

DROP TRIGGER IF EXISTS update_criminal_networks_updated_at ON criminal_networks;
DROP TRIGGER IF EXISTS update_graph_edges_updated_at ON graph_edges;
DROP TRIGGER IF EXISTS update_graph_nodes_updated_at ON graph_nodes;

DROP INDEX IF EXISTS idx_pattern_matches_reviewed;
DROP INDEX IF EXISTS idx_pattern_matches_type;
DROP INDEX IF EXISTS idx_network_members_person;
DROP INDEX IF EXISTS idx_network_members_network;
DROP INDEX IF EXISTS idx_criminal_networks_active;
DROP INDEX IF EXISTS idx_criminal_networks_threat;
DROP INDEX IF EXISTS idx_criminal_networks_type;
DROP INDEX IF EXISTS idx_graph_edges_active;
DROP INDEX IF EXISTS idx_graph_edges_type;
DROP INDEX IF EXISTS idx_graph_edges_to;
DROP INDEX IF EXISTS idx_graph_edges_from;
DROP INDEX IF EXISTS idx_graph_nodes_active;
DROP INDEX IF EXISTS idx_graph_nodes_source;
DROP INDEX IF EXISTS idx_graph_nodes_properties;
DROP INDEX IF EXISTS idx_graph_nodes_label;
DROP INDEX IF EXISTS idx_graph_nodes_type;

DROP TABLE IF EXISTS pattern_matches;
DROP TABLE IF EXISTS network_members;
DROP TABLE IF EXISTS criminal_networks;
DROP TABLE IF EXISTS graph_edges;
DROP TABLE IF EXISTS graph_nodes;

DROP TYPE IF EXISTS relation_type;
DROP TYPE IF EXISTS graph_node_type;
