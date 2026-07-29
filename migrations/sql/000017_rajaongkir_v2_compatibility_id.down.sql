ALTER TABLE locations
    DROP CONSTRAINT IF EXISTS locations_compatibility_id_key;

ALTER TABLE locations
    DROP COLUMN IF EXISTS compatibility_id;
