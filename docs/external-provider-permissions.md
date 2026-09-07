# External provider permission foundation

The main API supports opt-in rate checks through `EXTERNAL_PROVIDER_GRANT_FILE`.
When unset, no new grant calls or binding queries run. Rate adapters, shipment
handlers and built-in Emisell shipping retain their existing path. Nothing
enrolls existing merchants automatically.

## Rollout contract

`NewProviderGate` requires a server-owned provider policy with an explicit
`MerchantIDs` list. Empty or wildcard lists are rejected; `emisell` cannot be
enrolled. Unenrolled merchant/provider pairs retain existing authorization.
An enrolled pair fails closed for missing/inactive binding, revoked/missing grant,
or unavailable verification. Keep enrollment after uninstall so it cannot fall
back to legacy access. Enrollment removal is an operator migration decision.

Bindings hold merchant/provider/app/installation identities and credential
references only. Synchronization never supplies or changes credentials. A real
credential verifier must enforce merchant ownership and provider association.
Grant checks use current Apps Platform state, not the stored active flag.

## Synchronization

Migration 000077 adds a separate binding table; it does not migrate merchants or
modify old shipping records. Migration has not been run by this change.
`apps/provider-sync` can be invoked manually or with `-watch` using a private
configuration file (`DatabaseURL`, `PlatformURL`, `EngineKey`, `AllowLoopback`).
No worker is started by the default API or compose configuration. Configure only
approved external apps on the Platform side; never enroll built-in Emisell.

## Rate runtime (default off)

Use an absolute private JSON file (0600, not a symlink) mounted read-only into
the API container. Fields: `PlatformURL` (HTTPS), `EngineKey`, `Providers`.
Each provider policy requires `RequiresCredential: true` and an explicit
`MerchantIDs` array. Do not put the engine key in source control. Container
mounting and this variable must be configured separately by an operator.
The gate runs before rate cache/singleflight. The credential reference must match
the currently selected, active, validated merchant credential in existing storage.
No credential is decrypted by the verifier. Missing references fail closed.

## Still required before activation

- Connect remaining business operations (shipment, settings, tracking). Current
  runtime wiring protects rates only; do not claim full lifecycle enforcement.
- Provide an authenticated credential-binding workflow. Synchronization does not
  populate credential references automatically.
- Test install, credential connection, rates and revocation end-to-end in staging.
- Validate database migration against a staging backup before deployment.

This foundation is not an enabled RajaOngkir integration and is not a production
readiness claim. Tests use synthetic data; no shipment is created.
