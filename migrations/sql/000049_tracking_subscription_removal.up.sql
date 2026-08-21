ALTER TABLE tracking_shipments
    ADD COLUMN polling_enabled boolean NOT NULL DEFAULT true;
