-- Rollback traffic challan tables

DROP TRIGGER IF EXISTS update_defaulter_on_challan ON traffic_challans;
DROP TRIGGER IF EXISTS update_traffic_challans_updated_at ON traffic_challans;
DROP TRIGGER IF EXISTS update_challan_defaulters_updated_at ON challan_defaulters;

DROP FUNCTION IF EXISTS update_defaulter_record();
DROP FUNCTION IF EXISTS generate_challan_number(VARCHAR);

DROP TABLE IF EXISTS challan_hotspots;
DROP TABLE IF EXISTS challan_defaulters;
DROP TABLE IF EXISTS challan_payments;
DROP TABLE IF EXISTS traffic_challans;
DROP TABLE IF EXISTS traffic_violation_types;
