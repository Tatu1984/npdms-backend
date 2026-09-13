-- Uploads metadata table for tracking all file uploads
CREATE TABLE IF NOT EXISTS uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_key VARCHAR(500) NOT NULL UNIQUE,
    original_filename VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    category VARCHAR(50) NOT NULL,
    bucket_name VARCHAR(100) NOT NULL,

    -- Entity reference (e.g., FIR, Case, Evidence)
    entity_type VARCHAR(50),
    entity_id UUID,

    -- Upload metadata
    uploaded_by UUID NOT NULL REFERENCES users(id),
    uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Public URL for access
    public_url TEXT,

    -- Soft delete
    is_deleted BOOLEAN DEFAULT false,
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by UUID REFERENCES users(id),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for common queries
CREATE INDEX idx_uploads_entity ON uploads(entity_type, entity_id) WHERE is_deleted = false;
CREATE INDEX idx_uploads_uploaded_by ON uploads(uploaded_by);
CREATE INDEX idx_uploads_category ON uploads(category);
CREATE INDEX idx_uploads_created ON uploads(created_at DESC);
CREATE INDEX idx_uploads_object_key ON uploads(object_key);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_uploads_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER uploads_updated_at
    BEFORE UPDATE ON uploads
    FOR EACH ROW
    EXECUTE FUNCTION update_uploads_updated_at();

-- Add comment
COMMENT ON TABLE uploads IS 'Stores metadata for all file uploads to MinIO/S3 storage';
