-- FR19: owners record the total price paid for a grocery line at its
-- amount/unit. Food cost estimates are derived from the latest priced line per
-- food at read time; nothing is stored on foods. cost_recorded_at orders
-- "latest" across lists.
ALTER TABLE grocery_items
    ADD COLUMN cost numeric,
    ADD COLUMN cost_recorded_at timestamptz,
    ADD CONSTRAINT grocery_items_cost_nonnegative CHECK (cost IS NULL OR cost >= 0);
