-- FR13: rich link previews on scheduled meals. A meal may carry an external
-- URL plus fetched (or manually entered) preview title and image.
ALTER TABLE meal_plan
    ADD COLUMN link_url       text NOT NULL DEFAULT '',
    ADD COLUMN link_title     text NOT NULL DEFAULT '',
    ADD COLUMN link_image_url text NOT NULL DEFAULT '';
