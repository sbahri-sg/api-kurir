ALTER TABLE locations
    ALTER COLUMN compatibility_id DROP IDENTITY IF EXISTS;

ALTER TABLE locations
    ALTER COLUMN compatibility_id DROP NOT NULL;

UPDATE locations
SET compatibility_id = NULL;

UPDATE locations
SET compatibility_id = replace(official_region_code, '.', '')::bigint
WHERE official_region_code ~ '^[0-9]+(\.[0-9]+)*$';
