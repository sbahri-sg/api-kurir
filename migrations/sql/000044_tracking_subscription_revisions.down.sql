DROP TABLE IF EXISTS tracking_subscription_revisions;

ALTER TABLE tracking_subscriptions
    DROP COLUMN IF EXISTS revision;
