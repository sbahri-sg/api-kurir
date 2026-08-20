UPDATE couriers
SET active = true,
    updated_at = now()
WHERE code IN ('paxel', 'dse', 'ncs', 'star');
