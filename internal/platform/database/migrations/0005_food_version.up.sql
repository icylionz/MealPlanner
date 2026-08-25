-- FR16: optimistic locking for foods. Every save bumps version; a guarded
-- UPDATE ... WHERE version = <expected> rejects a stale write.
ALTER TABLE foods
    ADD COLUMN version int NOT NULL DEFAULT 1;
