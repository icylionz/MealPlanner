ALTER TABLE meal_series
    DROP CONSTRAINT IF EXISTS meal_series_version_positive,
    DROP COLUMN IF EXISTS version;
