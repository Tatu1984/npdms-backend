-- Create court_hearings table
CREATE TABLE IF NOT EXISTS court_hearings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    court VARCHAR(255) NOT NULL,
    court_room VARCHAR(100) NOT NULL,
    judge_name VARCHAR(255) NOT NULL,
    hearing_date DATE NOT NULL,
    hearing_time VARCHAR(20) NOT NULL,
    type VARCHAR(30) NOT NULL CHECK (type IN ('ARGUMENTS', 'EVIDENCE', 'BAIL_HEARING', 'REMAND_EXTENSION', 'JUDGMENT', 'CHARGESHEET')),
    investigating_officer UUID REFERENCES users(id) ON DELETE SET NULL,
    ipc_sections TEXT[] DEFAULT '{}',
    required_documents TEXT[] DEFAULT '{}',
    priority VARCHAR(20) NOT NULL CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create court_orders table
CREATE TABLE IF NOT EXISTS court_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    order_date DATE NOT NULL,
    order_type VARCHAR(30) NOT NULL CHECK (order_type IN ('REMAND', 'BAIL_REJECTED', 'BAIL_GRANTED', 'DIRECTIONS', 'JUDGMENT', 'STAY')),
    summary TEXT NOT NULL,
    court VARCHAR(255) NOT NULL,
    judge_name VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for court_hearings
CREATE INDEX idx_court_hearings_case_id ON court_hearings(case_id);
CREATE INDEX idx_court_hearings_hearing_date ON court_hearings(hearing_date);
CREATE INDEX idx_court_hearings_type ON court_hearings(type);
CREATE INDEX idx_court_hearings_priority ON court_hearings(priority);
CREATE INDEX idx_court_hearings_io ON court_hearings(investigating_officer);

-- Create indexes for court_orders
CREATE INDEX idx_court_orders_case_id ON court_orders(case_id);
CREATE INDEX idx_court_orders_order_date ON court_orders(order_date);
CREATE INDEX idx_court_orders_order_type ON court_orders(order_type);

-- Create updated_at triggers
CREATE TRIGGER update_court_hearings_updated_at
    BEFORE UPDATE ON court_hearings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_court_orders_updated_at
    BEFORE UPDATE ON court_orders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
