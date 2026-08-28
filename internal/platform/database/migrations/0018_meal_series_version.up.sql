-- G13: recurring edits use a series-wide optimistic token so overlapping
-- this-and-future/all-occurrences writes cannot silently overwrite each other.
ALTER TABLE meal_series
    ADD COLUMN version integer NOT NULL DEFAULT 1,
    ADD CONSTRAINT meal_series_version_positive CHECK (version >= 1);
