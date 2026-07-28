ALTER TABLE provider_location_mappings
    RENAME COLUMN raw_response_hash TO normalized_response_hash;

ALTER TABLE location_postal_codes
    RENAME COLUMN raw_response_hash TO normalized_response_hash;
