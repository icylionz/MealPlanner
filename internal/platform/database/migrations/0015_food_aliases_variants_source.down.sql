ALTER TABLE foods
    DROP COLUMN source_last_imported_at,
    DROP COLUMN source_url;

ALTER TABLE food_components
    DROP COLUMN variant_text;

DROP TABLE food_aliases;
