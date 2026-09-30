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

## Operator credential connection

After synchronizing an active installation, use `apps/provider-connect` in the
trusted backend environment. Supply `DATABASE_URL` through secret configuration
and `-config` pointing to the private runtime file. Pass `-merchant`, `-provider`,
`-app`, `-installation`, and `-credential` (an existing credential ID, never a key).
The operator must be authorized to administer that merchant. This is not a public
endpoint and does not provide seller authentication.

The tool checks explicit rollout, exact installation identity, fresh
`settings.write` permission, and current credential selection/ownership. The
database update rechecks active/valid selection and rejects concurrent binding
changes or synchronized revocation. A concurrent remote revocation can race the
write; subsequent rate requests still verify the current remote grant and deny.
No credential is copied, created or selected, and no shipping history is deleted.

## Service-only binding endpoint (opt-in)

`POST /api/v1/app-integrations/credential-binding` is registered only with the
external runtime enabled. Uses existing service-key authentication plus trusted
merchant context. JSON fields: `app_id`, `installation_id`, `provider_code`,
`credential_id`. It does not accept secrets or merchant identity in the body.
Browser Origin/Cookie requests are rejected. Success returns only
`{"status":"connected"}`. It reuses operator grant/ownership checks; 403 denies,
503 means verification failed, and 400 means invalid input. Never send the service
key to a seller browser. The calling application backend must authenticate its
seller session and authorize that merchant before using this endpoint.

Credential creation/validation still uses existing backend credential services.
There is no combined seller onboarding transaction yet. Connecting does not create
an installation or automatically activate the selected shipping provider.

## Still required before activation

- Connect remaining business operations (shipment, settings, tracking). Current
  runtime wiring protects rates only; do not claim full lifecycle enforcement.
- Connect the seller-authenticated application backend to the service endpoint.
  Synchronization never binds credentials. The Platform V2 installation source
  still requires runtime composition/readiness before end-to-end onboarding.
- Test install, credential connection, rates and revocation end-to-end in staging.
- Validate database migration against a staging backup before deployment.

This foundation is not an enabled RajaOngkir integration and is not a production
readiness claim. Tests use synthetic data; no shipment is created.
