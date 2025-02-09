CREATE TABLE schedules ( id SERIAL PRIMARY KEY, food_id INTEGER REFERENCES foods(id) ON DELETE CASCADE, scheduled_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW(), updated_at TIMESTAMPTZ DEFAULT NOW() );

-- Add index for range queries
CREATE INDEX idx_schedules_scheduled_at ON schedules(scheduled_at);