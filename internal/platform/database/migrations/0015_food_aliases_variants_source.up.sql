CREATE TABLE food_aliases (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    food_id    uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    alias      text NOT NULL CHECK (btrim(alias) <> ''),
    created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX food_aliases_food_alias_idx
    ON food_aliases (food_id, lower(alias));
CREATE INDEX food_aliases_alias_idx ON food_aliases (lower(alias));

ALTER TABLE food_components
    ADD COLUMN variant_text text NOT NULL DEFAULT '';

ALTER TABLE foods
    ADD COLUMN source_url text,
    ADD COLUMN source_last_imported_at timestamptz;
