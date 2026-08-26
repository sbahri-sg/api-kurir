DELETE FROM partner_explorer_credentials
WHERE credential_code <> 'default';

ALTER TABLE partner_explorer_credentials
    DROP CONSTRAINT partner_explorer_credentials_pkey,
    DROP CONSTRAINT partner_explorer_credentials_code_check,
    DROP COLUMN credential_code;

ALTER TABLE partner_explorer_credentials
    ADD PRIMARY KEY (provider_code);
