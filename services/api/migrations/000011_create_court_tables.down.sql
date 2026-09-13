-- Drop court tables
DROP TRIGGER IF EXISTS update_court_orders_updated_at ON court_orders;
DROP TRIGGER IF EXISTS update_court_hearings_updated_at ON court_hearings;

DROP INDEX IF EXISTS idx_court_orders_order_type;
DROP INDEX IF EXISTS idx_court_orders_order_date;
DROP INDEX IF EXISTS idx_court_orders_case_id;

DROP INDEX IF EXISTS idx_court_hearings_io;
DROP INDEX IF EXISTS idx_court_hearings_priority;
DROP INDEX IF EXISTS idx_court_hearings_type;
DROP INDEX IF EXISTS idx_court_hearings_hearing_date;
DROP INDEX IF EXISTS idx_court_hearings_case_id;

DROP TABLE IF EXISTS court_orders;
DROP TABLE IF EXISTS court_hearings;
