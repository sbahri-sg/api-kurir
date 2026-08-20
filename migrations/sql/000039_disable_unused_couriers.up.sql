UPDATE couriers
SET active = false,
    updated_at = now()
WHERE code IN ('paxel', 'dse', 'ncs', 'star');
