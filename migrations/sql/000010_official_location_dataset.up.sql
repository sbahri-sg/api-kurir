CREATE UNIQUE INDEX IF NOT EXISTS locations_official_region_unique_idx
    ON locations (official_region_code)
    WHERE official_region_code IS NOT NULL;

CREATE TABLE location_dataset_imports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_code text NOT NULL,
    dataset_version text NOT NULL,
    source_url text NOT NULL,
    source_commit text NOT NULL,
    sha256 text NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    record_count integer NOT NULL CHECK (record_count >= 0),
    active boolean NOT NULL DEFAULT true,
    imported_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (source_code, source_commit, sha256)
);

CREATE INDEX location_dataset_imports_active_idx
    ON location_dataset_imports (source_code, active, imported_at DESC);
