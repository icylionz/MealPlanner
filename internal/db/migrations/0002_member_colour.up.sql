-- Member colour is now a Household design-system member index ('1'..'8')
-- instead of a hex value, so a person keeps the same token in every theme.
ALTER TABLE household_members ALTER COLUMN color SET DEFAULT '1';

UPDATE household_members m
SET color = sub.c
FROM (
    SELECT id,
           (((row_number() OVER (PARTITION BY household_id ORDER BY created_at, id)) - 1) % 8 + 1)::text AS c
    FROM household_members
) sub
WHERE sub.id = m.id;
