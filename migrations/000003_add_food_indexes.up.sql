-- Indexes for autocomplete performance
CREATE INDEX IF NOT EXISTS idx_foods_name_prefix ON foods (name text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_foods_updated_at ON foods (updated_at DESC);