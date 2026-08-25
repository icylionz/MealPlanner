-- Recurring scheduled meals (FR6).
-- A series holds the recurrence rule; concrete occurrences live in meal_plan
-- and carry series_id so edits/deletes can be scoped to this/future/all.
-- Weekdays are stored as a CSV of Go time.Weekday ints (0=Sun..6=Sat).
CREATE TABLE meal_series (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    food_id    uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    plan_time  text NOT NULL DEFAULT '12:00' CHECK (plan_time ~ '^[0-2][0-9]:[0-5][0-9]$'),
    servings   int  NOT NULL DEFAULT 2,
    freq       text NOT NULL CHECK (freq IN ('daily', 'weekly')),
    byweekday  text NOT NULL DEFAULT '',
    start_date date NOT NULL,
    until_date date NOT NULL
);

ALTER TABLE meal_plan
    ADD COLUMN series_id uuid REFERENCES meal_series (id) ON DELETE CASCADE;

CREATE INDEX meal_plan_series_idx ON meal_plan (series_id);
