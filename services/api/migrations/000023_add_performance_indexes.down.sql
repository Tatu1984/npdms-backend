-- Drop performance indexes

-- FIR indexes
DROP INDEX IF EXISTS idx_firs_status_created;
DROP INDEX IF EXISTS idx_firs_station_status;
DROP INDEX IF EXISTS idx_firs_io_status;
DROP INDEX IF EXISTS idx_firs_priority_status;
DROP INDEX IF EXISTS idx_firs_incident_date;
DROP INDEX IF EXISTS idx_firs_created_today;
DROP INDEX IF EXISTS idx_firs_search;

-- Case indexes
DROP INDEX IF EXISTS idx_cases_status_priority;
DROP INDEX IF EXISTS idx_cases_fir_id;
DROP INDEX IF EXISTS idx_cases_io;
DROP INDEX IF EXISTS idx_cases_search;

-- Evidence indexes
DROP INDEX IF EXISTS idx_evidence_case_id;
DROP INDEX IF EXISTS idx_evidence_fir_id;
DROP INDEX IF EXISTS idx_evidence_status;
DROP INDEX IF EXISTS idx_evidence_type;

-- Warrant indexes
DROP INDEX IF EXISTS idx_warrants_status_created;
DROP INDEX IF EXISTS idx_warrants_case_id;
DROP INDEX IF EXISTS idx_warrants_execution_date;

-- Court hearing indexes
DROP INDEX IF EXISTS idx_court_hearings_date;
DROP INDEX IF EXISTS idx_court_hearings_case;
DROP INDEX IF EXISTS idx_court_hearings_upcoming;

-- Forensic request indexes
DROP INDEX IF EXISTS idx_forensic_requests_status;
DROP INDEX IF EXISTS idx_forensic_requests_evidence;

-- Citizen complaint indexes
DROP INDEX IF EXISTS idx_complaints_status_category;
DROP INDEX IF EXISTS idx_complaints_station_status;
DROP INDEX IF EXISTS idx_complaints_submitted;
DROP INDEX IF EXISTS idx_complaints_pending;

-- Traffic challan indexes
DROP INDEX IF EXISTS idx_challans_status;
DROP INDEX IF EXISTS idx_challans_vehicle;
DROP INDEX IF EXISTS idx_challans_pending;
DROP INDEX IF EXISTS idx_challans_location;
DROP INDEX IF EXISTS idx_challans_violation_date;

-- Cyber crime indexes
DROP INDEX IF EXISTS idx_cyber_crimes_status_type;
DROP INDEX IF EXISTS idx_cyber_crimes_platform;
DROP INDEX IF EXISTS idx_cyber_crimes_reported;

-- Graph intelligence indexes
DROP INDEX IF EXISTS idx_graph_nodes_active_type;
DROP INDEX IF EXISTS idx_graph_nodes_label;
DROP INDEX IF EXISTS idx_graph_edges_from;
DROP INDEX IF EXISTS idx_graph_edges_to;
DROP INDEX IF EXISTS idx_graph_edges_type;

-- Audit log indexes
DROP INDEX IF EXISTS idx_audit_logs_user;
DROP INDEX IF EXISTS idx_audit_logs_resource;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_created;

-- User indexes
DROP INDEX IF EXISTS idx_users_station;
DROP INDEX IF EXISTS idx_users_role;

-- Personnel indexes
DROP INDEX IF EXISTS idx_personnel_station;
DROP INDEX IF EXISTS idx_personnel_rank;

-- Vehicle indexes
DROP INDEX IF EXISTS idx_vehicles_station;
DROP INDEX IF EXISTS idx_vehicles_status;

-- Alert indexes
DROP INDEX IF EXISTS idx_alerts_status;
DROP INDEX IF EXISTS idx_alerts_severity;
DROP INDEX IF EXISTS idx_alerts_created;
DROP INDEX IF EXISTS idx_alerts_active;

-- State/District/Station hierarchy indexes
DROP INDEX IF EXISTS idx_districts_state;
DROP INDEX IF EXISTS idx_police_stations_district;
DROP INDEX IF EXISTS idx_ranges_zone;
DROP INDEX IF EXISTS idx_zones_state;

-- AI Review indexes
DROP INDEX IF EXISTS idx_ai_decisions_status;
DROP INDEX IF EXISTS idx_ai_decisions_model;
DROP INDEX IF EXISTS idx_ai_decisions_assigned;

-- Sync indexes
DROP INDEX IF EXISTS idx_sync_outbox_pending;
DROP INDEX IF EXISTS idx_sync_inbox_pending;
