ALTER TABLE provider_quota_ledger
    ADD CONSTRAINT provider_quota_ledger_provider_code_credential_alias_quota__key
    UNIQUE (provider_code, credential_alias, quota_date);
