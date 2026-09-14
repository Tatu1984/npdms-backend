-- Performance indexes for common query patterns.
--
-- Partial indexes originally filtered on CURRENT_DATE / NOW(). Postgres rejects
-- those: an index predicate must be immutable, or the index would silently go
-- stale as the clock moves. The predicates are dropped; the indexes still serve
-- the same queries, just without pre-filtering.

-- FIR indexes
CREATE INDEX IF NOT EXISTS idx_firs_status_created ON firs(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_firs_station_status ON firs(station_id, status);
CREATE INDEX IF NOT EXISTS idx_firs_io_status ON firs(investigating_officer, status);
CREATE INDEX IF NOT EXISTS idx_firs_priority_status ON firs(priority, status);
CREATE INDEX IF NOT EXISTS idx_firs_incident_date ON firs(incident_date DESC);
CREATE INDEX IF NOT EXISTS idx_firs_created_today ON firs(created_at);

-- Case indexes
CREATE INDEX IF NOT EXISTS idx_cases_status_priority ON cases(status, priority);
CREATE INDEX IF NOT EXISTS idx_cases_fir_id ON cases(fir_id);
CREATE INDEX IF NOT EXISTS idx_cases_io ON cases(investigating_officer);

-- Evidence indexes
CREATE INDEX IF NOT EXISTS idx_evidence_case_id ON evidence(case_id);
CREATE INDEX IF NOT EXISTS idx_evidence_fir_id ON evidence(fir_id);
CREATE INDEX IF NOT EXISTS idx_evidence_status ON evidence(status);
-- The column is evidence_type, not type.
CREATE INDEX IF NOT EXISTS idx_evidence_type ON evidence(evidence_type);

-- Warrant indexes
CREATE INDEX IF NOT EXISTS idx_warrants_status_created ON warrants(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_warrants_case_id ON warrants(case_id);
-- The column is executed_date, not execution_date.
CREATE INDEX IF NOT EXISTS idx_warrants_execution_date ON warrants(executed_date);

-- Court hearing indexes
CREATE INDEX IF NOT EXISTS idx_court_hearings_date ON court_hearings(hearing_date);
CREATE INDEX IF NOT EXISTS idx_court_hearings_case ON court_hearings(case_id);
CREATE INDEX IF NOT EXISTS idx_court_hearings_upcoming ON court_hearings(hearing_date);

-- Forensic request indexes
CREATE INDEX IF NOT EXISTS idx_forensic_requests_status ON forensic_requests(status);
CREATE INDEX IF NOT EXISTS idx_forensic_requests_evidence ON forensic_requests(evidence_id);

-- Citizen complaint indexes
CREATE INDEX IF NOT EXISTS idx_complaints_status_category ON citizen_complaints(status, category);
CREATE INDEX IF NOT EXISTS idx_complaints_station_status ON citizen_complaints(station_id, status);
CREATE INDEX IF NOT EXISTS idx_complaints_submitted ON citizen_complaints(submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_complaints_pending ON citizen_complaints(status)
    WHERE status IN ('SUBMITTED', 'ACKNOWLEDGED', 'ASSIGNED');

-- Traffic challan indexes
CREATE INDEX IF NOT EXISTS idx_challans_status ON traffic_challans(status);
CREATE INDEX IF NOT EXISTS idx_challans_vehicle ON traffic_challans(vehicle_number);
CREATE INDEX IF NOT EXISTS idx_challans_pending ON traffic_challans(status) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_challans_location ON traffic_challans(violation_location);
CREATE INDEX IF NOT EXISTS idx_challans_violation_date ON traffic_challans(violation_date DESC);

-- Cyber crime indexes
CREATE INDEX IF NOT EXISTS idx_cyber_crimes_status_type ON cyber_crimes(status, type);
CREATE INDEX IF NOT EXISTS idx_cyber_crimes_platform ON cyber_crimes(platform, status);
CREATE INDEX IF NOT EXISTS idx_cyber_crimes_reported ON cyber_crimes(reported_at DESC);

-- Graph intelligence indexes
CREATE INDEX IF NOT EXISTS idx_graph_nodes_active_type ON graph_nodes(is_active, node_type);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_label ON graph_nodes(label) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_graph_edges_from ON graph_edges(from_node_id, is_active);
CREATE INDEX IF NOT EXISTS idx_graph_edges_to ON graph_edges(to_node_id, is_active);
-- graph_edges stores the relationship in relation_type.
CREATE INDEX IF NOT EXISTS idx_graph_edges_type ON graph_edges(relation_type);

-- Audit log indexes.
-- Migration 000016 replaced audit_logs with the immutable schema: the actor is
-- actor_user_id and the time is event_timestamp.
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(event_timestamp DESC);

-- User indexes
CREATE INDEX IF NOT EXISTS idx_users_station ON users(station_id) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role) WHERE is_active = true;

-- Personnel indexes
CREATE INDEX IF NOT EXISTS idx_personnel_station ON personnel(station_id);
CREATE INDEX IF NOT EXISTS idx_personnel_rank ON personnel(rank);

-- Vehicle indexes
CREATE INDEX IF NOT EXISTS idx_vehicles_station ON vehicles(station_id);
CREATE INDEX IF NOT EXISTS idx_vehicles_status ON vehicles(status);

-- Alert indexes. The alerts table from 000012 has no status or severity column;
-- priority and acknowledged carry that meaning.
CREATE INDEX IF NOT EXISTS idx_alerts_priority_created ON alerts(priority, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at DESC);

-- State/District/Station hierarchy indexes
-- The hierarchy is state > zone > range > district, so districts key on range_id.
CREATE INDEX IF NOT EXISTS idx_districts_range ON districts(range_id);
CREATE INDEX IF NOT EXISTS idx_police_stations_district ON police_stations(district_id);
CREATE INDEX IF NOT EXISTS idx_ranges_zone ON ranges(zone_id);
CREATE INDEX IF NOT EXISTS idx_zones_state ON zones(state_id);

-- AI Review indexes
CREATE INDEX IF NOT EXISTS idx_ai_decisions_status ON ai_decisions(status);
CREATE INDEX IF NOT EXISTS idx_ai_decisions_model ON ai_decisions(model_name, status);
CREATE INDEX IF NOT EXISTS idx_ai_decisions_assigned ON ai_decisions(assigned_to) WHERE status = 'PENDING_REVIEW';

-- Full text search indexes
CREATE INDEX IF NOT EXISTS idx_firs_search ON firs USING gin(to_tsvector('english',
    COALESCE(fir_number, '') || ' ' ||
    COALESCE(complainant_name, '') || ' ' ||
    COALESCE(incident_description, '')));

CREATE INDEX IF NOT EXISTS idx_cases_search ON cases USING gin(to_tsvector('english',
    COALESCE(case_number, '') || ' ' ||
    COALESCE(title, '') || ' ' ||
    COALESCE(synopsis, '')));

-- Analyze tables after creating indexes
ANALYZE firs;
ANALYZE cases;
ANALYZE evidence;
ANALYZE warrants;
ANALYZE court_hearings;
ANALYZE citizen_complaints;
ANALYZE traffic_challans;
ANALYZE cyber_crimes;
ANALYZE graph_nodes;
ANALYZE graph_edges;
ANALYZE audit_logs;
ANALYZE users;
