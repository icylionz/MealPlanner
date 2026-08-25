-- FR10: ingredient densities for volume<->weight conversion.
-- density_g_per_ml = 0 is the "no density" sentinel; density_source records
-- whether the value came from the starter set or a user override.
ALTER TABLE foods
    ADD COLUMN density_g_per_ml numeric NOT NULL DEFAULT 0,
    ADD COLUMN density_source   text NOT NULL DEFAULT 'none'
        CHECK (density_source IN ('starter', 'custom', 'none'));

-- Backfill starter densities for known seed ingredients (only where unset).
UPDATE foods SET density_g_per_ml = 1.0,   density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'water';
UPDATE foods SET density_g_per_ml = 1.03,  density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'oat milk';
UPDATE foods SET density_g_per_ml = 0.918, density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'olive oil';
UPDATE foods SET density_g_per_ml = 1.42,  density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'honey';
UPDATE foods SET density_g_per_ml = 1.03,  density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'lemon juice';
UPDATE foods SET density_g_per_ml = 0.94,  density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'caesar dressing';
UPDATE foods SET density_g_per_ml = 0.53,  density_source = 'starter' WHERE density_source = 'none' AND lower(name) = 'bread flour';
