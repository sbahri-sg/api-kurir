DELETE FROM shipping_integration_providers
WHERE code = 'rajaongkir_hosted'
  AND active_release_id IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM partner_integration_submissions submission
      WHERE submission.provider_code = shipping_integration_providers.code
  );
