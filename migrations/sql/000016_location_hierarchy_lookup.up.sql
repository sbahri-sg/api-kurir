CREATE INDEX locations_active_parent_level_idx
    ON locations (parent_id, level)
    WHERE active;
