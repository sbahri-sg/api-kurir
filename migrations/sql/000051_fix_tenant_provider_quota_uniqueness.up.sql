-- The original global uniqueness constraint was left in place when tenant_id
-- was introduced. It prevents multiple Emisell merchants from consuming the
-- same shared platform credential alias on the same day.
ALTER TABLE provider_quota_ledger
    DROP CONSTRAINT IF EXISTS provider_quota_ledger_provider_code_credential_alias_quota__key;
