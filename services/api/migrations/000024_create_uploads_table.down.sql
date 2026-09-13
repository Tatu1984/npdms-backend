-- Drop uploads table and related objects
DROP TRIGGER IF EXISTS uploads_updated_at ON uploads;
DROP FUNCTION IF EXISTS update_uploads_updated_at();
DROP INDEX IF EXISTS idx_uploads_entity;
DROP INDEX IF EXISTS idx_uploads_uploaded_by;
DROP INDEX IF EXISTS idx_uploads_category;
DROP INDEX IF EXISTS idx_uploads_created;
DROP INDEX IF EXISTS idx_uploads_object_key;
DROP TABLE IF EXISTS uploads;
