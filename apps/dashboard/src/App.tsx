import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AdminApi,
	PartnerPortalApi,
  type Courier,
  type CustomerAPIKey,
  type GeneratedCustomerAPIKey,
  type LocationOption,
  type LocationMapping,
  type Overview,
	type PartnerSubmission,
	type PartnerSubmissionStatus,
	type PartnerAccessKey,
	type PartnerIdentity,
	type PartnerExplorerCatalog,
	type PartnerExplorerCredentialState,
	type PartnerExplorerExecution,
	type PartnerExplorerOperation,
	type PartnerExplorerRun,
  type ProviderCredential,
  type ProviderCredentialType,
  type ProviderQuota,
  type ShippingProvider,
  type ShippingProviderCreateInput,
  type ShippingProviderUpdateInput,
  type RateResult,
  type RateSnapshot,
  type TrackingShipment,
	type TrackingOperation,
	type TrackingOperationPage,
	type FulfillmentOperation,
	type FulfillmentOperationPage,
  type WebhookSettings,
  type WebhookTestResult,
} from "./api";
import {
  API_DOCUMENTATION,
  API_DOCUMENTATION_CONTRACTS,
  getApiDocumentationContract,
  type ApiDocumentationContract,
  type ApiDocumentationEndpoint,
} from "./apiDocumentation";

type Tab =
  | "overview"
  | "check-rate"
  | "tracking"
	| "tracking-operations"
	| "fulfillment-operations"
  | "couriers"
  | "rates"
  | "mappings"
  | "providers"
	| "partner-submissions"
  | "quota"
  | "api-keys"
  | "webhook"
  | "documentation";

type NavGroupId = "operations" | "master-data" | "provider" | "developer";
type ApiDocumentationView = ApiDocumentationContract | "partner";

type NavItem = {
  value: Tab;
  label: string;
  section?: string;
};

type NavGroup = {
  id: NavGroupId;
  label: string;
  description: string;
  items: NavItem[];
};

const NAV_GROUPS: NavGroup[] = [
  {
    id: "operations",
    label: "Operasional",
    description: "Tarif dan resi",
    items: [
      { value: "check-rate", label: "Cek Ongkir" },
      { value: "tracking", label: "Cek Resi" },
	  { value: "tracking-operations", label: "Monitor Resi" },
	  { value: "fulfillment-operations", label: "Monitor Fulfillment" },
    ],
  },
  {
    id: "master-data",
    label: "Master Data",
    description: "Ekspedisi dan layanan",
    items: [{ value: "couriers", label: "Ekspedisi & Service" }],
  },
  {
    id: "provider",
    label: "Integrasi Provider",
    description: "Koneksi dan legacy",
    items: [
      { value: "providers", label: "Provider" },
	  { value: "partner-submissions", label: "Partner Packages" },
      { value: "quota", label: "Credential & Kuota" },
      {
        value: "rates",
        label: "Snapshot Tarif",
        section: "Legacy RajaOngkir",
      },
      {
        value: "mappings",
        label: "Mapping Lokasi",
        section: "Legacy RajaOngkir",
      },
    ],
  },
  {
    id: "developer",
    label: "Developer",
    description: "Akses dan dokumentasi",
    items: [
      { value: "webhook", label: "Webhook" },
      { value: "documentation", label: "Dokumentasi API" },
      { value: "api-keys", label: "API Key" },
    ],
  },
];

const TAB_NAV_GROUP: Partial<Record<Tab, NavGroupId>> = Object.fromEntries(
  NAV_GROUPS.flatMap((group) =>
    group.items.map((item) => [item.value, group.id]),
  ),
);

const EMPTY_OVERVIEW: Overview = {
  active_rate_cards: 0,
  official_locations: 0,
  location_mappings: 0,
  fresh_quote_snapshots: 0,
  quota_used_today: 0,
  quota_limit_today: 0,
  tracking_pending_jobs: 0,
  tracking_shipments: 0,
};

const EMPTY_WEBHOOK_SETTINGS: WebhookSettings = {
  configured: false,
  callback_url: "",
  enabled: false,
  secret_configured: false,
  secret_hint: "",
  source: "database",
  last_test_at: null,
  last_test_success: null,
  last_test_http_status: null,
  last_test_error: "",
  updated_by: "",
  updated_at: null,
};

const WEBHOOK_REQUEST_HEADERS = `POST /api/v1/webhooks/tracking HTTP/1.1
Content-Type: application/json
X-Emisell-Event-ID: 8fd90a2e-1c2e-4ca2-9b15-4546396d94de
X-Emisell-Event-Type: tracking.status_changed
X-Emisell-Webhook-Timestamp: 1787200800
X-Emisell-Webhook-Signature: v1=<hex-hmac-sha256>`;

const WEBHOOK_TRACKING_PAYLOAD = `{
  "id": "8fd90a2e-1c2e-4ca2-9b15-4546396d94de",
  "type": "tracking.status_changed",
  "api_version": "2026-08-20",
  "occurred_at": "2026-08-20T10:00:00Z",
  "data": {
    "merchant_id": "merchant_123",
    "order_id": "order_123",
    "fulfillment_id": "fulfillment_123",
    "tracking_revision": 2,
    "shipment": {
      "courier": "jnt",
      "waybill": "JY1224870535",
      "validation_status": "valid",
      "status": "in_transit",
      "status_label": "Dalam perjalanan",
      "provider": "rajaongkir",
      "provider_fetched_at": "2026-08-20T10:00:00Z",
      "next_refresh_at": "2026-08-20T22:00:00Z",
      "shipped_at": "2026-08-20T09:00:00Z",
      "delivered_at": null,
      "is_final": false
    }
  }
}`;

const WEBHOOK_TEST_PAYLOAD = `{
  "id": "evt_test_123",
  "type": "tracking.test",
  "api_version": "2026-08-20",
  "occurred_at": "2026-08-20T10:00:00Z",
  "data": {
    "source": "api-kurir-dashboard",
    "message": "Webhook test berhasil diterima."
  }
}`;

const WEBHOOK_NODE_VERIFICATION = `import {
  createHmac,
  timingSafeEqual,
} from "node:crypto";

export function verifyApiKurirWebhook(rawBody, headers, secret) {
  const timestamp = headers["x-emisell-webhook-timestamp"];
  const received = headers["x-emisell-webhook-signature"];
  const timestampNumber = Number(timestamp);
  const age = Math.abs(Date.now() / 1000 - timestampNumber);
  if (!timestamp || !received || !Number.isFinite(timestampNumber) || age > 300) {
    return false;
  }

  const expected = "v1=" + createHmac("sha256", secret)
    .update(timestamp + ".")
    .update(rawBody)
    .digest("hex");
  const left = Buffer.from(received);
  const right = Buffer.from(expected);
  return left.length === right.length && timingSafeEqual(left, right);
}

// rawBody wajib Buffer asli sebelum JSON.parse().
// Setelah valid, simpan X-Emisell-Event-ID untuk deduplikasi.`;

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Terjadi kesalahan.";
}

function isUnauthorized(error: unknown) {
  return (
    error instanceof Error &&
    (error as Error & { status?: number }).status === 401
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("id-ID").format(value);
}

function formatMoney(value: number) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(value);
}

function trackingErrorMessage(code?: string) {
  switch (code) {
    case "PHONE_VALIDATION_REQUIRED":
      return "Provider ekspedisi meminta verifikasi penerima. Sistem tidak meminta nomor telepon dan akan mencoba kembali melalui jalur yang tersedia.";
    case "WAYBILL_NOT_FOUND":
      return "Nomor resi belum ditemukan oleh provider. Pastikan ekspedisi sudah benar atau tunggu sampai resi aktif.";
    case "PROVIDER_QUOTA_EXHAUSTED":
      return "Kuota provider sedang habis. Antrean akan dilanjutkan otomatis setelah kuota tersedia.";
    case "PROVIDER_RATE_LIMITED":
      return "Provider sedang membatasi request sementara. Sistem akan mencoba kembali otomatis dalam beberapa menit.";
    case "PROVIDER_UNAUTHORIZED":
      return "Kredensial provider perlu diperiksa oleh admin.";
    case "PROVIDER_ERROR":
      return "Provider tracking sedang tidak dapat dihubungi. Sistem akan mencoba kembali otomatis.";
    case "DECRYPTION_FAILED":
      return "Data legacy atau konteks privat resi tidak dapat dibaca worker. Kunci enkripsi perlu diperiksa oleh admin.";
    default:
      return "";
  }
}

function trackingStatusText(result: TrackingShipment) {
  if (result.status_label) return result.status_label;
  if (result.status === "unknown") return "Status belum diketahui";
  return result.status.replaceAll("_", " ");
}

function formatDate(value: string | null) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("id-ID", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatCatalogDate(value: string) {
  if (!value) return "Belum diverifikasi";
  return new Intl.DateTimeFormat("id-ID", {
    dateStyle: "long",
  }).format(new Date(`${value}T00:00:00`));
}

function formatETD(minimum: number | null, maximum: number | null) {
  if (minimum === null && maximum === null) return "—";
  if (minimum === maximum || maximum === null) return `${minimum} hari`;
  if (minimum === null) return `maks. ${maximum} hari`;
  return `${minimum}–${maximum} hari`;
}

export function App() {
  return window.location.pathname.startsWith("/partner")
    ? <PartnerPortal />
    : <AdminDashboard />;
}

function AdminDashboard() {
  const [adminKey, setAdminKey] = useState(
    () => sessionStorage.getItem("api-kurir-admin-key") ?? "",
  );
  const [actor, setActor] = useState(
    () => sessionStorage.getItem("api-kurir-admin-actor") ?? "emisell",
  );
  const [authenticated, setAuthenticated] = useState(Boolean(adminKey));
  const [tab, setTab] = useState<Tab>("overview");
  const [expandedNavGroup, setExpandedNavGroup] =
    useState<NavGroupId | null>(() => {
      const stored = sessionStorage.getItem("api-kurir-nav-group");
      return NAV_GROUPS.some((group) => group.id === stored)
        ? (stored as NavGroupId)
        : null;
    });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [overview, setOverview] = useState(EMPTY_OVERVIEW);
  const [couriers, setCouriers] = useState<Courier[]>([]);
  const [snapshots, setSnapshots] = useState<RateSnapshot[]>([]);
  const [mappings, setMappings] = useState<LocationMapping[]>([]);
  const [quotas, setQuotas] = useState<ProviderQuota[]>([]);
  const [shippingProviders, setShippingProviders] = useState<ShippingProvider[]>([]);
  const [providerCredentials, setProviderCredentials] = useState<
    ProviderCredential[]
  >([]);
  const [showProviderKeyModal, setShowProviderKeyModal] = useState(false);
  const [providerKeyCode, setProviderKeyCode] = useState<
    "rajaongkir" | "biteship"
  >("rajaongkir");
  const [providerKeySecret, setProviderKeySecret] = useState("");
  const [providerKeyLoading, setProviderKeyLoading] = useState(false);
  const [apiKeys, setAPIKeys] = useState<CustomerAPIKey[]>([]);
  const [generatedAPIKey, setGeneratedAPIKey] =
    useState<GeneratedCustomerAPIKey | null>(null);
  const [webhookSettings, setWebhookSettings] = useState<WebhookSettings>(
    EMPTY_WEBHOOK_SETTINGS,
  );
  const [generatedWebhookSecret, setGeneratedWebhookSecret] = useState("");
  const [webhookTestResult, setWebhookTestResult] =
    useState<WebhookTestResult | null>(null);
  const [webhookActionLoading, setWebhookActionLoading] = useState(false);
  const [keyActionLoading, setKeyActionLoading] = useState(false);
  const [rateSearch, setRateSearch] = useState("");
  const [mappingSearch, setMappingSearch] = useState("");
  const [documentationView, setDocumentationView] =
    useState<ApiDocumentationView>("rajaongkir-v2");

  const api = useMemo(
    () => new AdminApi(adminKey, actor || "emisell"),
    [adminKey, actor],
  );

  const loadDashboard = useCallback(
    async (signal?: AbortSignal) => {
      if (!adminKey) return;
      setLoading(true);
      setError("");
      try {
        const [
          overviewData,
          courierData,
          snapshotData,
          mappingsData,
          quotaData,
          shippingProviderData,
          apiKeyData,
          providerCredentialData,
          webhookSettingsData,
        ] =
          await Promise.all([
            api.overview(signal),
            api.couriers(signal),
            api.rateSnapshots("", signal),
            api.mappings("", "", signal),
            api.quotas(signal),
            api.shippingProviders(signal),
            api.apiKeys(signal),
            api.providerCredentials(signal),
            api.webhookSettings(signal),
          ]);
        setOverview(overviewData);
        setCouriers(courierData);
        setSnapshots(snapshotData);
        setMappings(mappingsData);
        setQuotas(quotaData);
        setShippingProviders(shippingProviderData);
        setAPIKeys(apiKeyData);
        setProviderCredentials(providerCredentialData);
        setWebhookSettings(webhookSettingsData);
        setAuthenticated(true);
      } catch (loadError) {
        if (loadError instanceof Error && loadError.name === "AbortError")
          return;
        if (isUnauthorized(loadError)) {
          sessionStorage.removeItem("api-kurir-admin-key");
          sessionStorage.removeItem("api-kurir-admin-actor");
          setAdminKey("");
          setActor("emisell");
        }
        setAuthenticated(false);
        setError(getErrorMessage(loadError));
      } finally {
        setLoading(false);
      }
    },
    [adminKey, api],
  );

  useEffect(() => {
    if (!authenticated || !adminKey) return;
    const controller = new AbortController();
    void loadDashboard(controller.signal);
    return () => controller.abort();
  }, [adminKey, authenticated, loadDashboard]);

  useEffect(() => {
    const groupId = TAB_NAV_GROUP[tab];
    if (groupId) setExpandedNavGroup(groupId);
  }, [tab]);

  useEffect(() => {
    if (expandedNavGroup) {
      sessionStorage.setItem("api-kurir-nav-group", expandedNavGroup);
      return;
    }
    sessionStorage.removeItem("api-kurir-nav-group");
  }, [expandedNavGroup]);

  async function login(event: FormEvent) {
    event.preventDefault();
    if (!adminKey.trim()) {
      setError("Masukkan admin API key.");
      return;
    }
    sessionStorage.setItem("api-kurir-admin-key", adminKey.trim());
    sessionStorage.setItem("api-kurir-admin-actor", actor.trim() || "emisell");
    await loadDashboard();
  }

  function logout() {
    sessionStorage.removeItem("api-kurir-admin-key");
    sessionStorage.removeItem("api-kurir-admin-actor");
    setAdminKey("");
    setActor("emisell");
    setAuthenticated(false);
    setOverview(EMPTY_OVERVIEW);
    setCouriers([]);
    setSnapshots([]);
    setMappings([]);
    setQuotas([]);
    setShippingProviders([]);
    setAPIKeys([]);
    setProviderCredentials([]);
    setGeneratedAPIKey(null);
    setWebhookSettings(EMPTY_WEBHOOK_SETTINGS);
    setGeneratedWebhookSecret("");
    setWebhookTestResult(null);
  }

  async function refreshRates() {
    setLoading(true);
    setError("");
    try {
      setSnapshots(await api.rateSnapshots(rateSearch));
    } catch (refreshError) {
      setError(getErrorMessage(refreshError));
    } finally {
      setLoading(false);
    }
  }

  async function refreshMappings() {
    setLoading(true);
    setError("");
    try {
      setMappings(await api.mappings(mappingSearch));
    } catch (refreshError) {
      setError(getErrorMessage(refreshError));
    } finally {
      setLoading(false);
    }
  }

  async function generateAPIKey(kind: "public" | "main_service") {
    setKeyActionLoading(true);
    setError("");
    try {
      const generated = await api.generateAPIKey(kind);
      setGeneratedAPIKey(generated);
      setAPIKeys((current) => [
        generated.api_key,
        ...current.filter((item) => item.id !== generated.api_key.id),
      ]);
    } catch (generateError) {
      setError(getErrorMessage(generateError));
      throw generateError;
    } finally {
      setKeyActionLoading(false);
    }
  }

  async function revokeAPIKey(id: string) {
    setKeyActionLoading(true);
    setError("");
    try {
      await api.revokeAPIKey(id);
      setAPIKeys(await api.apiKeys());
    } catch (revokeError) {
      setError(getErrorMessage(revokeError));
    } finally {
      setKeyActionLoading(false);
    }
  }

  async function addProviderCredential(event: FormEvent) {
    event.preventDefault();
    if (!providerKeySecret.trim()) return;
    setProviderKeyLoading(true);
    setError("");
    try {
      await api.addProviderCredential(
        providerKeyCode,
        providerKeySecret.trim(),
      );
      const [credentialData, quotaData] = await Promise.all([
        api.providerCredentials(),
        api.quotas(),
      ]);
      setProviderCredentials(credentialData);
      setQuotas(quotaData);
      setProviderKeySecret("");
      setProviderKeyCode("rajaongkir");
      setShowProviderKeyModal(false);
    } catch (credentialError) {
      setError(getErrorMessage(credentialError));
    } finally {
      setProviderKeyLoading(false);
    }
  }

  async function disableProviderCredential(id: string) {
    setProviderKeyLoading(true);
    setError("");
    try {
      await api.disableProviderCredential(id);
      setProviderCredentials(await api.providerCredentials());
    } catch (credentialError) {
      setError(getErrorMessage(credentialError));
    } finally {
      setProviderKeyLoading(false);
    }
  }

  async function saveWebhook(callbackURL: string, enabled: boolean) {
    setWebhookActionLoading(true);
    setError("");
    try {
      const settings = await api.updateWebhookSettings(callbackURL, enabled);
      setWebhookSettings(settings);
      setWebhookTestResult(null);
    } catch (saveError) {
      setError(getErrorMessage(saveError));
      throw saveError;
    } finally {
      setWebhookActionLoading(false);
    }
  }

  async function generateWebhookSecret() {
    setWebhookActionLoading(true);
    setError("");
    try {
      const generated = await api.generateWebhookSecret();
      setWebhookSettings(generated.settings);
      setGeneratedWebhookSecret(generated.secret);
      setWebhookTestResult(null);
    } catch (generateError) {
      setError(getErrorMessage(generateError));
    } finally {
      setWebhookActionLoading(false);
    }
  }

  async function testWebhook() {
    setWebhookActionLoading(true);
    setError("");
    try {
      const result = await api.testWebhook();
      setWebhookTestResult(result);
      setWebhookSettings(await api.webhookSettings());
    } catch (testError) {
      setError(getErrorMessage(testError));
    } finally {
      setWebhookActionLoading(false);
    }
  }

  if (!authenticated) {
    return (
      <main className="login-shell">
        <section className="login-card">
          <div className="brand-mark">EK</div>
          <p className="eyebrow">EMISELL OPERATIONS</p>
          <h1>Masuk ke API Kurir</h1>
          <p>
            Admin key hanya disimpan selama sesi browser dan tidak masuk ke
            source code atau local storage.
          </p>
          <form onSubmit={login} className="login-form">
            <label>
              Nama operator
              <input
                value={actor}
                onChange={(event) => setActor(event.target.value)}
                placeholder="emisell"
                autoComplete="username"
              />
            </label>
            <label>
              Admin API key
              <input
                type="password"
                value={adminKey}
                onChange={(event) => setAdminKey(event.target.value)}
                placeholder="Masukkan credential admin"
                autoComplete="current-password"
              />
            </label>
            {error && <div className="alert alert-error">{error}</div>}
            <button className="button button-primary" disabled={loading}>
              {loading ? "Memeriksa…" : "Masuk dashboard"}
            </button>
          </form>
        </section>
      </main>
    );
  }

  const quotaPercentage =
    overview.quota_limit_today > 0
      ? Math.min(
          100,
          (overview.quota_used_today / overview.quota_limit_today) * 100,
        )
      : 0;

  const titles: Record<Tab, string> = {
    overview: "Dashboard",
    "check-rate": "Cek Ongkir",
    tracking: "Cek Resi",
	"tracking-operations": "Monitor Resi",
	"fulfillment-operations": "Monitor Fulfillment",
    couriers: "Ekspedisi & Service",
    rates: "Snapshot Tarif",
    mappings: "Mapping Lokasi Provider",
    providers: "Provider",
	"partner-submissions": "Partner Packages",
    quota: "Credential & Kuota",
    "api-keys": "API Key",
    webhook: "Webhook",
    documentation: "Dokumentasi API",
  };

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/" aria-label="API Kurir">
          <span className="brand-mark">EK</span>
          <span>
            <strong>API Kurir</strong>
            <small>Emisell Operations</small>
          </span>
        </a>
        <nav aria-label="Navigasi utama">
          <button
            className={`nav-dashboard ${tab === "overview" ? "nav-active" : ""}`}
            onClick={() => setTab("overview")}
            aria-current={tab === "overview" ? "page" : undefined}
          >
            <span className="nav-dot" />
            <span>Dashboard</span>
          </button>

          {NAV_GROUPS.map((group) => {
            const expanded = expandedNavGroup === group.id;
            const active = group.items.some((item) => item.value === tab);

            return (
              <div
                className={`nav-group ${expanded ? "nav-group-open" : ""}`}
                key={group.id}
              >
                <button
                  className={`nav-group-trigger ${
                    active ? "nav-group-trigger-active" : ""
                  }`}
                  onClick={() =>
                    setExpandedNavGroup((current) =>
                      current === group.id ? null : group.id,
                    )
                  }
                  aria-expanded={expanded}
                  aria-controls={`nav-group-${group.id}`}
                >
                  <span className="nav-dot" />
                  <span className="nav-group-copy">
                    <strong>{group.label}</strong>
                    <small>{group.description}</small>
                  </span>
                  <span className="nav-chevron" aria-hidden="true" />
                </button>

                {expanded && (
                  <div
                    className="nav-submenu"
                    id={`nav-group-${group.id}`}
                  >
                    {group.items.map((item, index) => {
                      const previousSection = group.items[index - 1]?.section;
                      const showSection =
                        item.section && item.section !== previousSection;

                      return (
                        <div className="nav-entry" key={item.value}>
                          {showSection && (
                            <div className="nav-subsection">
                              <span>{item.section}</span>
                              <small>Internal</small>
                            </div>
                          )}
                          <button
                            className={
                              tab === item.value ? "nav-active" : ""
                            }
                            onClick={() => setTab(item.value)}
                            aria-current={
                              tab === item.value ? "page" : undefined
                            }
                          >
                            <span className="nav-dot" />
                            {item.label}
                          </button>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </nav>
        <div className="sidebar-footer">
          <span>Operator</span>
          <strong>{actor || "emisell"}</strong>
          <button onClick={logout}>Keluar</button>
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">OPERASIONAL PENGIRIMAN</p>
            <h1>{titles[tab]}</h1>
          </div>
          <div className="topbar-actions">
            <span className="health">
              <span className="health-dot" />
              API siap
            </span>
            <button
              className="button button-secondary"
              onClick={() => void loadDashboard()}
              disabled={loading}
            >
              Muat ulang
            </button>
          </div>
        </header>

        {error && (
          <div className="alert alert-error">
            {error}
            <button onClick={() => setError("")}>Tutup</button>
          </div>
        )}

        {tab === "overview" && (
          <>
            <section className="metrics-grid">
              <Metric
                label="Master wilayah lokal"
                value={formatNumber(overview.official_locations)}
                hint="Data resmi hingga kelurahan"
              />
              <Metric
                label="Mapping provider legacy"
                value={formatNumber(overview.location_mappings)}
                hint="Dibentuk otomatis untuk RajaOngkir"
              />
              <Metric
                label="Snapshot tarif aktif"
                value={formatNumber(overview.fresh_quote_snapshots)}
                hint="Harga provider belum kedaluwarsa"
              />
              <Metric
                label="Antrean tracking"
                value={formatNumber(overview.tracking_pending_jobs)}
                hint={`${formatNumber(overview.tracking_shipments)} resi tersimpan`}
              />
            </section>

            <section className="panel overview-grid">
              <div>
                <div className="panel-heading">
                  <div>
                    <p className="eyebrow">PEMAKAIAN HARI INI</p>
                    <h2>Kuota RajaOngkir</h2>
                  </div>
                  <strong>{quotaPercentage.toFixed(1)}%</strong>
                </div>
                <div className="progress">
                  <span style={{ width: `${quotaPercentage}%` }} />
                </div>
                <div className="quota-labels">
                  <span>{formatNumber(overview.quota_used_today)} digunakan</span>
                  <span>
                    {formatNumber(overview.quota_limit_today)} batas tercatat
                  </span>
                </div>
              </div>
              <div className="operational-note">
                <span>Mode aktif</span>
                <strong>Otomatis, database-first</strong>
                <p>
                  Tidak ada input tarif atau mapping manual. Rute baru dicocokkan
                  ke provider saat cek ongkir pertama, kemudian hasilnya dipakai
                  ulang dari database selama masih berlaku.
                </p>
              </div>
            </section>

            <section className="panel">
              <div className="panel-heading">
                <div>
                  <p className="eyebrow">AKTIVITAS PROVIDER</p>
                  <h2>Snapshot tarif terbaru</h2>
                </div>
                <button
                  className="text-button"
                  onClick={() => setTab("rates")}
                >
                  Lihat semua
                </button>
              </div>
              <RateSnapshotTable items={snapshots.slice(0, 5)} />
            </section>
          </>
        )}

        {tab === "check-rate" && (
          <ShippingCostTool api={api} couriers={couriers} onError={setError} />
        )}

        {tab === "tracking" && (
          <TrackingTool api={api} couriers={couriers} onError={setError} />
        )}

		{tab === "tracking-operations" && (
		  <TrackingOperationsTable api={api} couriers={couriers} onError={setError} />
		)}

		{tab === "fulfillment-operations" && (
		  <FulfillmentOperationsTable api={api} onError={setError} />
		)}

        {tab === "couriers" && (
          <CourierCatalog
            couriers={couriers}
            onCheckRate={() => setTab("check-rate")}
          />
        )}

        {tab === "rates" && (
          <>
            <section className="toolbar">
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  void refreshRates();
                }}
                className="search-form"
              >
                <input
                  value={rateSearch}
                  onChange={(event) => setRateSearch(event.target.value)}
                  placeholder="Cari kurir, layanan, provider, atau wilayah…"
                />
                <button className="button button-secondary">Cari</button>
              </form>
              <span className="subtle">
                Read-only · tersimpan otomatis dari respons provider
              </span>
            </section>
            <section className="panel panel-table">
              <RateSnapshotTable items={snapshots} />
            </section>
          </>
        )}

        {tab === "mappings" && (
          <>
            <section className="toolbar">
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  void refreshMappings();
                }}
                className="search-form"
              >
                <input
                  value={mappingSearch}
                  onChange={(event) => setMappingSearch(event.target.value)}
                  placeholder="Cari wilayah atau ID provider…"
                />
                <button className="button button-secondary">Cari</button>
              </form>
              <span className="subtle">
                Read-only · dicocokkan otomatis saat cek ongkir pertama
              </span>
            </section>
            <section className="panel panel-table">
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Lokasi lokal</th>
                      <th>Provider</th>
                      <th>ID provider</th>
                      <th>Granularitas</th>
                      <th>Status otomatis</th>
                    </tr>
                  </thead>
                  <tbody>
                    {mappings.map((mapping) => (
                      <tr key={mapping.id}>
                        <td>
                          <strong>{mapping.location_label}</strong>
                          <small>{mapping.location_public_id}</small>
                        </td>
                        <td>
                          <span className="badge">{mapping.provider_code}</span>
                        </td>
                        <td>
                          <strong>{mapping.provider_location_id}</strong>
                          <small>{mapping.provider_location_name || "—"}</small>
                        </td>
                        <td>{mapping.granularity}</td>
                        <td>
                          <span
                            className={`badge ${
                              mapping.active
                                ? "badge-success"
                                : "badge-warning"
                            }`}
                          >
                            {mapping.active ? "aktif" : "nonaktif"}
                          </span>
                          <small>Diverifikasi {formatDate(mapping.verified_at)}</small>
                        </td>
                      </tr>
                    ))}
                    {!mappings.length && (
                      <tr>
                        <td colSpan={5} className="empty-state">
                          Belum ada mapping. Data akan muncul otomatis setelah
                          rute pertama digunakan untuk cek ongkir.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          </>
        )}

        {tab === "providers" && (
          <ProviderManagement
            api={api}
            items={shippingProviders}
            onChange={setShippingProviders}
            onError={setError}
          />
        )}

		{tab === "partner-submissions" && (
		  <PartnerSubmissionManagement
			api={api}
			onError={setError}
		  />
		)}

        {tab === "quota" && (
          <>
            <section className="provider-key-hero">
              <div>
                <p className="eyebrow">CREDENTIAL PROVIDER</p>
                <h2>Key platform dan seller</h2>
                <p>
                  Key platform ditambahkan operator di sini. Key milik seller
                  masuk melalui Emisell Gateway dan hanya dapat dipilih oleh
                  request merchant pemiliknya.
                </p>
              </div>
              <button
                className="button button-primary"
                onClick={() => setShowProviderKeyModal(true)}
              >
                Tambah key platform
              </button>
            </section>

            <section className="panel panel-table">
              <div className="panel-heading panel-padding">
                <div>
                  <p className="eyebrow">KEY TERSIMPAN</p>
                  <h2>Credential provider</h2>
                </div>
                <span className="subtle">
                  Secret asli tidak pernah ditampilkan kembali
                </span>
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Provider / key</th>
                      <th>Pemilik</th>
                      <th>Kuota harian</th>
                      <th>Validasi</th>
                      <th>Ditambahkan</th>
                      <th>Terakhir dipilih</th>
                      <th>Aksi</th>
                    </tr>
                  </thead>
                  <tbody>
                    {providerCredentials.map((credential) => (
                      <tr key={credential.id}>
                        <td>
                          <strong>{credential.provider_code}</strong>
                          <small className="api-key-mask">
                            {credential.display_key} ·{" "}
                            {credential.credential_alias}
                          </small>
                        </td>
                        <td>
                          {credential.tenant_id ? (
                            <>
                              <strong>Merchant</strong>
                              <small>{credential.tenant_id}</small>
                            </>
                          ) : (
                            <span className="badge">Platform</span>
                          )}
                        </td>
                        <td>{formatNumber(credential.daily_limit)} hit</td>
                        <td>
                          <span
                            className={`badge ${
                              credential.active &&
                              credential.validation_status === "valid"
                                ? "badge-success"
                                : "badge-warning"
                            }`}
                          >
                            {credential.active
                              ? credential.validation_status
                              : "nonaktif"}
                          </span>
                          <small>
                            Divalidasi{" "}
                            {formatDate(credential.last_validated_at)}
                          </small>
                        </td>
                        <td>{formatDate(credential.created_at)}</td>
                        <td>{formatDate(credential.last_selected_at)}</td>
                        <td>
                          {credential.active && (
                            <button
                              className="table-action table-action-danger"
                              disabled={providerKeyLoading}
                              onClick={() => {
                                if (
                                  window.confirm(
                                    `Nonaktifkan key ${credential.display_key}?`,
                                  )
                                ) {
                                  void disableProviderCredential(credential.id);
                                }
                              }}
                            >
                              Nonaktifkan
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                    {!providerCredentials.length && (
                      <tr>
                        <td colSpan={7} className="empty-state">
                          Belum ada key database. Tambahkan RajaOngkir untuk
                          ongkir/tracking utama atau Biteship sebagai fallback
                          internal Emisell Kurir.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </section>

            <section className="panel panel-table">
              <div className="panel-heading panel-padding">
                <div>
                  <p className="eyebrow">LEDGER LOKAL</p>
                  <h2>Pemakaian credential</h2>
                </div>
                <span className="subtle">Key tidak pernah ditampilkan</span>
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Tanggal</th>
                      <th>Provider / alias</th>
                      <th>Pemilik</th>
                      <th>Terpakai</th>
                      <th>Sisa</th>
                      <th>Penggunaan</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {quotas.map((quota) => (
                      <tr
                        key={`${quota.tenant_id || "platform"}-${quota.provider_code}-${quota.credential_alias}-${quota.quota_date}`}
                      >
                        <td>{quota.quota_date}</td>
                        <td>
                          <strong>{quota.provider_code}</strong>
                          <small>{quota.credential_alias}</small>
                        </td>
                        <td>{quota.tenant_id || "Platform"}</td>
                        <td>
                          {formatNumber(quota.used_count)} /{" "}
                          {formatNumber(quota.daily_limit)}
                        </td>
                        <td>{formatNumber(quota.remaining_count)}</td>
                        <td>
                          <div className="mini-progress">
                            <span
                              style={{
                                width: `${Math.min(100, quota.usage_percentage)}%`,
                              }}
                            />
                          </div>
                          <small>{quota.usage_percentage.toFixed(2)}%</small>
                        </td>
                        <td>
                          <span className="badge badge-success">
                            {quota.health_status}
                          </span>
                        </td>
                      </tr>
                    ))}
                    {!quotas.length && (
                      <tr>
                        <td colSpan={7} className="empty-state">
                          Belum ada hit provider yang tercatat.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </section>

            {showProviderKeyModal && (
              <div className="modal-backdrop">
                <section className="modal provider-key-modal">
                  <div className="modal-heading">
                    <div>
                      <p className="eyebrow">TAMBAH CREDENTIAL PLATFORM</p>
                      <h2>Credential provider internal</h2>
                    </div>
                    <button
                      onClick={() => {
                        setProviderKeySecret("");
                        setProviderKeyCode("rajaongkir");
                        setShowProviderKeyModal(false);
                      }}
                    >
                      Tutup
                    </button>
                  </div>
                  <form
                    className="form-stack"
                    onSubmit={addProviderCredential}
                  >
                    <label>
                      Provider
                      <select
                        value={providerKeyCode}
                        onChange={(event) => {
                          setProviderKeyCode(
                            event.target.value as "rajaongkir" | "biteship",
                          );
                          setProviderKeySecret("");
                        }}
                      >
                        <option value="rajaongkir">RajaOngkir</option>
                        <option value="biteship">
                          Biteship · fallback Emisell Kurir
                        </option>
                      </select>
                    </label>
                    <label>
                      API key provider
                      <input
                        type="password"
                        value={providerKeySecret}
                        onChange={(event) =>
                          setProviderKeySecret(event.target.value)
                        }
                        placeholder={
                          providerKeyCode === "biteship"
                            ? "Tempel token biteship_live atau biteship_test"
                            : "Tempel API key RajaOngkir"
                        }
                        autoComplete="new-password"
                        required
                      />
                    </label>
                    <div className="provider-key-note">
                      Key platform diuji langsung ke provider lalu disimpan
                      terenkripsi. Biteship hanya dipakai di balik Emisell
                      Kurir untuk fallback cek ongkir dan resi; seller yang
                      memakai RajaOngkir BYOK tidak memakai saldo ini.
                    </div>
                    <div className="form-actions">
                      <button
                        type="button"
                        className="button button-secondary"
                        onClick={() => {
                          setProviderKeySecret("");
                          setProviderKeyCode("rajaongkir");
                          setShowProviderKeyModal(false);
                        }}
                      >
                        Batal
                      </button>
                      <button
                        className="button button-primary"
                        disabled={
                          providerKeyLoading || !providerKeySecret.trim()
                        }
                      >
                        {providerKeyLoading
                          ? "Memvalidasi…"
                          : "Validasi & simpan"}
                      </button>
                    </div>
                  </form>
                </section>
              </div>
            )}
          </>
        )}

        {tab === "api-keys" && (
          <APIKeyManagement
            items={apiKeys}
            generated={generatedAPIKey}
            loading={keyActionLoading}
            onGenerate={generateAPIKey}
            onRevoke={revokeAPIKey}
            onDismissGenerated={() => setGeneratedAPIKey(null)}
          />
        )}

        {tab === "webhook" && (
          <WebhookManagement
            settings={webhookSettings}
            generatedSecret={generatedWebhookSecret}
            testResult={webhookTestResult}
            loading={webhookActionLoading}
            onSave={saveWebhook}
            onGenerateSecret={generateWebhookSecret}
            onDismissSecret={() => setGeneratedWebhookSecret("")}
            onTest={testWebhook}
          />
        )}

        {tab === "documentation" && (
          <ApiDocumentation
            view={documentationView}
            onViewChange={setDocumentationView}
          />
        )}
      </section>
    </main>
  );
}

type ProviderDraft = {
  code: string;
  name: string;
  logo: string;
  description: string;
  integration_type: "built_in" | "managed_upstream" | "partner_hosted";
  credential_type: ProviderCredentialType;
  available: boolean;
  display_order: number;
};

const EMPTY_PROVIDER_DRAFT: ProviderDraft = {
  code: "",
  name: "",
  logo: "https://api-kurir.emisell.com/provider-logos/default.svg",
  description: "",
  integration_type: "partner_hosted",
  credential_type: "none",
  available: false,
  display_order: 100,
};

function credentialTypeLabel(value: ProviderCredentialType) {
  switch (value) {
    case "api_key": return "API key";
    case "capability_api_keys": return "API key per fungsi";
    case "bearer_token": return "Bearer token";
    case "api_key_secret": return "API key + API secret";
    case "oauth2_client_credentials": return "OAuth client credentials";
    default: return "Dikelola platform";
  }
}

function credentialFieldPreview(value: ProviderCredentialType) {
  switch (value) {
    case "api_key": return "API key";
    case "capability_api_keys": return "Shipping API key · Delivery API key (opsional)";
    case "bearer_token": return "Access token";
    case "api_key_secret": return "API key · API secret";
    case "oauth2_client_credentials": return "Client ID · Client secret";
    default: return "Tidak ada field credential seller";
  }
}

function ProviderLogo({
  source,
  code,
  alt,
}: {
  source: string;
  code: string;
  alt: string;
}) {
  const fallbackSources = [
    source,
    `/provider-logos/${code}.svg`,
    "/provider-logos/default.svg",
  ].filter((item, index, items) => item && items.indexOf(item) === index);
  const [sourceIndex, setSourceIndex] = useState(0);

  useEffect(() => setSourceIndex(0), [source, code]);

  return (
    <img
      src={fallbackSources[sourceIndex]}
      alt={alt}
      onError={() =>
        setSourceIndex((current) =>
          Math.min(current + 1, fallbackSources.length - 1),
        )
      }
    />
  );
}

function CourierLogo({
  source,
  code,
  alt,
}: {
  source: string;
  code: string;
  alt: string;
}) {
  const bundledSource = `/courier-logos/${code === "rex" ? "rex.png" : `${code}.webp`}`;
  const fallbackSources = [
    bundledSource,
    source,
    "/courier-logos/default.svg",
  ].filter((item, index, items) => item && items.indexOf(item) === index);
  const [sourceIndex, setSourceIndex] = useState(0);

  useEffect(() => setSourceIndex(0), [source, code]);

  return (
    <img
      src={fallbackSources[sourceIndex]}
      alt={alt}
      loading="lazy"
      onError={() =>
        setSourceIndex((current) =>
          Math.min(current + 1, fallbackSources.length - 1),
        )
      }
    />
  );
}

function ProviderManagement({
  api,
  items,
  onChange,
  onError,
}: {
  api: AdminApi;
  items: ShippingProvider[];
  onChange: (items: ShippingProvider[]) => void;
  onError: (message: string) => void;
}) {
  const [draft, setDraft] = useState<ProviderDraft | null>(null);
  const [editingCode, setEditingCode] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [accessProvider, setAccessProvider] = useState<ShippingProvider | null>(null);
  const [accessKeys, setAccessKeys] = useState<PartnerAccessKey[]>([]);
  const [generatedAccessSecret, setGeneratedAccessSecret] = useState("");
  const [accessLoading, setAccessLoading] = useState(false);
  const [deletingCode, setDeletingCode] = useState<string | null>(null);

  function openCreate() {
    setEditingCode(null);
    setDraft({ ...EMPTY_PROVIDER_DRAFT });
  }

  function openEdit(provider: ShippingProvider) {
    setEditingCode(provider.code);
    setDraft({
      code: provider.code,
      name: provider.name,
      logo: provider.logo,
      description: provider.description,
      integration_type: provider.integration_type,
      credential_type: provider.credential_type,
      available: provider.available,
      display_order: provider.display_order,
    });
  }

  function closeEditor() {
    if (saving) return;
    setDraft(null);
    setEditingCode(null);
  }

  async function openPartnerAccess(provider: ShippingProvider) {
    setAccessProvider(provider);
    setGeneratedAccessSecret("");
    setAccessLoading(true);
    onError("");
    try {
      setAccessKeys(await api.partnerAccessKeys(provider.code));
    } catch (accessError) {
      onError(getErrorMessage(accessError));
      setAccessProvider(null);
    } finally {
      setAccessLoading(false);
    }
  }

  async function generatePartnerAccess() {
    if (!accessProvider) return;
    setAccessLoading(true);
    onError("");
    try {
      const generated = await api.generatePartnerAccessKey(accessProvider.code);
      setGeneratedAccessSecret(generated.secret);
      setAccessKeys(await api.partnerAccessKeys(accessProvider.code));
    } catch (accessError) {
      onError(getErrorMessage(accessError));
    } finally {
      setAccessLoading(false);
    }
  }

  async function revokePartnerAccess(id: string) {
    if (!accessProvider) return;
    setAccessLoading(true);
    onError("");
    try {
      await api.revokePartnerAccessKey(accessProvider.code, id);
      setAccessKeys(await api.partnerAccessKeys(accessProvider.code));
    } catch (accessError) {
      onError(getErrorMessage(accessError));
    } finally {
      setAccessLoading(false);
    }
  }

  async function deleteProvider(provider: ShippingProvider) {
    if (provider.built_in) return;
    const confirmed = window.confirm(
      `Hapus provider ${provider.name} secara permanen? ${provider.release_count > 0 ? `${provider.release_count} package pengujian, Partner Access Key, dan credential Explorer ikut dihapus. ` : ""}Tindakan ini tidak dapat dibatalkan.`,
    );
    if (!confirmed) return;
    setDeletingCode(provider.code);
    onError("");
    try {
      await api.deleteShippingProvider(provider.code);
      onChange(await api.shippingProviders());
    } catch (deleteError) {
      onError(getErrorMessage(deleteError));
    } finally {
      setDeletingCode(null);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!draft) return;
    setSaving(true);
    onError("");
    try {
      if (editingCode) {
        const input: ShippingProviderUpdateInput = {
          name: draft.name.trim(),
          logo: draft.logo.trim(),
          description: draft.description.trim(),
          integration_type: draft.integration_type,
          credential_type: draft.credential_type,
          available: draft.available,
          display_order: draft.display_order,
        };
        await api.updateShippingProvider(editingCode, input);
      } else {
        const input: ShippingProviderCreateInput = {
          code: draft.code.trim().toLowerCase(),
          name: draft.name.trim(),
          logo: draft.logo.trim(),
          description: draft.description.trim(),
          integration_type: draft.integration_type === "built_in" ? "partner_hosted" : draft.integration_type,
          credential_type: draft.credential_type,
          display_order: draft.display_order,
        };
        await api.createShippingProvider(input);
      }
      onChange(await api.shippingProviders());
      setDraft(null);
      setEditingCode(null);
    } catch (saveError) {
      onError(getErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  }

  const availableCount = items.filter((provider) => provider.available).length;
  const activeMerchantCount = items.reduce(
    (total, provider) => total + provider.active_merchant_count,
    0,
  );

  return (
    <>
      <section className="provider-key-hero provider-management-hero">
        <div>
          <p className="eyebrow">MASTER INTEGRASI</p>
          <h2>Provider terintegrasi</h2>
          <p>
            Kelola identitas provider yang tampil di extension Emisell. Provider
            baru selalu dibuat belum tersedia sampai adapter dan credential-nya
            siap diuji.
          </p>
        </div>
        <button className="button button-primary" onClick={openCreate}>
          Tambah provider
        </button>
      </section>

      <section className="provider-management-summary">
        <article>
          <span>Total provider</span>
          <strong>{formatNumber(items.length)}</strong>
        </article>
        <article>
          <span>Siap digunakan</span>
          <strong>{formatNumber(availableCount)}</strong>
        </article>
        <article>
          <span>Merchant aktif</span>
          <strong>{formatNumber(activeMerchantCount)}</strong>
        </article>
      </section>

      <section className="panel panel-table provider-management-table">
        <div className="panel-heading panel-padding">
          <div>
            <p className="eyebrow">KATALOG PROVIDER</p>
            <h2>Provider yang masuk ke API Kurir</h2>
          </div>
          <span className="subtle">Kode dan model credential tidak dapat diubah</span>
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Provider</th>
                <th>Deskripsi</th>
                <th>Status</th>
                <th>Penggunaan</th>
                <th>Urutan</th>
                <th>Aksi</th>
              </tr>
            </thead>
            <tbody>
              {items.map((provider) => (
                <tr key={provider.code}>
                  <td>
                    <div className="provider-identity">
                      <ProviderLogo
                        source={provider.logo}
                        code={provider.code}
                        alt={`Logo ${provider.name}`}
                      />
                      <div>
                        <strong>{provider.name}</strong>
                        <small>{provider.code}</small>
                      </div>
                    </div>
                  </td>
                  <td className="provider-description-cell">
                    {provider.description}
                  </td>
                  <td>
                    <span
                      className={`badge ${
                        provider.available ? "badge-success" : "badge-warning"
                      }`}
                    >
                      {provider.available ? "tersedia" : "belum tersedia"}
                    </span>
                    <small>
                      {provider.integration_type.replaceAll("_", " ")}
                    </small>
                    <small>Credential: {credentialTypeLabel(provider.credential_type)}</small>
                    <small>
                      {provider.active_release_version
                        ? `Release aktif v${provider.active_release_version}`
                        : provider.integration_type === "partner_hosted"
                          ? "Belum ada release aktif"
                          : provider.requires_credential
                            ? "Credential seller"
                            : "Dikelola platform"}
                    </small>
                  </td>
                  <td>
                    <strong>
                      {formatNumber(provider.active_merchant_count)} merchant aktif
                    </strong>
                    <small>
                      {formatNumber(provider.installed_merchant_count)} terpasang ·{" "}
                      {formatNumber(provider.credential_count)} credential
                    </small>
                  </td>
                  <td>
                    <strong>{provider.display_order}</strong>
                    <small>Diperbarui {formatDate(provider.updated_at)}</small>
                  </td>
                  <td>
                    <div className="provider-row-actions">
                      {provider.integration_type === "partner_hosted" && (
                        <button
                          className="table-action"
                          onClick={() => void openPartnerAccess(provider)}
                        >
                          Akses partner
                        </button>
                      )}
                      <button
                        className="table-action provider-edit-action"
                        onClick={() => openEdit(provider)}
                      >
                        Edit
                      </button>
                      {!provider.built_in && (
                        <button
                          className="table-action table-action-danger"
                          disabled={
                            deletingCode === provider.code ||
                            provider.active_merchant_count > 0 ||
                            provider.installed_merchant_count > 0 ||
                            provider.credential_count > 0 ||
                            provider.active_release_id !== null
                          }
                          title={
                            provider.active_merchant_count > 0 ||
                            provider.installed_merchant_count > 0 ||
                            provider.credential_count > 0 ||
                            provider.active_release_id !== null
                              ? "Provider masih aktif atau mempunyai data merchant."
                              : provider.release_count > 0
                                ? "Hapus provider beserta seluruh artefak pengujian"
                                : "Hapus provider secara permanen"
                          }
                          onClick={() => void deleteProvider(provider)}
                        >
                          {deletingCode === provider.code ? "Menghapus…" : "Hapus"}
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
              {!items.length && (
                <tr>
                  <td colSpan={6} className="empty-state">
                    Belum ada provider pada master integrasi.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      {draft && (
        <div className="modal-backdrop">
          <section className="modal modal-wide provider-management-modal">
            <div className="modal-heading">
              <div>
                <p className="eyebrow">
                  {editingCode ? "EDIT PROVIDER" : "TAMBAH PROVIDER"}
                </p>
                <h2>{editingCode ? draft.name : "Provider baru"}</h2>
              </div>
              <button onClick={closeEditor}>Tutup</button>
            </div>
            <form className="form-grid" onSubmit={submit}>
              <label>
                Kode provider
                <input
                  value={draft.code}
                  disabled={Boolean(editingCode)}
                  required
                  minLength={2}
                  maxLength={48}
                  pattern="[a-z0-9_-]+"
                  placeholder="mengantar"
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      code: event.target.value.toLowerCase().replace(/\s+/g, "-"),
                    })
                  }
                />
                <small>Huruf kecil, angka, tanda hubung, atau garis bawah.</small>
              </label>
              <label>
                Urutan tampil
                <input
                  type="number"
                  min={1}
                  max={9999}
                  required
                  value={draft.display_order}
                  onChange={(event) =>
                    setDraft({ ...draft, display_order: Number(event.target.value) })
                  }
                />
              </label>
              <label>
                Jenis integrasi
                <select
                  value={draft.integration_type}
                  disabled={draft.integration_type === "built_in"}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      integration_type: event.target.value as ProviderDraft["integration_type"],
                      credential_type:
                        event.target.value === "managed_upstream"
                          ? draft.credential_type === "none" ? "api_key" : draft.credential_type
                          : event.target.value === "built_in" ? "none" : draft.credential_type,
                    })
                  }
                >
                  {draft.integration_type === "built_in" && <option value="built_in">Built-in Emisell</option>}
                  <option value="managed_upstream">Managed upstream</option>
                  <option value="partner_hosted">Partner-hosted</option>
                </select>
                <small>Managed upstream memakai adapter API Kurir; partner-hosted memakai package tersertifikasi.</small>
              </label>
              <label>
                Model credential seller
                <select
                  value={draft.credential_type}
                  disabled={draft.integration_type === "built_in"}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      credential_type: event.target.value as ProviderCredentialType,
                    })
                  }
                >
                  <option value="none">Tidak ada credential seller</option>
                  <option value="api_key">Satu API key</option>
                  <option value="capability_api_keys">API key shipping + delivery</option>
                  <option value="bearer_token">Bearer token</option>
                  <option value="api_key_secret">API key + API secret</option>
                  <option value="oauth2_client_credentials">OAuth client ID + client secret</option>
                </select>
                <small>Emisell merender field aktivasi merchant berdasarkan model ini.</small>
              </label>
              {draft.integration_type !== "built_in" && draft.credential_type !== "none" && (
                <div className="provider-key-note">
                  <strong>Preview form merchant</strong>
                  <span>{credentialFieldPreview(draft.credential_type)}</span>
                </div>
              )}
              <label className="span-two">
                Nama provider
                <input
                  value={draft.name}
                  required
                  minLength={2}
                  maxLength={100}
                  placeholder="Mengantar"
                  onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                />
              </label>
              <label className="span-two">
                URL logo HTTPS
                <input
                  type="url"
                  value={draft.logo}
                  required
                  placeholder="https://api-kurir.emisell.com/provider-logos/provider.svg"
                  onChange={(event) => setDraft({ ...draft, logo: event.target.value })}
                />
              </label>
              <label className="span-two">
                Deskripsi
                <textarea
                  value={draft.description}
                  required
                  minLength={10}
                  maxLength={500}
                  rows={4}
                  placeholder="Jelaskan fungsi provider untuk seller Emisell."
                  onChange={(event) =>
                    setDraft({ ...draft, description: event.target.value })
                  }
                />
                <small>{draft.description.length}/500 karakter · teks tanpa HTML</small>
              </label>
              <div className="span-two provider-logo-preview">
                <span>Preview logo</span>
                <ProviderLogo
                  source={draft.logo}
                  code={draft.code || "default"}
                  alt="Preview logo provider"
                />
              </div>
              {editingCode ? (
                <label className="span-two provider-availability-control">
                  <input
                    type="checkbox"
                    checked={draft.available}
                    onChange={(event) =>
                      setDraft({ ...draft, available: event.target.checked })
                    }
                  />
                  <span>
                    <strong>Provider tersedia untuk aktivasi merchant</strong>
                    <small>
                      Aktifkan hanya setelah Partner Connector, sertifikasi, dan validasi credential siap.
                      Provider dengan merchant aktif tidak dapat dinonaktifkan langsung.
                    </small>
                  </span>
                </label>
              ) : (
                <div className="span-two provider-key-note">
                  Provider baru dibuat belum tersedia. Partner-hosted wajib memiliki
                  release published; managed upstream wajib memiliki adapter dan
                  credential seller yang valid sebelum diaktifkan.
                </div>
              )}
              <div className="form-actions span-two">
                <button
                  type="button"
                  className="button button-secondary"
                  onClick={closeEditor}
                  disabled={saving}
                >
                  Batal
                </button>
                <button className="button button-primary" disabled={saving}>
                  {saving ? "Menyimpan…" : editingCode ? "Simpan perubahan" : "Tambah provider"}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}

      {accessProvider && (
        <div className="modal-backdrop">
          <section className="modal modal-wide partner-access-modal">
            <div className="modal-heading">
              <div>
                <p className="eyebrow">PARTNER PORTAL ACCESS</p>
                <h2>{accessProvider.name}</h2>
                <small>{accessProvider.code} · provider dikunci oleh backend</small>
              </div>
              <button
                onClick={() => {
                  if (accessLoading) return;
                  setAccessProvider(null);
                  setGeneratedAccessSecret("");
                }}
              >
                Tutup
              </button>
            </div>

            <div className="partner-access-intro">
              <p>
                Berikan access key ini hanya kepada partner terkait. Saat login,
                Partner Portal otomatis mengenali provider ini sehingga tidak ada
                dropdown dan package tidak dapat dikirim atas nama provider lain.
              </p>
              <a className="button button-secondary" href="/partner" target="_blank" rel="noreferrer">
                Buka Partner Portal
              </a>
            </div>

            {generatedAccessSecret && (
              <div className="webhook-generated-secret partner-access-secret">
                <strong>Salin sekarang — key hanya ditampilkan sekali</strong>
                <code>{generatedAccessSecret}</code>
                <button
                  type="button"
                  className="button button-secondary"
                  onClick={() => void navigator.clipboard.writeText(generatedAccessSecret)}
                >
                  Salin key
                </button>
              </div>
            )}

            <div className="panel-heading">
              <div>
                <p className="eyebrow">ACCESS KEY</p>
                <h3>Credential milik {accessProvider.name}</h3>
              </div>
              <button
                className="button button-primary"
                onClick={() => void generatePartnerAccess()}
                disabled={accessLoading}
              >
                {accessLoading ? "Memproses…" : "Generate access key"}
              </button>
            </div>

            <div className="table-scroll">
              <table>
                <thead>
                  <tr><th>Key</th><th>Status</th><th>Dibuat</th><th>Terakhir dipakai</th><th>Aksi</th></tr>
                </thead>
                <tbody>
                  {accessKeys.map((item) => (
                    <tr key={item.id}>
                      <td><code>{item.display_key}</code></td>
                      <td><span className={`badge ${item.active ? "badge-success" : "badge-warning"}`}>{item.active ? "aktif" : "dicabut"}</span></td>
                      <td><strong>{item.created_by}</strong><small>{formatDate(item.created_at)}</small></td>
                      <td>{formatDate(item.last_used_at)}</td>
                      <td>
                        {item.active ? (
                          <button className="table-action table-action-danger" onClick={() => void revokePartnerAccess(item.id)} disabled={accessLoading}>
                            Cabut
                          </button>
                        ) : "—"}
                      </td>
                    </tr>
                  ))}
                  {!accessKeys.length && (
                    <tr><td colSpan={5} className="empty-state">Belum ada access key untuk provider ini.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>
        </div>
      )}

    </>
  );
}

function PartnerPortal() {
  const [accessKey, setAccessKey] = useState(
    () => sessionStorage.getItem("api-kurir-partner-key") ?? "",
  );
  const [accessKeyInput, setAccessKeyInput] = useState(accessKey);
  const [identity, setIdentity] = useState<PartnerIdentity | null>(null);
  const [items, setItems] = useState<PartnerSubmission[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [version, setVersion] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(Boolean(accessKey));
  const [uploading, setUploading] = useState(false);
  const [starterDownloading, setStarterDownloading] = useState(false);
  const [error, setError] = useState("");

  const api = useMemo(() => new PartnerPortalApi(accessKey), [accessKey]);

  const loadPortal = useCallback(async (signal?: AbortSignal) => {
    if (!accessKey) return;
    setLoading(true);
    setError("");
    try {
		const [partnerIdentity, submissions] = await Promise.all([
        api.me(signal),
        api.submissions("", signal),
      ]);
      setIdentity(partnerIdentity);
      setItems(submissions);
      setSelectedID((current) =>
        current && submissions.some((item) => item.id === current)
          ? current
          : submissions[0]?.id ?? "",
      );
    } catch (portalError) {
      if (portalError instanceof Error && portalError.name === "AbortError") return;
      if (isUnauthorized(portalError)) {
        sessionStorage.removeItem("api-kurir-partner-key");
        setAccessKey("");
        setAccessKeyInput("");
        setIdentity(null);
      }
      setError(getErrorMessage(portalError));
    } finally {
      setLoading(false);
    }
  }, [accessKey, api]);

  useEffect(() => {
    if (!accessKey) return;
    const controller = new AbortController();
    void loadPortal(controller.signal);
    return () => controller.abort();
  }, [accessKey, loadPortal]);

  function login(event: FormEvent) {
    event.preventDefault();
    const normalized = accessKeyInput.trim();
    if (!normalized) return;
    sessionStorage.setItem("api-kurir-partner-key", normalized);
    setAccessKey(normalized);
  }

  function logout() {
    sessionStorage.removeItem("api-kurir-partner-key");
    setAccessKey("");
    setAccessKeyInput("");
    setIdentity(null);
    setItems([]);
    setSelectedID("");
    setError("");
  }

  async function upload(event: FormEvent) {
    event.preventDefault();
    if (!file || !version.trim()) return;
    setUploading(true);
    setError("");
    try {
      const created = await api.uploadSubmission(version.trim(), file);
      setVersion("");
      setFile(null);
      const input = document.getElementById("partner-portal-package") as HTMLInputElement | null;
      if (input) input.value = "";
      const submissions = await api.submissions();
      setItems(submissions);
      setSelectedID(created.id);
    } catch (uploadError) {
      setError(getErrorMessage(uploadError));
    } finally {
      setUploading(false);
    }
  }

  async function download(item: PartnerSubmission) {
    setError("");
    try {
      const blob = await api.downloadArtifact(item.id);
      const source = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = source;
      anchor.download = item.file_name;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(source);
    } catch (downloadError) {
      setError(getErrorMessage(downloadError));
    }
  }

  async function downloadStarterPackage() {
    if (!identity) return;
    setStarterDownloading(true);
    setError("");
    try {
      const blob = await api.downloadStarterPackage();
      const source = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = source;
      anchor.download = `${identity.provider_code}-partner-starter.zip`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(source);
    } catch (downloadError) {
      setError(getErrorMessage(downloadError));
    } finally {
      setStarterDownloading(false);
    }
  }

  if (!identity) {
    return (
      <main className="login-shell partner-login-shell">
        <section className="login-card partner-login-card">
          <div className="brand-mark">EP</div>
          <p className="eyebrow">EMISELL PARTNER PORTAL</p>
          <h1>Ajukan connector kurir</h1>
          <p>
            Gunakan access key yang diterbitkan Emisell. Provider akan dikenali
            otomatis dan tidak dapat dipilih atau diganti dari browser.
          </p>
          <form onSubmit={login} className="login-form">
            <label>
              Partner access key
              <input
                type="password"
                value={accessKeyInput}
                onChange={(event) => setAccessKeyInput(event.target.value)}
                placeholder="epk_live_…"
                autoComplete="current-password"
                required
              />
            </label>
            {error && <div className="alert alert-error">{error}</div>}
            <button className="button button-primary" disabled={loading}>
              {loading ? "Memeriksa…" : "Masuk Partner Portal"}
            </button>
          </form>
          <a className="partner-staff-link" href="/">Kembali ke API Kurir Admin</a>
        </section>
      </main>
    );
  }

  const selected = items.find((item) => item.id === selectedID) ?? null;

  return (
    <main className="partner-portal-shell">
      <header className="partner-portal-topbar">
        <a className="brand" href="/partner" aria-label="Emisell Partner Portal">
          <span className="brand-mark">EP</span>
          <span><strong>Partner Portal</strong><small>Emisell Logistics Platform</small></span>
        </a>
        <nav className="partner-portal-nav" aria-label="Navigasi Partner Portal">
          <a href="#partner-package">Package</a>
          <a href="#partner-api-explorer">API Explorer</a>
          <a href="#partner-integration-docs">Dokumentasi</a>
          <a href="/openapi/api-kurir-partner-v1.yaml" target="_blank" rel="noreferrer">
            OpenAPI
          </a>
        </nav>
        <div className="partner-portal-identity">
          <span>Provider terautentikasi</span>
          <strong>{identity.provider_name}</strong>
          <code>{identity.provider_code}</code>
          <button onClick={logout}>Keluar</button>
        </div>
      </header>

      <section className="partner-portal-workspace">
        <section className="provider-key-hero partner-portal-hero">
          <div>
            <p className="eyebrow">PACKAGE CERTIFICATION</p>
            <h1>Integrasi {identity.provider_name}</h1>
            <p>
              Upload versi connector untuk scan otomatis, pengujian kontrak,
              review keamanan, dan UAT. Seluruh submission pada halaman ini terkunci
              untuk provider <strong>{identity.provider_code}</strong>.
            </p>
          </div>
          <span className="badge badge-success">Identitas terkunci</span>
        </section>

        {error && <div className="alert alert-error">{error}</div>}

        <section className="partner-portal-grid" id="partner-package">
          <form className="panel partner-package-upload" onSubmit={upload}>
            <div className="panel-heading">
              <div><p className="eyebrow">NEW SUBMISSION</p><h2>Upload package ZIP</h2></div>
              <span className="subtle">Maks. 25 MB</span>
            </div>
            <div className="partner-provider-lock">
              <span>Provider</span>
              <strong>{identity.provider_name}</strong>
              <code>{identity.provider_code}</code>
            </div>
            <label>
              Versi connector
              <input
                value={version}
                onChange={(event) => setVersion(event.target.value)}
                placeholder="1.0.0"
                maxLength={64}
                pattern="[A-Za-z0-9][A-Za-z0-9._-]*"
                required
              />
            </label>
            <label>
              File package
              <input
                id="partner-portal-package"
                type="file"
                accept=".zip,application/zip"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                required
              />
            </label>
            <div className="partner-package-requirements">
              <strong>Wajib pada root ZIP</strong>
              <span>emisell-extension.yaml</span>
              <span>openapi.yaml</span>
              <small>10 percobaan/jam · 1 upload aktif · 25 versi atau 500 MB/provider</small>
              <a href="#partner-developer-preparation">Lihat checklist developer lengkap →</a>
            </div>
            <button className="button button-primary" disabled={uploading}>
              {uploading ? "Memvalidasi package…" : "Upload & validasi"}
            </button>
          </form>

          <section className="panel partner-portal-guidance">
            <p className="eyebrow">ALUR SERTIFIKASI</p>
            <h2>Dari scan hingga published</h2>
            <ol>
              <li><strong>Static scan</strong><span>Format ZIP, manifest, OpenAPI, dan secret diperiksa.</span></li>
              <li><strong>Pengujian & keamanan</strong><span>Tim Emisell menguji kontrak tanpa menjalankan source di API Kurir.</span></li>
              <li><strong>UAT</strong><span>Skenario tarif, order, pickup, dan tracking diverifikasi.</span></li>
              <li><strong>Published</strong><span>Hanya versi yang lulus dapat diaktifkan ke merchant.</span></li>
            </ol>
          </section>
        </section>

        <PartnerVisualApiExplorer
          api={api}
          identity={identity}
          submissions={items}
          onError={setError}
        />

        <PartnerPortalIntegrationDocumentation
          providerCode={identity.provider_code}
          providerName={identity.provider_name}
          starterDownloading={starterDownloading}
          onDownloadStarter={() => void downloadStarterPackage()}
        />

        <section className="panel panel-table partner-package-table">
          <div className="panel-heading panel-padding">
            <div><p className="eyebrow">SUBMISSION HISTORY</p><h2>Versi milik {identity.provider_name}</h2></div>
            <button className="button button-secondary" onClick={() => void loadPortal()} disabled={loading}>
              {loading ? "Memuat…" : "Muat ulang"}
            </button>
          </div>
          <div className="table-scroll">
            <table>
              <thead><tr><th>Versi</th><th>Package</th><th>Scan</th><th>Status</th><th>Diajukan</th><th>Aksi</th></tr></thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id} className={selectedID === item.id ? "table-row-selected" : ""}>
                    <td>
                      <strong>v{item.version}</strong>
                      <small>{item.provider_code}</small>
                      {item.is_active_release && <span className="badge badge-success">release aktif</span>}
                    </td>
                    <td><strong>{item.file_name}</strong><small>{formatFileSize(item.artifact_size)}</small></td>
                    <td><span className={`badge ${item.scan_report.passed ? "badge-success" : "badge-danger"}`}>{item.scan_report.passed ? "lulus" : "gagal"}</span></td>
                    <td><span className={`badge ${partnerStatusTone(item.status)}`}>{PARTNER_SUBMISSION_STATUS_LABELS[item.status]}</span></td>
                    <td>{formatDate(item.created_at)}</td>
                    <td><button className="table-action" onClick={() => setSelectedID(item.id)}>Detail</button></td>
                  </tr>
                ))}
                {!items.length && <tr><td colSpan={6} className="empty-state">Belum ada package yang diajukan.</td></tr>}
              </tbody>
            </table>
          </div>
        </section>

        {selected && (
          <section className="panel partner-package-review partner-portal-result">
            <div className="panel-heading">
              <div><p className="eyebrow">HASIL &amp; FEEDBACK</p><h2>v{selected.version} · {PARTNER_SUBMISSION_STATUS_LABELS[selected.status]}</h2></div>
              <button className="button button-secondary" onClick={() => void download(selected)}>Unduh ZIP</button>
            </div>
            {selected.review_note && <div className="alert alert-warning"><strong>Catatan reviewer</strong><p>{selected.review_note}</p></div>}
            <div className="partner-package-review-grid">
              <div className="partner-package-checks">
                {selected.scan_report.checks.map((check) => (
                  <article key={check.code}>
                    <span className={`badge ${check.status === "passed" ? "badge-success" : "badge-danger"}`}>{check.status}</span>
                    <div><strong>{check.code.replaceAll("_", " ")}</strong><p>{check.message}</p></div>
                  </article>
                ))}
              </div>
              <div className="partner-package-manifest">
                <h3>Manifest terdeteksi</h3>
                <dl>
                  <div><dt>Contract</dt><dd>{selected.scan_report.manifest.contract_version || "—"}</dd></div>
                  <div><dt>Endpoint</dt><dd>{selected.scan_report.manifest.base_url || "—"}</dd></div>
                  <div><dt>Capability</dt><dd>{selected.scan_report.manifest.declared_capabilities.join(", ") || "—"}</dd></div>
                  <div><dt>Scope runtime</dt><dd>{selected.required_scopes.join(", ") || "Tidak ada"}</dd></div>
                  <div><dt>Service</dt><dd>{selected.scan_report.manifest.declared_services.join(", ") || "—"}</dd></div>
                </dl>
              </div>
            </div>
          </section>
        )}
      </section>
    </main>
  );
}

type PartnerExplorerCodeLanguage = "curl" | "typescript" | "php" | "go";

function PartnerVisualApiExplorer({
  api,
  identity,
  submissions,
  onError,
}: {
  api: PartnerPortalApi;
  identity: PartnerIdentity;
  submissions: PartnerSubmission[];
  onError: (message: string) => void;
}) {
  const eligibleSubmissions = useMemo(
    () => submissions.filter((item) => item.scan_report.passed),
    [submissions],
  );
  const [submissionID, setSubmissionID] = useState(eligibleSubmissions[0]?.id ?? "");
  const [catalog, setCatalog] = useState<PartnerExplorerCatalog | null>(null);
  const [credentials, setCredentials] = useState<PartnerExplorerCredentialState[]>([]);
  const [credentialInputs, setCredentialInputs] = useState<Record<string, string>>({});
  const [capability, setCapability] = useState("all");
  const [operationID, setOperationID] = useState("");
  const [parameterValues, setParameterValues] = useState<Record<string, string>>({});
  const [requestBody, setRequestBody] = useState("");
  const [result, setResult] = useState<PartnerExplorerExecution | null>(null);
  const [runs, setRuns] = useState<PartnerExplorerRun[]>([]);
  const [codeLanguage, setCodeLanguage] = useState<PartnerExplorerCodeLanguage>("curl");
  const [loading, setLoading] = useState(false);
  const [savingCredential, setSavingCredential] = useState(false);
  const [executing, setExecuting] = useState(false);

  useEffect(() => {
    if (submissionID && eligibleSubmissions.some((item) => item.id === submissionID)) return;
    setSubmissionID(eligibleSubmissions[0]?.id ?? "");
  }, [eligibleSubmissions, submissionID]);

  useEffect(() => {
    if (!submissionID) {
      setCatalog(null);
      setRuns([]);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    onError("");
    void Promise.all([
      api.explorerCatalog(submissionID, controller.signal),
      api.explorerRuns(submissionID, controller.signal),
    ]).then(([nextCatalog, nextRuns]) => {
      setCatalog(nextCatalog);
      setCredentials(nextCatalog.credentials ?? []);
      setRuns(nextRuns);
      setCapability("all");
      const nextOperation =
        nextCatalog.operations.find((operation) => operation.safety === "read_only") ??
        nextCatalog.operations[0];
      setOperationID(nextOperation?.id ?? "");
      setParameterValues(parameterDefaults(nextOperation));
      setRequestBody(formatExplorerRequestExample(nextOperation));
      setResult(null);
    }).catch((loadError) => {
      if (loadError instanceof Error && loadError.name === "AbortError") return;
      onError(getErrorMessage(loadError));
    }).finally(() => setLoading(false));
    return () => controller.abort();
  }, [api, onError, submissionID]);

  const capabilities = useMemo(() => {
    const values = new Set(catalog?.operations.map((operation) => operation.capability) ?? []);
    return ["all", ...Array.from(values).sort()];
  }, [catalog]);
  const visibleOperations = useMemo(
    () => catalog?.operations.filter(
      (operation) => capability === "all" || operation.capability === capability,
    ) ?? [],
    [capability, catalog],
  );
  const selectedOperation =
    catalog?.operations.find((operation) => operation.id === operationID) ?? null;

  useEffect(() => {
    if (!visibleOperations.length) {
      setOperationID("");
      return;
    }
    if (visibleOperations.some((operation) => operation.id === operationID)) return;
    const nextOperation =
      visibleOperations.find((operation) => operation.safety === "read_only") ??
      visibleOperations[0];
    setOperationID(nextOperation.id);
    setParameterValues(parameterDefaults(nextOperation));
    setRequestBody(formatExplorerRequestExample(nextOperation));
    setResult(null);
  }, [operationID, visibleOperations]);

  function selectOperation(id: string) {
    const operation = catalog?.operations.find((item) => item.id === id);
    setOperationID(id);
    setParameterValues(parameterDefaults(operation));
    setRequestBody(formatExplorerRequestExample(operation));
    setResult(null);
  }

  async function saveCredential(event: FormEvent, profile: PartnerExplorerCredentialState) {
    event.preventDefault();
    const officialAPIKey = credentialInputs[profile.code]?.trim() ?? "";
    if (!officialAPIKey) return;
    setSavingCredential(true);
    onError("");
    try {
      const state = await api.saveExplorerCredentialProfile(profile.code, {
        official_api_key: officialAPIKey,
        auth_header: profile.auth_header,
        auth_prefix: profile.auth_prefix,
      });
      const merged = { ...profile, ...state, label: profile.label, description: profile.description };
      setCredentials((current) => current.map((item) => item.code === profile.code ? merged : item));
      setCatalog((current) => current ? {
        ...current,
        credentials: current.credentials.map((item) => item.code === profile.code ? merged : item),
      } : current);
      setCredentialInputs((current) => ({ ...current, [profile.code]: "" }));
    } catch (credentialError) {
      onError(getErrorMessage(credentialError));
    } finally {
      setSavingCredential(false);
    }
  }

  async function removeCredential(profile: PartnerExplorerCredentialState) {
    if (!window.confirm(`Hapus ${profile.label} dari Explorer? Endpoint terkait tidak dapat diuji sampai key dihubungkan kembali.`)) return;
    onError("");
    try {
      await api.deleteExplorerCredentialProfile(profile.code);
      const state = { ...profile, configured: false, credential: null };
      setCredentials((current) => current.map((item) => item.code === profile.code ? state : item));
      setCatalog((current) => current ? {
        ...current,
        credentials: current.credentials.map((item) => item.code === profile.code ? state : item),
      } : current);
      setResult(null);
    } catch (credentialError) {
      onError(getErrorMessage(credentialError));
    }
  }

  async function execute(event: FormEvent) {
    event.preventDefault();
    if (!selectedOperation || !catalog || selectedOperation.safety !== "read_only") return;
    let parsedBody: unknown = null;
    if (requestBody.trim()) {
      try {
        parsedBody = JSON.parse(requestBody);
      } catch {
        onError("Request body wajib berupa JSON yang valid.");
        return;
      }
    }
    const pathParams: Record<string, string> = {};
    const query: Record<string, string> = {};
    for (const parameter of selectedOperation.parameters) {
      const value = parameterValues[`${parameter.in}:${parameter.name}`] ?? "";
      if (parameter.in === "path") pathParams[parameter.name] = value;
      if (parameter.in === "query") query[parameter.name] = value;
    }
    setExecuting(true);
    onError("");
    try {
      const execution = await api.executeExplorer(catalog.submission_id, {
        operation_id: selectedOperation.id,
        path_params: pathParams,
        query,
        body: parsedBody,
      });
      setResult(execution);
      setRuns(await api.explorerRuns(catalog.submission_id));
    } catch (executeError) {
      onError(getErrorMessage(executeError));
    } finally {
      setExecuting(false);
    }
  }

  const selectedCredential = selectedOperation?.credential_code
    ? credentials.find((item) => item.code === selectedOperation.credential_code)
    : undefined;
  const selectedCredentialReady = !selectedOperation?.credential_code || selectedCredential?.configured === true;

  const codeSample = catalog && selectedOperation
    ? partnerExplorerCodeSample(
      codeLanguage,
      catalog,
      selectedOperation,
      parameterValues,
      requestBody,
      selectedCredential,
    )
    : "Pilih endpoint untuk melihat contoh request.";

  return (
    <section className="partner-api-explorer" id="partner-api-explorer">
      <section className="panel partner-explorer-heading">
        <div>
          <p className="eyebrow">VISUAL API EXPLORER</p>
          <h2>Pilih platform, endpoint, dan lihat hasilnya langsung</h2>
          <p>
            Request read-only dijalankan melalui test runner API Kurir menggunakan
            API key resmi <strong>{identity.provider_name}</strong>. Key tidak pernah
            dikirim ke browser atau ditampilkan kembali setelah tersimpan.
          </p>
          <p>
            Setiap security scheme OpenAPI mendapat credential terpisah. Contohnya,
            RajaOngkir memakai key Shipping Cost untuk rate/tracking dan key Shipping
            Delivery untuk shipment/pickup.
          </p>
        </div>
        <div className="partner-explorer-live-badge">
          <span>OFFICIAL / LIVE</span>
          <strong>Menggunakan quota provider</strong>
        </div>
      </section>

      {!eligibleSubmissions.length ? (
        <section className="panel partner-explorer-empty">
          <strong>Upload package yang lulus validasi terlebih dahulu.</strong>
          <p>Explorer membaca endpoint langsung dari <code>openapi.yaml</code> pada package.</p>
        </section>
      ) : (
        <>
          <section className="partner-explorer-grid">
            <section className="partner-explorer-credential-stack">
              {credentials.map((profile) => (
                <form className="panel partner-explorer-credential" key={profile.code} onSubmit={(event) => void saveCredential(event, profile)}>
                  <div className="panel-heading">
                    <div><p className="eyebrow">OFFICIAL CREDENTIAL</p><h3>{profile.label}</h3></div>
                    <span className={`badge ${profile.configured ? "badge-success" : "badge-warning"}`}>
                      {profile.configured ? "Terhubung" : "Belum ada"}
                    </span>
                  </div>
                  {profile.description && <p className="partner-explorer-note">{profile.description}</p>}
                  {profile.credential && (
                    <div className="partner-explorer-connected-key">
                      <span>{profile.credential.auth_header}</span>
                      <code>{profile.credential.display_key}</code>
                      <small>Diperbarui {formatDate(profile.credential.updated_at)}</small>
                    </div>
                  )}
                  <label>
                    API key resmi
                    <input
                      type="password"
                      value={credentialInputs[profile.code] ?? ""}
                      onChange={(event) => setCredentialInputs((current) => ({ ...current, [profile.code]: event.target.value }))}
                      placeholder={profile.configured ? "Masukkan key baru untuk rotasi" : `Masukkan ${profile.label}`}
                      autoComplete="new-password"
                      required={!profile.configured}
                    />
                  </label>
                  <div className="partner-explorer-auth-readonly">
                    <span>Header</span><code>{profile.auth_header}</code>
                    <span>Prefix</span><code>{profile.auth_prefix || "tanpa prefix"}</code>
                  </div>
                  <p className="partner-explorer-note">
                    Credential disimpan terenkripsi dan hanya dipakai endpoint dengan security scheme <code>{profile.code}</code>.
                  </p>
                  <div className="partner-explorer-actions">
                    <button className="button button-primary" disabled={savingCredential || !(credentialInputs[profile.code]?.trim())}>
                      {savingCredential ? "Menyimpan…" : profile.configured ? "Rotasi credential" : "Hubungkan credential"}
                    </button>
                    {profile.configured && (
                      <button className="button button-danger" type="button" onClick={() => void removeCredential(profile)}>
                        Hapus
                      </button>
                    )}
                  </div>
                </form>
              ))}
              {!credentials.length && (
                <section className="panel partner-explorer-credential">
                  <p className="eyebrow">OFFICIAL CREDENTIAL</p>
                  <h3>Tidak diperlukan</h3>
                  <p className="partner-explorer-note">OpenAPI package tidak mendeklarasikan security scheme pada endpoint.</p>
                </section>
              )}
            </section>

            <form className="panel partner-explorer-request" onSubmit={execute}>
              <div className="partner-explorer-select-grid">
                <label>
                  Platform
                  <select value={identity.provider_code} disabled>
                    <option value={identity.provider_code}>{identity.provider_name}</option>
                  </select>
                </label>
                <label>
                  Package
                  <select value={submissionID} onChange={(event) => setSubmissionID(event.target.value)}>
                    {eligibleSubmissions.map((item) => (
                      <option key={item.id} value={item.id}>v{item.version} · {PARTNER_SUBMISSION_STATUS_LABELS[item.status]}</option>
                    ))}
                  </select>
                </label>
                <label>
                  Capability
                  <select value={capability} onChange={(event) => setCapability(event.target.value)} disabled={loading}>
                    {capabilities.map((item) => <option key={item} value={item}>{item === "all" ? "Semua capability" : item}</option>)}
                  </select>
                </label>
                <label>
                  Endpoint
                  <select value={operationID} onChange={(event) => selectOperation(event.target.value)} disabled={loading || !visibleOperations.length}>
                    {visibleOperations.map((operation) => (
                      <option key={operation.id} value={operation.id}>{operation.method} {operation.path}</option>
                    ))}
                  </select>
                </label>
              </div>

              {selectedOperation && (
                <div className="partner-explorer-operation-summary">
                  <span className={`method-badge method-${selectedOperation.method.toLowerCase()}`}>{selectedOperation.method}</span>
                  <div><strong>{selectedOperation.summary}</strong><code>{selectedOperation.path}</code></div>
                  <span className={`badge ${selectedOperation.safety === "read_only" ? "badge-success" : "badge-warning"}`}>
                    {selectedOperation.safety === "read_only" ? "Live read-only" : "Transaksi dikunci"}
                  </span>
                </div>
              )}

              {!!selectedOperation?.parameters.length && (
                <div className="partner-explorer-parameters">
                  {selectedOperation.parameters.map((parameter) => (
                    <label key={`${parameter.in}:${parameter.name}`}>
                      {parameter.name} <small>{parameter.in}{parameter.required ? " · wajib" : ""}</small>
                      <input
                        value={parameterValues[`${parameter.in}:${parameter.name}`] ?? ""}
                        onChange={(event) => setParameterValues((current) => ({
                          ...current,
                          [`${parameter.in}:${parameter.name}`]: event.target.value,
                        }))}
                        placeholder={parameter.example || parameter.description || parameter.type}
                        required={parameter.required}
                      />
                    </label>
                  ))}
                </div>
              )}

              {selectedOperation && !["GET", "HEAD", "OPTIONS"].includes(selectedOperation.method) && (
                <label className="partner-explorer-body">
                  JSON request body
                  <textarea value={requestBody} onChange={(event) => setRequestBody(event.target.value)} rows={10} spellCheck={false} />
                </label>
              )}

              {selectedOperation?.safety === "transactional_locked" && (
                <div className="alert alert-warning">
                  Endpoint transaksi belum dapat dijalankan dengan key live. Admin perlu menyetujui mode transaksi pengujian terlebih dahulu.
                </div>
              )}
              <button
                className="button button-primary partner-explorer-run"
                disabled={executing || loading || !selectedCredentialReady || selectedOperation?.safety !== "read_only"}
              >
                {executing ? "Menjalankan request…" : "Explore endpoint"}
              </button>
              <small className="partner-explorer-quota">Maksimal 120 request/jam per Partner Access Key.</small>
            </form>
          </section>

          <section className="partner-explorer-output-grid">
            <section className="panel partner-explorer-code">
              <div className="partner-explorer-tabs">
                {(["curl", "typescript", "php", "go"] as PartnerExplorerCodeLanguage[]).map((language) => (
                  <button key={language} type="button" className={codeLanguage === language ? "active" : ""} onClick={() => setCodeLanguage(language)}>
                    {language === "curl" ? "cURL" : language === "typescript" ? "TypeScript" : language === "php" ? "PHP" : "Go"}
                  </button>
                ))}
                <button type="button" className="partner-explorer-copy" onClick={() => void navigator.clipboard.writeText(codeSample)}>Salin</button>
              </div>
              <pre><code>{codeSample}</code></pre>
              <small>Credential selalu ditampilkan sebagai placeholder, tidak pernah sebagai secret asli.</small>
            </section>

            <section className="panel partner-explorer-response">
              <div className="panel-heading">
                <div><p className="eyebrow">RESPONSE</p><h3>{result ? `${result.response_status || "Network"} · ${result.duration_ms} ms` : "Belum ada hasil"}</h3></div>
                {result && <span className={`badge ${result.success ? "badge-success" : "badge-danger"}`}>{result.success ? "Passed" : "Failed"}</span>}
              </div>
              <pre><code>{result ? prettyExplorerResponse(result.response_body) : "Jalankan endpoint untuk melihat response provider."}</code></pre>
              {result && (
                <div className="partner-explorer-validation">
                  <span className={result.validation.http_passed ? "passed" : "failed"}>HTTP {result.validation.http_passed ? "valid" : "gagal"}</span>
                  <span className={result.validation.json_passed ? "passed" : "failed"}>JSON {result.validation.json_passed ? "valid" : "gagal"}</span>
                  {result.truncated && <span className="failed">Response dipotong</span>}
                </div>
              )}
            </section>
          </section>

          <section className="panel panel-table partner-explorer-runs">
            <div className="panel-heading panel-padding">
              <div><p className="eyebrow">RECENT TESTS</p><h3>Bukti pengujian package</h3></div>
              <span className="subtle">Response tersimpan dalam bentuk preview yang sudah disensor.</span>
            </div>
            <div className="table-scroll">
              <table>
                <thead><tr><th>Endpoint</th><th>Hasil</th><th>Status</th><th>Latency</th><th>Waktu</th></tr></thead>
                <tbody>
                  {runs.map((run) => (
                    <tr key={run.id}>
                      <td><strong>{run.method} {run.path}</strong><small>{run.operation_id}</small></td>
                      <td><span className={`badge ${run.outcome === "passed" ? "badge-success" : "badge-danger"}`}>{run.outcome}</span></td>
                      <td>{run.response_status || run.error_code || "Network error"}</td>
                      <td>{run.duration_ms} ms</td>
                      <td>{formatDate(run.created_at)}</td>
                    </tr>
                  ))}
                  {!runs.length && <tr><td colSpan={5} className="empty-state">Belum ada endpoint yang diuji pada package ini.</td></tr>}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </section>
  );
}

function parameterDefaults(operation?: PartnerExplorerOperation) {
  const result: Record<string, string> = {};
  for (const parameter of operation?.parameters ?? []) {
    result[`${parameter.in}:${parameter.name}`] = parameter.example ?? "";
  }
  return result;
}

function formatExplorerRequestExample(operation?: PartnerExplorerOperation) {
  if (!operation?.request_example) return "";
  return JSON.stringify(operation.request_example, null, 2);
}

function partnerExplorerRequestURL(
  catalog: PartnerExplorerCatalog,
  operation: PartnerExplorerOperation,
  values: Record<string, string>,
) {
  let endpointPath = operation.path;
  const query = new URLSearchParams();
  for (const parameter of operation.parameters) {
    const value = values[`${parameter.in}:${parameter.name}`] ?? "";
    if (parameter.in === "path" && value) endpointPath = endpointPath.replace(`{${parameter.name}}`, value);
    if (parameter.in === "query" && value) query.set(parameter.name, value);
  }
  const baseURL = operation.base_url || catalog.base_url;
  return `${baseURL.replace(/\/+$/, "")}/${endpointPath.replace(/^\/+/, "")}${query.size ? `?${query}` : ""}`;
}

function partnerExplorerCodeSample(
  language: PartnerExplorerCodeLanguage,
  catalog: PartnerExplorerCatalog,
  operation: PartnerExplorerOperation,
  values: Record<string, string>,
  body: string,
  credential?: PartnerExplorerCredentialState,
) {
  const url = partnerExplorerRequestURL(catalog, operation, values);
  if (!operation.credential_code) {
    if (language === "curl") {
      return `curl -X ${operation.method} ${JSON.stringify(url)} -H "Accept: application/json"`;
    }
    switch (language) {
      case "typescript":
        return `const response = await fetch(${JSON.stringify(url)}, {\n  method: ${JSON.stringify(operation.method)},\n  headers: { "Accept": "application/json" }\n});\n\nconsole.log(await response.json());`;
      case "php":
        return `$response = file_get_contents(${JSON.stringify(url)}, false, stream_context_create([\n  'http' => [\n    'method' => ${JSON.stringify(operation.method)},\n    'header' => "Accept: application/json"\n  ]\n]));`;
      case "go":
        return `req, _ := http.NewRequest(${JSON.stringify(operation.method)}, ${JSON.stringify(url)}, nil)\nreq.Header.Set("Accept", "application/json")\nresp, err := http.DefaultClient.Do(req)`;
      default:
        return `curl -X ${operation.method} ${JSON.stringify(url)} \\\n+  -H "Accept: application/json"`;
    }
  }
  const header = operation.auth_header || credential?.credential?.auth_header || "Authorization";
  const prefixValue = operation.auth_prefix || credential?.credential?.auth_prefix || "";
  const prefix = prefixValue ? `${prefixValue} ` : "";
  const auth = `${prefix}<OFFICIAL_API_KEY>`;
  const hasBody = body.trim() && !["GET", "HEAD", "OPTIONS"].includes(operation.method);
  switch (language) {
    case "typescript":
      return `const response = await fetch(${JSON.stringify(url)}, {\n  method: ${JSON.stringify(operation.method)},\n  headers: {\n    ${JSON.stringify(header)}: ${JSON.stringify(auth)},\n    "Accept": "application/json"${hasBody ? ',\n    "Content-Type": "application/json"' : ""}\n  }${hasBody ? `,\n  body: JSON.stringify(${body})` : ""}\n});\n\nconsole.log(await response.json());`;
    case "php":
      return `$response = file_get_contents(${JSON.stringify(url)}, false, stream_context_create([\n  'http' => [\n    'method' => ${JSON.stringify(operation.method)},\n    'header' => ${JSON.stringify(`${header}: ${auth}\\r\\nAccept: application/json${hasBody ? "\\r\\nContent-Type: application/json" : ""}`)}${hasBody ? `,\n    'content' => ${JSON.stringify(body)}` : ""}\n  ]\n]));`;
    case "go":
      return `payload := strings.NewReader(${JSON.stringify(hasBody ? body : "")})\nreq, _ := http.NewRequest(${JSON.stringify(operation.method)}, ${JSON.stringify(url)}, payload)\nreq.Header.Set(${JSON.stringify(header)}, ${JSON.stringify(auth)})\nreq.Header.Set("Accept", "application/json")${hasBody ? '\nreq.Header.Set("Content-Type", "application/json")' : ""}\nresp, err := http.DefaultClient.Do(req)`;
    default:
      return `curl -X ${operation.method} ${JSON.stringify(url)} \\\n  -H ${JSON.stringify(`${header}: ${auth}`)} \\\n  -H "Accept: application/json"${hasBody ? ` \\\n  -H "Content-Type: application/json" \\\n  --data ${JSON.stringify(body)}` : ""}`;
  }
}

function prettyExplorerResponse(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function PartnerPortalIntegrationDocumentation({
  providerCode,
  providerName,
  starterDownloading,
  onDownloadStarter,
}: {
  providerCode: string;
  providerName: string;
  starterDownloading: boolean;
  onDownloadStarter: () => void;
}) {
  return (
    <section className="partner-integration-docs" id="partner-integration-docs">
      <section className="panel partner-integration-docs-heading">
        <div>
          <p className="eyebrow">DOKUMENTASI INTEGRASI</p>
          <h2>Bangun connector {providerName} untuk Emisell</h2>
          <p>
            Connector tetap berjalan pada infrastruktur partner. API Kurir akan
            memanggil kontrak standar ini setelah package, pengujian, keamanan,
            dan UAT dinyatakan lulus.
          </p>
        </div>
        <div className="partner-doc-actions">
          <button
            className="button button-primary"
            type="button"
            onClick={onDownloadStarter}
            disabled={starterDownloading}
          >
            {starterDownloading ? "Menyiapkan ZIP…" : "Download Starter Package"}
          </button>
          <a
            className="button button-secondary"
            href="/openapi/api-kurir-partner-v1.yaml"
            download="api-kurir-partner-v1.yaml"
          >
            Unduh kontrak OpenAPI (wajib)
          </a>
        </div>
      </section>

      <div className="partner-integration-flow" aria-label="Alur integrasi partner">
        <span>Emisell</span><i>→</i><strong>API Kurir</strong><i>→</i>
        <span>Connector {providerName}</span>
      </div>

      <section className="partner-integration-docs-grid">
        <article className="panel">
          <p className="eyebrow">QUICK START</p>
          <h3>Tujuh langkah integrasi</h3>
          <ol>
            <li>Unduh dan pelajari kontrak OpenAPI Partner API v1.</li>
            <li>Implementasikan endpoint yang dibutuhkan pada base path <code>/partner/v1</code>.</li>
            <li>Sediakan satu endpoint connector HTTPS aktif; alamat gateway API Kurir mengikuti environment.</li>
            <li>Siapkan manifest, OpenAPI implementasi, dokumentasi, dan bukti pengujian.</li>
            <li>Upload ZIP dengan nomor versi baru yang tidak pernah dipakai sebelumnya.</li>
            <li>Hubungkan API key resmi lalu jalankan endpoint read-only melalui Visual API Explorer.</li>
            <li>Perbaiki feedback sampai pengujian kontrak, security review, dan UAT lulus.</li>
          </ol>
        </article>

        <article className="panel partner-integration-auth">
          <p className="eyebrow">AUTENTIKASI</p>
          <h3>Portal key bukan runtime key</h3>
          <p>
            Key <code>epk_live_*</code> hanya untuk portal dan submission milik
            <strong> {providerCode}</strong>. API key resmi untuk pengujian hanya
            dimasukkan pada Visual API Explorer, disimpan terenkripsi, dan tidak
            menjadi runtime token production. Credential server-to-server final
            tetap diprovisikan terpisah ketika release siap diaktifkan.
          </p>
          <pre><code>{`Authorization: Bearer <partner-runtime-token>
X-Partner-Key-Id: pk_live_...
X-Request-Id: req_...`}</code></pre>
          <p>
            Semua request POST memakai HMAC-SHA256, timestamp, nonce, dan
            <code> Idempotency-Key</code> sesuai spesifikasi OpenAPI.
          </p>
        </article>
      </section>

      <section className="partner-developer-preparation" id="partner-developer-preparation">
        <div className="docs-section-heading">
          <div>
            <p className="eyebrow">SEBELUM UPLOAD</p>
            <h2>Yang perlu disiapkan developer</h2>
            <p>
              ZIP adalah artefak sertifikasi, bukan aplikasi yang dijalankan di
              server API Kurir. Connector tetap di-host dan dioperasikan oleh partner.
            </p>
          </div>
          <span className="badge badge-success">Checklist developer</span>
        </div>

        <div className="partner-developer-preparation-grid">
          <article className="panel">
            <span className="partner-preparation-number">01</span>
            <p className="eyebrow">WAJIB SAAT UPLOAD</p>
            <h3>Package dapat divalidasi</h3>
            <ul>
              <li><code>emisell-extension.yaml</code> berada tepat pada root ZIP.</li>
              <li><code>openapi.yaml</code> OpenAPI 3.x berada tepat pada root ZIP.</li>
              <li>Provider manifest harus <code>{providerCode}</code>.</li>
              <li><code>connector.base_url</code> memakai satu endpoint HTTPS aktif.</li>
              <li>ZIP maksimal 25 MB, hasil ekstraksi 100 MB, dan 250 entry.</li>
              <li>Limit 10 percobaan per jam dan satu upload aktif per access key.</li>
              <li>Kuota maksimal 25 versi atau total 500 MB per provider.</li>
            </ul>
          </article>

          <article className="panel">
            <span className="partner-preparation-number">02</span>
            <p className="eyebrow">DIREKOMENDASIKAN</p>
            <h3>Mempercepat proses review</h3>
            <ul>
              <li><code>README.md</code> berisi setup, endpoint, dan cara pengujian.</li>
              <li><code>SECURITY.md</code> berisi kontak dan prosedur insiden.</li>
              <li><code>CHANGELOG.md</code> menjelaskan perubahan setiap versi.</li>
              <li>Contoh request/response dan contract test dengan data aman.</li>
              <li>API key resmi tersedia untuk pengujian read-only melalui Explorer.</li>
              <li>Source code dan SBOM tidak diwajibkan pada fase awal.</li>
            </ul>
          </article>

          <article className="panel partner-preparation-danger">
            <span className="partner-preparation-number">03</span>
            <p className="eyebrow">DILARANG</p>
            <h3>Jangan masukkan data sensitif</h3>
            <ul>
              <li>API key, password, token, private key, atau file <code>.env</code>.</li>
              <li>Data merchant, customer, alamat, nomor telepon, atau AWB asli.</li>
              <li>Binary native, symlink, path absolut, atau file hasil build yang tidak perlu.</li>
              <li>Credential runtime; credential diprovisikan terpisah setelah sertifikasi.</li>
              <li>API key resmi di dalam ZIP; masukkan hanya melalui Visual API Explorer.</li>
              <li>Source code bersifat opsional dan tidak pernah dieksekusi API Kurir.</li>
            </ul>
          </article>
        </div>
      </section>

      <PartnerConnectorEndpointCatalog />

      <section className="partner-integration-docs-grid">
        <article className="panel partner-package-doc-checklist">
          <p className="eyebrow">PACKAGE ZIP</p>
          <h3>Struktur yang direkomendasikan</h3>
          <pre><code>{`partner-package.zip
├── emisell-extension.yaml   # wajib saat upload
├── openapi.yaml             # wajib saat upload
├── README.md                # direkomendasikan
├── SECURITY.md              # direkomendasikan
├── CHANGELOG.md             # direkomendasikan
├── examples/
├── contract-tests/
└── src/                     # opsional, tidak dieksekusi`}</code></pre>
          <p>
            Provider pada manifest wajib <code>{providerCode}</code>. Secret,
            file <code>.env</code>, private key, binary native, dan data customer
            tidak boleh dimasukkan ke package.
          </p>
        </article>

        <article className="panel partner-package-doc-checklist">
          <p className="eyebrow">CONTOH MANIFEST</p>
          <h3>emisell-extension.yaml</h3>
          <pre><code>{`schema_version: "1"
provider:
  code: ${providerCode}
  name: ${providerName}
connector:
  contract_version: v1
  base_url: https://api.partner.co.id/partner/v1
capabilities:
  - rates
  - shipments
  - pickup
  - tracking
services:
  - regular
  - next_day
  - economy
  - cargo`}</code></pre>
          <p>
            Deklarasikan hanya capability dan service yang benar-benar tersedia.
            Endpoint wajib akan diperiksa otomatis berdasarkan capability tersebut.
          </p>
        </article>
      </section>

      <section className="partner-integration-docs-grid">
        <article className="panel partner-integration-readiness">
          <p className="eyebrow">PRODUCTION READINESS</p>
          <h3>Syarat sebelum published</h3>
          <ul>
            <li>TLS 1.2+ dan credential dapat dirotasi tanpa downtime.</li>
            <li>HMAC, replay protection, idempotency, retry, dan audit trail.</li>
            <li>Mapping status, service, AWB, label, pickup, dan error konsisten.</li>
            <li>Isolasi data merchant dan tidak ada credential pada log.</li>
            <li>Contract test serta skenario kegagalan lulus.</li>
          </ul>
        </article>

        <article className="panel partner-integration-readiness">
          <p className="eyebrow">HASIL UPLOAD</p>
          <h3>Apa yang terjadi setelah dikirim?</h3>
          <ol>
            <li>ZIP masuk karantina dan memperoleh checksum SHA-256.</li>
            <li>Struktur arsip, secret, manifest, dan OpenAPI diperiksa otomatis.</li>
            <li>Hasil gagal mendapatkan status <strong>Perlu revisi</strong> beserta alasannya.</li>
            <li>Hasil lulus masuk review teknis, pengujian, keamanan, dan UAT.</li>
            <li>Hanya release published yang dapat diaktifkan untuk merchant.</li>
          </ol>
        </article>
      </section>
    </section>
  );
}

const PARTNER_SUBMISSION_STATUS_LABELS: Record<PartnerSubmissionStatus, string> = {
  technical_review: "Review teknis",
  sandbox_testing: "Pengujian",
  security_review: "Review keamanan",
  uat: "UAT",
  approved: "Disetujui",
  published: "Published",
  changes_requested: "Perlu revisi",
  rejected: "Ditolak",
  suspended: "Ditangguhkan",
  superseded: "Digantikan",
};

const PARTNER_SUBMISSION_TRANSITIONS: Record<
  PartnerSubmissionStatus,
  PartnerSubmissionStatus[]
> = {
  technical_review: ["sandbox_testing", "changes_requested", "rejected"],
  sandbox_testing: ["security_review", "changes_requested", "rejected"],
  security_review: ["uat", "changes_requested", "rejected"],
  uat: ["approved", "changes_requested", "rejected"],
  approved: ["published", "technical_review"],
  published: ["suspended"],
  suspended: ["published", "technical_review"],
  changes_requested: ["rejected"],
  rejected: [],
  superseded: ["published"],
};

function formatFileSize(value: number) {
  if (value < 1024) return `${value} byte`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function partnerStatusTone(status: PartnerSubmissionStatus) {
  if (status === "published" || status === "approved") return "badge-success";
  if (status === "changes_requested" || status === "suspended") return "badge-warning";
  if (status === "rejected") return "badge-danger";
  return "";
}

function PartnerSubmissionManagement({
  api,
  onError,
}: {
  api: AdminApi;
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<PartnerSubmission[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [reviewStatus, setReviewStatus] = useState<PartnerSubmissionStatus | "">("");
  const [reviewNote, setReviewNote] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const reviewPanelRef = useRef<HTMLElement | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      const result = await api.partnerSubmissions({
        status: statusFilter,
        limit: 100,
      }, signal);
      setItems(result);
      setSelectedID((current) =>
        current && result.some((item) => item.id === current)
          ? current
          : "",
      );
    } catch (requestError) {
      if (requestError instanceof Error && requestError.name === "AbortError") return;
      onError(getErrorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }, [api, onError, statusFilter]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const selected = items.find((item) => item.id === selectedID) ?? null;
  const transitions = selected
    ? PARTNER_SUBMISSION_TRANSITIONS[selected.status]
    : [];

  useEffect(() => {
    if (!selected) {
      setReviewStatus("");
      setReviewNote("");
      return;
    }
    setReviewStatus(PARTNER_SUBMISSION_TRANSITIONS[selected.status][0] ?? "");
    setReviewNote(selected.review_note);
  }, [selected?.id, selected?.status]);

  async function updateStatus(event: FormEvent) {
    event.preventDefault();
    if (!selected || !reviewStatus) return;
    setReviewing(true);
    onError("");
    try {
      const updated = await api.updatePartnerSubmissionStatus(
        selected.id,
        reviewStatus,
        reviewNote,
      );
      await load();
      setSelectedID(updated.id);
    } catch (reviewError) {
      onError(getErrorMessage(reviewError));
    } finally {
      setReviewing(false);
    }
  }

  async function downloadArtifact() {
    if (!selected) return;
    setDownloading(true);
    onError("");
    try {
      const blob = await api.downloadPartnerSubmissionArtifact(selected.id);
      const source = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = source;
      anchor.download = selected.file_name;
	  document.body.appendChild(anchor);
      anchor.click();
	  anchor.remove();
      URL.revokeObjectURL(source);
    } catch (downloadError) {
      onError(getErrorMessage(downloadError));
    } finally {
      setDownloading(false);
    }
  }

  function openReview(id: string) {
    setSelectedID(id);
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        reviewPanelRef.current?.scrollIntoView({
          behavior: "smooth",
          block: "start",
        });
      });
    });
  }

  const passedCount = items.filter((item) => item.scan_report.passed).length;
  const publishedCount = items.filter((item) => item.status === "published").length;
  const revisionCount = items.filter((item) => item.status === "changes_requested").length;

  return (
    <div className="partner-package-page">
      <section className="provider-key-hero partner-package-hero">
        <div>
          <p className="eyebrow">SUBMISSION &amp; REVIEW CONSOLE</p>
          <h2>Package integrasi partner</h2>
          <p>
            Staff meninjau package yang dikirim langsung oleh partner melalui
            Partner Portal. Identitas provider dikunci oleh access key partner,
            bukan dipilih oleh staff pada formulir upload.
          </p>
        </div>
        <span className="badge">Internal staff</span>
      </section>

      <section className="provider-management-summary partner-package-summary">
          <article><span>Submission</span><strong>{formatNumber(items.length)}</strong></article>
          <article><span>Lulus scan</span><strong>{formatNumber(passedCount)}</strong></article>
          <article><span>Published</span><strong>{formatNumber(publishedCount)}</strong></article>
          <article><span>Perlu revisi</span><strong>{formatNumber(revisionCount)}</strong></article>
      </section>

      <section className="toolbar partner-package-toolbar">
        <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          <option value="">Semua status</option>
          {Object.entries(PARTNER_SUBMISSION_STATUS_LABELS).map(([value, label]) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>
        <button className="button button-secondary" onClick={() => void load()} disabled={loading}>
          {loading ? "Memuat…" : "Muat ulang"}
        </button>
        <span className="subtle">Versi immutable · satu published per provider</span>
      </section>

      <section className="panel panel-table partner-package-table">
        <div className="table-scroll">
          <table>
            <thead><tr>
              <th>Provider / versi</th><th>Package</th><th>Scan</th><th>Status</th>
              <th>Diajukan</th><th>Aktivitas</th><th>Aksi</th>
            </tr></thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id} className={selectedID === item.id ? "table-row-selected" : ""}>
                  <td>
                    <strong>{item.provider_name}</strong>
                    <small>{item.provider_code} · v{item.version}</small>
                    {item.is_active_release && <span className="badge badge-success">release aktif</span>}
                  </td>
                  <td><strong>{item.file_name}</strong><small>{formatFileSize(item.artifact_size)} · sha256 {item.artifact_sha256.slice(0, 12)}…</small></td>
                  <td><span className={`badge ${item.scan_report.passed ? "badge-success" : "badge-danger"}`}>{item.scan_report.passed ? "lulus" : "gagal"}</span><small>{item.scan_report.file_count} file</small></td>
                  <td><span className={`badge ${partnerStatusTone(item.status)}`}>{PARTNER_SUBMISSION_STATUS_LABELS[item.status]}</span></td>
                  <td><strong>{item.submitted_by}</strong><small>{formatDate(item.created_at)}</small></td>
                  <td><strong>{item.reviewed_by || "Belum direview"}</strong><small>{formatDate(item.updated_at)}</small></td>
                  <td>
                    <button
                      className="table-action partner-package-review-trigger"
                      onClick={() => openReview(item.id)}
                      aria-expanded={selectedID === item.id}
                    >
                      {selectedID === item.id ? "Review dibuka" : "Buka review"}
                      <small>{PARTNER_SUBMISSION_TRANSITIONS[item.status].length} aksi</small>
                    </button>
                  </td>
                </tr>
              ))}
              {!items.length && <tr><td colSpan={7} className="empty-state">{loading ? "Memuat submission partner…" : "Belum ada package yang sesuai filter."}</td></tr>}
            </tbody>
          </table>
        </div>
      </section>

      {selected && (
        <section className="panel partner-package-review" ref={reviewPanelRef} tabIndex={-1}>
          <div className="panel-heading">
            <div>
              <p className="eyebrow">HASIL VALIDASI</p>
              <h2>{selected.provider_name} · v{selected.version}</h2>
              <span className={`badge ${partnerStatusTone(selected.status)}`}>
                Status: {PARTNER_SUBMISSION_STATUS_LABELS[selected.status]}
              </span>
            </div>
            <div className="partner-package-review-heading-actions">
              <button className="button button-secondary" onClick={() => void downloadArtifact()} disabled={downloading}>
                {downloading ? "Mengunduh…" : "Unduh ZIP"}
              </button>
              <button className="button button-quiet" onClick={() => setSelectedID("")}>
                Tutup review
              </button>
            </div>
          </div>

          <div className="partner-package-review-grid">
            <div>
              <h3>Automated checks</h3>
              <div className="partner-package-checks">
                {selected.scan_report.checks.map((check) => (
                  <article key={check.code}>
                    <span className={`badge ${check.status === "passed" ? "badge-success" : "badge-danger"}`}>{check.status}</span>
                    <div><strong>{check.code.replaceAll("_", " ")}</strong><p>{check.message}</p></div>
                  </article>
                ))}
              </div>
              {selected.scan_report.warnings.map((warning) => (
                <div className="alert alert-warning" key={warning}>{warning}</div>
              ))}
            </div>

            <div className="partner-package-manifest">
              <h3>Manifest connector</h3>
              <dl>
                <div><dt>Contract</dt><dd>{selected.scan_report.manifest.contract_version || "—"}</dd></div>
                <div><dt>Endpoint</dt><dd>{selected.scan_report.manifest.base_url || "—"}</dd></div>
                <div><dt>Capability</dt><dd>{selected.scan_report.manifest.declared_capabilities.join(", ") || "—"}</dd></div>
                <div><dt>Scope runtime</dt><dd>{selected.required_scopes.join(", ") || "Tidak ada"}</dd></div>
                <div><dt>Service</dt><dd>{selected.scan_report.manifest.declared_services.join(", ") || "—"}</dd></div>
                <div><dt>OpenAPI paths</dt><dd>{selected.scan_report.required_openapi_paths.join(", ") || "—"}</dd></div>
              </dl>
            </div>
          </div>

          <form className="partner-package-review-form" onSubmit={updateStatus}>
            <label>
              Tahap berikutnya
              <select
                value={reviewStatus}
                onChange={(event) => setReviewStatus(event.target.value as PartnerSubmissionStatus)}
                disabled={!transitions.length}
                required
              >
                {!transitions.length && <option value="">Tidak ada transisi</option>}
                {transitions.map((status) => (
                  <option key={status} value={status}>{PARTNER_SUBMISSION_STATUS_LABELS[status]}</option>
                ))}
              </select>
            </label>
            <label className="partner-package-review-note">
              Catatan review
              <textarea
                value={reviewNote}
                onChange={(event) => setReviewNote(event.target.value)}
                maxLength={4000}
                rows={3}
                placeholder="Temuan, bukti pengujian, atau perubahan yang diminta…"
              />
            </label>
            <button className="button button-primary" disabled={reviewing || !reviewStatus}>
              {reviewing ? "Menyimpan…" : "Jalankan aksi review"}
            </button>
          </form>
        </section>
      )}
    </div>
  );
}

function WebhookManagement({
  settings,
  generatedSecret,
  testResult,
  loading,
  onSave,
  onGenerateSecret,
  onDismissSecret,
  onTest,
}: {
  settings: WebhookSettings;
  generatedSecret: string;
  testResult: WebhookTestResult | null;
  loading: boolean;
  onSave: (callbackURL: string, enabled: boolean) => Promise<void>;
  onGenerateSecret: () => Promise<void>;
  onDismissSecret: () => void;
  onTest: () => Promise<void>;
}) {
  const [callbackURL, setCallbackURL] = useState(settings.callback_url);
  const [enabled, setEnabled] = useState(settings.enabled);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setCallbackURL(settings.callback_url);
    setEnabled(settings.enabled);
  }, [settings.callback_url, settings.enabled]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await onSave(callbackURL.trim(), enabled);
  }

  async function copySecret() {
    if (!generatedSecret) return;
    await navigator.clipboard.writeText(generatedSecret);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1800);
  }

  const canTest = Boolean(
    settings.callback_url && settings.secret_configured,
  );

  return (
    <div className="webhook-page">
      <section className="webhook-hero">
        <div>
          <p className="eyebrow">TRACKING OUTBOUND</p>
          <h2>Status order bergerak otomatis</h2>
          <p>
            API Kurir mengirim perubahan status dan snapshot riwayat ke Emisell.
            Worker membaca pengaturan terbaru tanpa rebuild container.
          </p>
        </div>
        <div className="webhook-hero-status">
          <span
            className={`badge ${
              settings.enabled ? "badge-success" : "badge-warning"
            }`}
          >
            {settings.enabled ? "Aktif" : "Nonaktif"}
          </span>
          <small>
            Sumber {settings.source === "database" ? "dashboard" : ".env lama"}
          </small>
        </div>
      </section>

      <div className="webhook-grid">
        <form className="panel webhook-config-card" onSubmit={submit}>
          <div className="panel-heading">
            <div>
              <p className="eyebrow">TUJUAN WEBHOOK</p>
              <h2>Endpoint Emisell</h2>
            </div>
          </div>
          <label>
            Callback URL
            <input
              type="url"
              value={callbackURL}
              onChange={(event) => setCallbackURL(event.target.value)}
              placeholder="https://api.emisell.com/api/v1/webhooks/tracking"
              required={enabled}
            />
            <small>
              Production wajib HTTPS. Gunakan endpoint backend Emisell, bukan
              alamat dashboard browser.
            </small>
          </label>
          <label className="webhook-toggle">
            <input
              type="checkbox"
              checked={enabled}
              disabled={!settings.secret_configured}
              onChange={(event) => setEnabled(event.target.checked)}
            />
            <span>
              <strong>Aktifkan pengiriman event</strong>
              <small>
                {settings.secret_configured
                  ? "Worker akan mengirim event status yang menunggu."
                  : "Generate secret sebelum webhook dapat diaktifkan."}
              </small>
            </span>
          </label>
          <button className="button button-primary" disabled={loading}>
            {loading ? "Menyimpan…" : "Simpan pengaturan"}
          </button>
        </form>

        <section className="panel webhook-secret-card">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">HMAC SIGNING</p>
              <h2>Webhook secret</h2>
            </div>
            <span
              className={`badge ${
                settings.secret_configured ? "badge-success" : "badge-warning"
              }`}
            >
              {settings.secret_configured ? "Tersimpan" : "Belum dibuat"}
            </span>
          </div>
          {settings.secret_configured && !generatedSecret && (
            <div className="webhook-secret-mask">
              <code>{settings.secret_hint || "Secret terenkripsi"}</code>
              <small>Secret asli tidak dapat ditampilkan kembali.</small>
            </div>
          )}
          {!settings.secret_configured && !generatedSecret && (
            <p className="webhook-muted">
              Generate secret, lalu salin satu kali ke konfigurasi penerima
              webhook di API Emisell.
            </p>
          )}
          {generatedSecret && (
            <div className="webhook-generated-secret">
              <strong>Salin sekarang — hanya ditampilkan sekali</strong>
              <code>{generatedSecret}</code>
              <div className="form-actions">
                <button
                  type="button"
                  className="button button-secondary"
                  onClick={() => void copySecret()}
                >
                  {copied ? "Sudah disalin" : "Salin secret"}
                </button>
                <button
                  type="button"
                  className="table-action"
                  onClick={onDismissSecret}
                >
                  Saya sudah menyimpan
                </button>
              </div>
            </div>
          )}
          <button
            type="button"
            className="button button-secondary"
            disabled={loading}
            onClick={() => {
              if (
                settings.secret_configured &&
                !window.confirm(
                  "Rotate secret akan membuat secret lama tidak berlaku. Lanjutkan?",
                )
              ) {
                return;
              }
              void onGenerateSecret();
            }}
          >
            {settings.secret_configured ? "Rotate secret" : "Generate secret"}
          </button>
        </section>
      </div>

      <section className="panel webhook-test-card">
        <div>
          <p className="eyebrow">VERIFIKASI KONEKSI</p>
          <h2>Test webhook tanpa data pelanggan</h2>
          <p>
            Mengirim event <code>tracking.test</code> bertanda tangan. Nama,
            alamat, nomor resi, dan data order tidak disertakan.
          </p>
        </div>
        <div className="webhook-test-action">
          <button
            type="button"
            className="button button-primary"
            disabled={loading || !canTest}
            onClick={() => void onTest()}
          >
            {loading ? "Menguji…" : "Kirim test webhook"}
          </button>
          {(testResult || settings.last_test_at) && (
            <div
              className={`webhook-test-result ${
                (testResult?.success ?? settings.last_test_success)
                  ? "webhook-test-success"
                  : "webhook-test-failed"
              }`}
            >
              <strong>
                {(testResult?.success ?? settings.last_test_success)
                  ? "Koneksi berhasil"
                  : "Koneksi belum berhasil"}
              </strong>
              <small>
                {testResult?.message ||
                  settings.last_test_error ||
                  `HTTP ${settings.last_test_http_status ?? "—"}`} ·{" "}
                {formatDate(testResult?.tested_at || settings.last_test_at)}
              </small>
            </div>
          )}
        </div>
      </section>

      <section className="webhook-flow">
        <article>
          <span>1</span>
          <strong>Snapshot berubah</strong>
          <small>Worker mendapatkan status baru dari provider.</small>
        </article>
        <article>
          <span>2</span>
          <strong>Event ditandatangani</strong>
          <small>HMAC SHA-256 melindungi isi dan timestamp.</small>
        </article>
        <article>
          <span>3</span>
          <strong>Order Emisell diperbarui</strong>
          <small>Timeline dan status fulfillment bergerak otomatis.</small>
        </article>
      </section>

      <section className="panel webhook-guide-card">
        <div className="webhook-doc-heading">
          <div>
            <p className="eyebrow">PANDUAN INTEGRASI</p>
            <h2>Urutan setup yang aman</h2>
            <p>
              Webhook hanya dipanggil oleh worker API Kurir ke backend Emisell.
              Browser seller tidak perlu mengetahui URL callback maupun secret.
            </p>
          </div>
          <span className="badge">API version 2026-08-20</span>
        </div>
        <div className="webhook-setup-steps">
          <article>
            <span>1</span>
            <div>
              <strong>Buat endpoint penerima</strong>
              <small>
                Siapkan endpoint POST HTTPS di backend Emisell dan pertahankan
                raw request body untuk pemeriksaan signature.
              </small>
            </div>
          </article>
          <article>
            <span>2</span>
            <div>
              <strong>Generate dan simpan secret</strong>
              <small>
                Secret hanya tampil sekali. Simpan di secret manager backend,
                jangan di database seller atau frontend.
              </small>
            </div>
          </article>
          <article>
            <span>3</span>
            <div>
              <strong>Simpan URL lalu kirim test</strong>
              <small>
                Event <code>tracking.test</code> tidak membawa data pelanggan
                dan aman dipakai untuk menguji signature serta respons 2xx.
              </small>
            </div>
          </article>
          <article>
            <span>4</span>
            <div>
              <strong>Aktifkan pengiriman event</strong>
              <small>
                Setelah test berhasil, aktifkan webhook. Event status yang
                menunggu akan diproses oleh worker.
              </small>
            </div>
          </article>
        </div>
      </section>

      <div className="webhook-doc-grid">
        <section className="panel webhook-events-card">
          <div className="webhook-doc-heading">
            <div>
              <p className="eyebrow">EVENT PRODUKSI</p>
              <h2>Kapan event dikirim</h2>
            </div>
          </div>
          <div className="webhook-event-list">
            <article>
              <code>tracking.validated</code>
              <p>Resi berhasil dikonfirmasi provider dan cocok dengan kurir.</p>
            </article>
            <article>
              <code>tracking.status_changed</code>
              <p>
                Status canonical berubah. Gunakan <code>shipment.status</code>
                untuk memperbarui timeline fulfillment.
              </p>
            </article>
            <article>
              <code>tracking.delivered</code>
              <p>
                Paket sampai tujuan. Fulfillment boleh menjadi delivered;
                order selesai hanya jika semua fulfillment sudah delivered.
              </p>
            </article>
            <article>
              <code>tracking.invalid</code>
              <p>
                Resi dinyatakan tidak valid setelah proses validasi. Tampilkan
                peringatan ke seller tanpa mengubah status order otomatis.
              </p>
            </article>
          </div>
        </section>

        <section className="panel webhook-status-card">
          <div className="webhook-doc-heading">
            <div>
              <p className="eyebrow">STATUS CANONICAL</p>
              <h2>Nilai shipment.status</h2>
            </div>
          </div>
          <div className="webhook-status-list">
            <span><code>unknown</code> belum diketahui</span>
            <span><code>pending_pickup</code> menunggu pickup</span>
            <span><code>picked_up</code> sudah diambil kurir</span>
            <span><code>in_transit</code> dalam perjalanan</span>
            <span><code>out_for_delivery</code> dibawa kurir tujuan</span>
            <span><code>delivery_failed</code> pengantaran gagal</span>
            <span><code>delivered</code> terkirim, status final</span>
            <span><code>returned</code> dikembalikan, status final</span>
            <span><code>cancelled</code> dibatalkan, status final</span>
          </div>
        </section>
      </div>

      <section className="panel webhook-payload-card">
        <div className="webhook-doc-heading">
          <div>
            <p className="eyebrow">HTTP CONTRACT</p>
            <h2>Header dan payload yang diterima Emisell</h2>
            <p>
              Nomor resi dikirim utuh hanya ke backend Emisell. Tetap cocokkan
              order menggunakan
              <code>merchant_id</code>, <code>order_id</code>, dan
              <code>fulfillment_id</code>, bukan menggunakan nomor resi.
            </p>
            <p>
              <code>shipped_at</code> adalah waktu paket diserahkan ke kurir.
              <code>delivered_at</code> tetap <code>null</code> sampai provider
              mengonfirmasi paket diterima. Keduanya memakai waktu manifest/POD,
              bukan waktu polling API Kurir.
            </p>
          </div>
        </div>
        <div className="code-grid webhook-code-grid">
          <CodeExample title="Header request" content={WEBHOOK_REQUEST_HEADERS} />
          <CodeExample
            title="Payload tracking.status_changed"
            content={WEBHOOK_TRACKING_PAYLOAD}
          />
          <CodeExample title="Payload tracking.test" content={WEBHOOK_TEST_PAYLOAD} />
          <CodeExample
            title="Verifikasi HMAC · Node.js"
            content={WEBHOOK_NODE_VERIFICATION}
          />
        </div>
      </section>

      <div className="webhook-doc-grid">
        <section className="panel webhook-security-card">
          <div className="webhook-doc-heading">
            <div>
              <p className="eyebrow">VERIFIKASI WAJIB</p>
              <h2>Signature, replay, dan urutan event</h2>
            </div>
          </div>
          <ol className="webhook-rules">
            <li>
              Hitung HMAC SHA-256 atas
              <code>{"<timestamp>.<raw-request-body>"}</code> menggunakan
              webhook secret.
            </li>
            <li>
              Bandingkan dengan header signature secara constant-time dan
              tolak timestamp yang lebih lama dari lima menit.
            </li>
            <li>
              Simpan <code>X-Emisell-Event-ID</code> sebagai idempotency key;
              event duplikat harus tetap dibalas 2xx tanpa diproses ulang.
            </li>
            <li>
              Abaikan <code>tracking_revision</code> yang lebih kecil daripada
              revision aktif atau snapshot yang lebih lama dari data saat ini.
            </li>
          </ol>
        </section>

        <section className="panel webhook-delivery-card">
          <div className="webhook-doc-heading">
            <div>
              <p className="eyebrow">RESPONS & RETRY</p>
              <h2>Balas cepat dengan status 2xx</h2>
            </div>
          </div>
          <ul className="webhook-rules">
            <li><code>200/202/204</code> dianggap berhasil dan tidak diulang.</li>
            <li>
              Network error, <code>429</code>, dan <code>5xx</code> diulang
              setelah 1 menit, 5 menit, 30 menit, 2 jam, 6 jam, lalu 24 jam.
            </li>
            <li>
              Respons <code>4xx</code> selain 429 dianggap penolakan permanen
              dan dipindahkan ke dead-letter.
            </li>
            <li>
              Verifikasi, simpan event, lalu balas 2xx. Proses order yang berat
              sebaiknya dilanjutkan melalui antrean internal Emisell.
            </li>
          </ul>
        </section>
      </div>
    </div>
  );
}

type CourierCapabilityFilter = "all" | "tracking" | "international";

function CourierCatalog({
  couriers,
  onCheckRate,
}: {
  couriers: Courier[];
  onCheckRate: () => void;
}) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<CourierCapabilityFilter>("all");

  const visibleCouriers = useMemo(() => {
    const normalizedSearch = search.trim().toLowerCase();
    return couriers.filter((courier) => {
      if (filter === "tracking" && !courier.supports_tracking) return false;
      if (
        filter === "international" &&
        !courier.supports_international_cost
      ) {
        return false;
      }
      if (!normalizedSearch) return true;
      return [
        courier.code,
        courier.name,
        courier.provider_code,
        courier.rate_provider_code,
        courier.tracking_provider_code,
        ...courier.services.flatMap((service) => [
          service.code,
          service.name,
          service.group,
          service.service_type,
        ]),
      ]
        .join(" ")
        .toLowerCase()
        .includes(normalizedSearch);
    });
  }, [couriers, filter, search]);

  const trackingCount = couriers.filter(
    (courier) => courier.supports_tracking,
  ).length;
  const internationalCount = couriers.filter(
    (courier) => courier.supports_international_cost,
  ).length;

  return (
    <div className="courier-page">
      <section className="courier-hero">
        <div>
          <p className="eyebrow">MASTER EKSPEDISI</p>
          <h2>Kurir yang siap dipakai dari satu API</h2>
          <p>
            Katalog mengikuti kemampuan provider. Layanan dan tarif aktual
            tetap dikembalikan berdasarkan rute yang dipilih saat cek ongkir.
          </p>
          <button className="button courier-hero-button" onClick={onCheckRate}>
            Coba cek ongkir
          </button>
        </div>
        <div className="courier-summary-grid">
          <article>
            <strong>{formatNumber(couriers.length)}</strong>
            <span>Ekspedisi terdaftar</span>
          </article>
          <article>
            <strong>{formatNumber(trackingCount)}</strong>
            <span>Mendukung cek resi</span>
          </article>
          <article>
            <strong>{formatNumber(internationalCount)}</strong>
            <span>Mendukung internasional</span>
          </article>
        </div>
      </section>

      <section className="courier-toolbar">
        <label className="courier-search">
          <span>Cari ekspedisi atau layanan</span>
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Contoh: JNE, cargo, REG…"
          />
        </label>
        <div className="courier-filters" aria-label="Filter kemampuan">
          {(
            [
              ["all", "Semua"],
              ["tracking", "Bisa cek resi"],
              ["international", "Internasional"],
            ] as [CourierCapabilityFilter, string][]
          ).map(([value, label]) => (
            <button
              type="button"
              className={filter === value ? "filter-active" : ""}
              key={value}
              onClick={() => setFilter(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <span className="courier-result-count">
          {formatNumber(visibleCouriers.length)} ekspedisi
        </span>
      </section>

      {visibleCouriers.length ? (
        <section className="courier-card-grid">
          {visibleCouriers.map((courier) => (
            <article className="courier-card" key={courier.code}>
              <header>
                <span className="courier-card-mark">
                  <CourierLogo
                    source={courier.logo}
                    code={courier.code}
                    alt={`Logo ${courier.name}`}
                  />
                </span>
                <div>
                  <span className="courier-provider">
                    {courier.rate_provider_code
                      ? `Ongkir · ${courier.rate_provider_code}`
                      : "Tracking only"}
                  </span>
                  <h3>{courier.name}</h3>
                  <code>{courier.code}</code>
                </div>
              </header>

              <div className="courier-capabilities">
                {courier.supports_domestic_cost && (
                  <span className="capability-on">Cek ongkir</span>
                )}
                {courier.supports_tracking ? (
                  <span className="capability-on">
                    Cek resi · {courier.tracking_provider_code}
                  </span>
                ) : (
                  <span className="capability-off">Resi belum tersedia</span>
                )}
                {courier.supports_international_cost && (
                  <span className="capability-special">Internasional</span>
                )}
              </div>

              <div className="courier-services">
                <div className="courier-section-title">
                  <span>Layanan master tersedia</span>
                  <strong>{courier.services.length}</strong>
                </div>
                {courier.services.length ? (
                  <div className="courier-service-list">
                    {courier.services.map((service) => (
                      <div key={`${courier.code}-${service.code}`}>
                        <span>
                          <strong>{service.code}</strong>
                          {service.name}
                        </span>
                        <small>
                          {service.group.replaceAll("_", " ")} ·{" "}
                          {service.service_type.replaceAll("_", " ")} ·{" "}
                          {service.calculation_mode === "local_rate_card"
                            ? "tarif lokal"
                            : "quote provider"}
                        </small>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p>
                    {courier.supports_tracking &&
                    !courier.supports_domestic_cost
                      ? "Tracking tersedia melalui provider fallback. Cek ongkir belum diaktifkan."
                      : "Provider belum pernah mengembalikan layanan pada rute yang dicek. Master akan terisi otomatis saat layanan pertama tersedia."}
                  </p>
                )}
              </div>

              <footer>
                <span>
                  Diverifikasi {formatCatalogDate(courier.catalog_verified_at)}
                </span>
                {courier.catalog_source && (
                  <a
                    href={courier.catalog_source}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Sumber katalog ↗
                  </a>
                )}
                {courier.tracking_catalog_source &&
                  courier.tracking_catalog_source !== courier.catalog_source && (
                    <a
                      href={courier.tracking_catalog_source}
                      target="_blank"
                      rel="noreferrer"
                    >
                      Sumber tracking ↗
                    </a>
                  )}
              </footer>
            </article>
          ))}
        </section>
      ) : (
        <section className="courier-empty">
          <strong>Ekspedisi tidak ditemukan</strong>
          <p>Coba kata kunci atau filter kemampuan yang berbeda.</p>
        </section>
      )}
    </div>
  );
}

function ShippingCostTool({
  api,
  couriers,
  onError,
}: {
  api: AdminApi;
  couriers: Courier[];
  onError: (message: string) => void;
}) {
  const [origin, setOrigin] = useState<LocationOption | null>(null);
  const [destination, setDestination] = useState<LocationOption | null>(null);
  const [weight, setWeight] = useState("1000");
  const [courier, setCourier] = useState("jne");
  const [results, setResults] = useState<RateResult[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!couriers.length) return;
    if (!couriers.some((item) => item.code === courier)) {
      setCourier(couriers[0].code);
    }
  }, [courier, couriers]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    onError("");
    if (!origin || !destination) {
      onError("Pilih lokasi asal dan tujuan dari hasil pencarian.");
      return;
    }
    if (origin.id === destination.id) {
      onError("Lokasi asal dan tujuan tidak boleh sama.");
      return;
    }
    const weightGrams = Number(weight);
    if (!Number.isInteger(weightGrams) || weightGrams < 1) {
      onError("Berat harus berupa gram dengan nilai minimal 1.");
      return;
    }

    setLoading(true);
    try {
      const data = await api.calculateDomesticCost({
        origin: origin.id,
        destination: destination.id,
        weight: weightGrams,
        courier,
      });
      setResults([...data].sort((left, right) => left.cost - right.cost));
    } catch (requestError) {
      setResults([]);
      onError(getErrorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }

  function swapLocations() {
    setOrigin(destination);
    setDestination(origin);
  }

  return (
    <div className="tool-page">
      <section className="tool-hero tool-hero-rate">
        <div className="tool-intro">
          <p className="eyebrow">KALKULATOR PENGIRIMAN</p>
          <h2>Cek tarif terbaik sekarang</h2>
          <p>
            Cari wilayah lokal sampai kode pos. Sistem memakai snapshot yang
            masih aktif atau mengambil tarif provider secara otomatis saat
            rute belum tersedia.
          </p>
          <div className="tool-assurance">
            <span>Wilayah lokal</span>
            <span>Tarif terurut</span>
            <span>Tanpa input mapping</span>
          </div>
        </div>

        <form className="tool-form-card" onSubmit={submit}>
          <div className="tool-form-heading">
            <div>
              <span>Rute pengiriman</span>
              <strong>Asal dan tujuan paket</strong>
            </div>
            <div className="tool-route-mark" aria-hidden="true">
              <span />
              <i />
              <span />
            </div>
          </div>

          <div className="tool-location-grid">
            <LocationPicker
              api={api}
              label="Dari"
              placeholder="Cari kota, kecamatan, kelurahan, atau kode pos"
              selected={origin}
              onSelect={setOrigin}
              excludeID={destination?.id}
            />
            <button
              type="button"
              className="tool-swap"
              onClick={swapLocations}
              disabled={!origin && !destination}
              aria-label="Tukar lokasi asal dan tujuan"
            >
              ⇄
            </button>
            <LocationPicker
              api={api}
              label="Tujuan"
              placeholder="Cari tujuan pengiriman"
              selected={destination}
              onSelect={setDestination}
              excludeID={origin?.id}
            />
          </div>

          <div className="tool-details-grid">
            <label>
              Berat final paket
              <div className="input-suffix">
                <input
                  type="number"
                  min="1"
                  step="1"
                  value={weight}
                  onChange={(event) => setWeight(event.target.value)}
                  placeholder="1000"
                  required
                />
                <span>gram</span>
              </div>
            </label>
            <label>
              Ekspedisi
              <select
                value={courier}
                onChange={(event) => setCourier(event.target.value)}
                disabled={!couriers.length}
              >
                {couriers.map((item) => (
                  <option key={item.code} value={item.code}>
                    {item.name}
                    {item.services.length
                      ? ` (${item.services.length} layanan lokal)`
                      : ""}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <button
            className="button tool-submit"
            disabled={loading || !origin || !destination || !courier}
          >
            {loading ? "Mencari tarif…" : "Cek Ongkir"}
          </button>
        </form>
      </section>

      <section className="tool-results">
        <div className="tool-results-heading">
          <div>
            <p className="eyebrow">HASIL CEK ONGKIR</p>
            <h2>
              {results.length
                ? `${results.length} layanan ditemukan`
                : "Tarif akan tampil di sini"}
            </h2>
          </div>
          {origin && destination && (
            <span>
              {origin.subdistrict} → {destination.subdistrict} ·{" "}
              {formatNumber(Number(weight) || 0)} gram
            </span>
          )}
        </div>

        {results.length ? (
          <div className="rate-result-grid">
            {results.map((result) => (
              <article
                className="rate-result-card"
                key={`${result.courier.code}-${result.service.code}`}
              >
                <div className="rate-result-top">
                  <div className="courier-avatar">
                    {result.courier.code.slice(0, 3).toUpperCase()}
                  </div>
                  <div>
                    <strong>{result.courier.name}</strong>
                    <span>
                      {result.service.name} · {result.service.code}
                    </span>
                  </div>
                  <span className="rate-result-price">
                    {formatMoney(result.cost)}
                  </span>
                </div>
                <div className="rate-result-meta">
                  <div>
                    <span>Estimasi</span>
                    <strong>
                      {formatETD(
                        result.etd.min_days,
                        result.etd.max_days,
                      )}
                    </strong>
                  </div>
                  <div>
                    <span>Berat tagihan</span>
                    <strong>
                      {formatNumber(result.weight.billing_grams)} gram
                    </strong>
                  </div>
                  <div>
                    <span>Sumber</span>
                    <strong>{result.source.provider || "lokal"}</strong>
                  </div>
                </div>
                <div className="rate-result-footer">
                  <span className="badge">
                    {result.service.group.replaceAll("_", " ")}
                  </span>
                  {result.service.canonical_code !== result.service.code && (
                    <span className="badge">
                      canonical {result.service.canonical_code}
                    </span>
                  )}
                  <span className="badge">{result.source.type}</span>
                  <span
                    className={`badge ${
                      result.source.verification_status ===
                      "official_contract"
                        ? "badge-success"
                        : "badge-warning"
                    }`}
                  >
                    {result.source.verification_status}
                  </span>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="tool-empty">
            <div className="tool-empty-icon" aria-hidden="true">
              ↗
            </div>
            <strong>Pilih rute, berat, dan ekspedisi</strong>
            <p>
              Tarif exact-match yang sudah tersimpan tidak mengurangi hit
              provider.
            </p>
          </div>
        )}
      </section>
    </div>
  );
}

function TrackingOperationsTable({
  api,
  couriers,
  onError,
}: {
  api: AdminApi;
  couriers: Courier[];
  onError: (message: string) => void;
}) {
  const [search, setSearch] = useState("");
  const [courier, setCourier] = useState("");
  const [validationStatus, setValidationStatus] = useState("");
  const [queueStatus, setQueueStatus] = useState("");
  const [loading, setLoading] = useState(true);
  const [deletingID, setDeletingID] = useState("");
  const [page, setPage] = useState<TrackingOperationPage>({
    items: [], total: 0,
    summary: { total: 0, pending: 0, running: 0, failed: 0, invalid: 0, final: 0 },
  });

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await api.trackingOperations({
        search, courier, validation_status: validationStatus,
        queue_status: queueStatus, limit: 100,
      }, signal);
      setPage(result);
    } catch (requestError) {
      if (requestError instanceof Error && requestError.name === "AbortError") return;
      onError(getErrorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }, [api, courier, onError, queueStatus, search, validationStatus]);

  const removePermanently = useCallback(async (item: TrackingOperation) => {
    const confirmed = window.confirm(
      `Hapus permanen resi ${item.courier.toUpperCase()} · ${item.waybill}?\n\n` +
      "Antrean, snapshot history, subscription, revision, dan webhook terkait ikut dihapus. Tindakan ini tidak dapat dibatalkan.",
    );
    if (!confirmed) return;
    setDeletingID(item.id);
    onError("");
    try {
      await api.deleteTrackingOperation(item.id);
      await load();
    } catch (requestError) {
      onError(getErrorMessage(requestError));
    } finally {
      setDeletingID("");
    }
  }, [api, load, onError]);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    const debounce = window.setTimeout(() => void load(controller.signal), 250);
    const refresh = window.setInterval(() => void load(controller.signal), 15_000);
    return () => {
      window.clearTimeout(debounce);
      window.clearInterval(refresh);
      controller.abort();
    };
  }, [load]);

  return (
    <div className="tracking-operations-page">
      <section className="tracking-monitor-summary">
        <TrackingMonitorMetric label="Semua resi" value={page.summary.total} tone="neutral" />
        <TrackingMonitorMetric label="Menunggu" value={page.summary.pending} tone="warning" />
        <TrackingMonitorMetric label="Diproses worker" value={page.summary.running} tone="success" />
        <TrackingMonitorMetric label="Gagal / dead" value={page.summary.failed} tone="danger" />
        <TrackingMonitorMetric label="Status final" value={page.summary.final} tone="neutral" />
      </section>

      <section className="toolbar tracking-monitor-toolbar">
        <input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Cari merchant, order, fulfillment, resi, atau provider…"
        />
        <select value={courier} onChange={(event) => setCourier(event.target.value)}>
          <option value="">Semua ekspedisi</option>
          {couriers.filter((item) => item.supports_tracking).map((item) => (
            <option key={item.code} value={item.code}>{item.name}</option>
          ))}
        </select>
        <select value={validationStatus} onChange={(event) => setValidationStatus(event.target.value)}>
          <option value="">Semua validasi</option>
          <option value="unverified">Belum diverifikasi</option>
          <option value="valid">Valid</option>
          <option value="not_found">Belum ditemukan</option>
          <option value="invalid">Tidak valid</option>
        </select>
        <select value={queueStatus} onChange={(event) => setQueueStatus(event.target.value)}>
          <option value="">Semua antrean</option>
          <option value="pending">Menunggu</option>
          <option value="running">Sedang diproses</option>
          <option value="dead">Gagal / dead</option>
          <option value="final">Selesai</option>
          <option value="idle">Tidak mengantre</option>
        </select>
        <button className="button button-secondary" onClick={() => void load()} disabled={loading}>
          {loading ? "Memuat…" : "Muat ulang"}
        </button>
        <span className="subtle">Otomatis diperbarui setiap 15 detik · {formatNumber(page.total)} hasil</span>
      </section>

      <section className="panel panel-table tracking-monitor-table">
        <div className="table-scroll">
          <table>
            <thead><tr>
              <th>Resi</th><th>Merchant / order</th><th>Validasi</th><th>Status kiriman</th>
              <th>Antrean worker</th><th>Provider / hit</th><th>Jadwal</th><th>Diperbarui</th>
              <th>Aksi</th>
            </tr></thead>
            <tbody>
              {page.items.map((item) => <TrackingOperationRow
                key={item.id}
                item={item}
                deleting={deletingID === item.id}
                onRemove={() => void removePermanently(item)}
              />)}
              {!page.items.length && <tr><td colSpan={9} className="empty-state">
                {loading ? "Memuat data operasional resi…" : "Belum ada resi yang sesuai filter."}
              </td></tr>}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function TrackingMonitorMetric({ label, value, tone }: { label: string; value: number; tone: string }) {
  return <article className={`tracking-monitor-metric tracking-monitor-${tone}`}>
    <span>{label}</span><strong>{formatNumber(value)}</strong>
  </article>;
}

function TrackingOperationRow({
  item,
  deleting,
  onRemove,
}: {
  item: TrackingOperation;
  deleting: boolean;
  onRemove: () => void;
}) {
  const queueLabel: Record<string, string> = {
    pending: "menunggu", running: "diproses", completed: "selesai job",
    dead: "gagal", final: "final", idle: "idle",
  };
  const validationLabel: Record<string, string> = {
    unverified: "belum dicek", valid: "valid", not_found: "belum ditemukan", invalid: "tidak valid",
  };
  const queueTone = item.queue_status === "running" ? "badge-success" :
    item.queue_status === "pending" ? "badge-warning" :
    item.queue_status === "dead" ? "badge-danger" : "";
  const validationTone = item.validation_status === "valid" ? "badge-success" :
    item.validation_status === "invalid" ? "badge-danger" : "badge-warning";
  return <tr>
    <td><strong>{item.courier.toUpperCase()} · {item.waybill}</strong><small>ID {item.id.slice(0, 8)}</small></td>
    <td><strong>{item.tenant_id || "Platform"}</strong><small>{item.order_id || "Tanpa order"} · {item.fulfillment_id || "tanpa fulfillment"}</small>{Boolean(item.subscription_revision) && <small>Revisi {item.subscription_revision} · {item.revision_history_count} diganti</small>}</td>
    <td><span className={`badge ${validationTone}`}>{validationLabel[item.validation_status] || item.validation_status}</span>{item.last_error_code && <small>{item.last_error_code}</small>}</td>
    <td><strong>{item.status_label || item.status.replaceAll("_", " ")}</strong><small>{item.is_final ? "Status final" : item.status}</small></td>
    <td><span className={`badge ${queueTone}`}>{queueLabel[item.queue_status] || item.queue_status}</span><small>Percobaan {item.job_attempt_count}/{item.job_max_attempts || "—"}{item.job_locked_by ? ` · ${item.job_locked_by}` : ""}</small></td>
    <td><strong>{item.provider || "Belum dipilih"}</strong><small>{item.provider_hit_count}/{item.provider_hit_limit} hit AWB</small></td>
    <td><strong>{item.queue_status === "running" ? "Sekarang" : formatDate(item.job_available_at || item.next_refresh_at)}</strong><small>Snapshot {formatDate(item.provider_fetched_at)}</small></td>
    <td><strong>{formatDate(item.updated_at)}</strong><small>Masuk {formatDate(item.created_at)}</small></td>
    <td><button
      className="table-action table-action-danger"
      disabled={deleting}
      onClick={onRemove}
    >{deleting ? "Menghapus…" : "Hapus permanen"}</button></td>
  </tr>;
}

function FulfillmentOperationsTable({
	api,
	onError,
}: {
	api: AdminApi;
	onError: (message: string) => void;
}) {
	const [search, setSearch] = useState("");
	const [provider, setProvider] = useState("");
	const [status, setStatus] = useState("");
	const [queueStatus, setQueueStatus] = useState("");
	const [loading, setLoading] = useState(true);
	const [refreshingID, setRefreshingID] = useState("");
	const [page, setPage] = useState<FulfillmentOperationPage>({
		items: [], total: 0,
		summary: { total: 0, booking_pending: 0, tracking_pending: 0, failed: 0, final: 0 },
	});

	const load = useCallback(async (signal?: AbortSignal) => {
		try {
			const result = await api.fulfillmentOperations({
				search, provider, status, queue_status: queueStatus, limit: 100,
			}, signal);
			setPage(result);
		} catch (requestError) {
			if (requestError instanceof Error && requestError.name === "AbortError") return;
			onError(getErrorMessage(requestError));
		} finally {
			setLoading(false);
		}
	}, [api, onError, provider, queueStatus, search, status]);

	const reconcile = useCallback(async (item: FulfillmentOperation) => {
		setRefreshingID(item.shipment_id);
		onError("");
		try {
			await api.reconcileFulfillment(item.shipment_id);
			await load();
		} catch (requestError) {
			onError(getErrorMessage(requestError));
		} finally {
			setRefreshingID("");
		}
	}, [api, load, onError]);

	useEffect(() => {
		const controller = new AbortController();
		setLoading(true);
		const debounce = window.setTimeout(() => void load(controller.signal), 250);
		const refresh = window.setInterval(() => void load(controller.signal), 15_000);
		return () => {
			window.clearTimeout(debounce);
			window.clearInterval(refresh);
			controller.abort();
		};
	}, [load]);

	return (
		<div className="tracking-operations-page">
			<section className="tracking-monitor-summary">
				<TrackingMonitorMetric label="Semua shipment" value={page.summary.total} tone="neutral" />
				<TrackingMonitorMetric label="Booking diproses" value={page.summary.booking_pending} tone="warning" />
				<TrackingMonitorMetric label="AWB ke tracking" value={page.summary.tracking_pending} tone="warning" />
				<TrackingMonitorMetric label="Perlu perhatian" value={page.summary.failed} tone="danger" />
				<TrackingMonitorMetric label="Status final" value={page.summary.final} tone="success" />
			</section>

			<section className="toolbar tracking-monitor-toolbar">
				<input value={search} onChange={(event) => setSearch(event.target.value)}
					placeholder="Cari merchant, order, shipment, AWB, atau provider…" />
				<select value={provider} onChange={(event) => setProvider(event.target.value)}>
					<option value="">Semua provider</option>
					<option value="rajaongkir">RajaOngkir</option>
				</select>
				<select value={status} onChange={(event) => setStatus(event.target.value)}>
					<option value="">Semua status</option>
					<option value="booked">Booked</option>
					<option value="pickup_requested">Pickup requested</option>
					<option value="in_transit">In transit</option>
					<option value="delivered">Delivered</option>
					<option value="cancelled">Cancelled</option>
					<option value="booking_failed">Booking failed</option>
				</select>
				<select value={queueStatus} onChange={(event) => setQueueStatus(event.target.value)}>
					<option value="">Semua antrean</option>
					<option value="pending">Menunggu</option>
					<option value="running">Diproses</option>
					<option value="dead">Gagal</option>
					<option value="final">Final</option>
					<option value="idle">Idle</option>
				</select>
				<button className="button button-secondary" onClick={() => void load()} disabled={loading}>
					{loading ? "Memuat…" : "Muat ulang"}
				</button>
				<span className="subtle">Otomatis diperbarui setiap 15 detik · {formatNumber(page.total)} hasil</span>
			</section>

			<section className="panel panel-table tracking-monitor-table">
				<div className="table-scroll">
					<table>
						<thead><tr>
							<th>Shipment</th><th>Merchant / order</th><th>Provider</th><th>Pengiriman</th>
							<th>Status</th><th>Tracking</th><th>Antrean</th><th>Webhook</th><th>Aksi</th>
						</tr></thead>
						<tbody>
							{page.items.map((item) => (
								<FulfillmentOperationRow key={item.shipment_id} item={item}
									refreshing={refreshingID === item.shipment_id}
									onReconcile={() => void reconcile(item)} />
							))}
							{!page.items.length && <tr><td colSpan={9} className="empty-state">
								{loading ? "Memuat lifecycle fulfillment…" : "Belum ada shipment yang sesuai filter."}
							</td></tr>}
						</tbody>
					</table>
				</div>
			</section>
		</div>
	);
}

function FulfillmentOperationRow({
	item,
	refreshing,
	onReconcile,
}: {
	item: FulfillmentOperation;
	refreshing: boolean;
	onReconcile: () => void;
}) {
	const final = item.status === "delivered" || item.status === "cancelled";
	const queueTone = item.queue_status === "running" ? "badge-success" :
		item.queue_status === "pending" ? "badge-warning" :
		item.queue_status === "dead" ? "badge-danger" : "";
	const trackingTone = item.tracking_registration_status === "registered" ? "badge-success" :
		item.tracking_registration_status === "failed" ? "badge-danger" : "badge-warning";
	return <tr>
		<td><strong>{item.shipment_id.slice(0, 8)}</strong><small>{item.provider_shipment_id || "Belum ada order provider"}</small></td>
		<td><strong>{item.merchant_id}</strong><small>{item.order_id}</small></td>
		<td><strong>{item.provider}</strong><small>{item.provider_status || "Belum direkonsiliasi"}</small></td>
		<td><strong>{item.courier.toUpperCase()} · {item.service}</strong><small>{item.waybill || "AWB belum tersedia"}</small></td>
		<td><span className={`badge ${final ? "badge-success" : ""}`}>{item.status.replaceAll("_", " ")}</span><small>Update {formatDate(item.updated_at)}</small></td>
		<td><span className={`badge ${trackingTone}`}>{item.tracking_registration_status.replaceAll("_", " ")}</span><small>{item.tracking_status || "Belum ada snapshot"}</small></td>
		<td><span className={`badge ${queueTone}`}>{item.queue_status}</span><small>{item.job_type || "tanpa job"} · {item.job_attempt_count}/{item.job_max_attempts || "—"}</small>{item.reconcile_error && <small>{item.reconcile_error}</small>}</td>
		<td><strong>{item.webhook_status}</strong><small>Reconcile {formatDate(item.last_reconciled_at || null)}</small></td>
		<td><button className="table-action" disabled={refreshing || final || !item.provider_shipment_id}
			onClick={onReconcile}>{refreshing ? "Mengantre…" : "Refresh provider"}</button></td>
	</tr>;
}

function TrackingTool({
  api,
  couriers,
  onError,
}: {
  api: AdminApi;
  couriers: Courier[];
  onError: (message: string) => void;
}) {
  const [waybill, setWaybill] = useState("");
  const [courier, setCourier] = useState("jne");
  const [result, setResult] = useState<TrackingShipment | null>(null);
  const [loading, setLoading] = useState(false);

  const trackingCouriers = useMemo(
    () =>
      couriers
        .filter((item) => item.supports_tracking)
        .sort((left, right) => left.name.localeCompare(right.name, "id")),
    [couriers],
  );

  useEffect(() => {
    if (
      trackingCouriers.length > 0 &&
      !trackingCouriers.some((item) => item.code === courier)
    ) {
      setCourier(trackingCouriers[0].code);
    }
  }, [courier, trackingCouriers]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    onError("");
    if (waybill.trim().length < 6) {
      onError("Nomor resi minimal 6 karakter.");
      return;
    }
    setLoading(true);
    try {
      const request = {
        waybill: waybill.trim(),
        courier,
      };
      let latest = await api.trackWaybill(request);
      setResult(latest);

      for (
        let attempt = 0;
        attempt < 8 && latest.refresh_queued && !latest.last_error_code;
        attempt += 1
      ) {
        await new Promise((resolve) => window.setTimeout(resolve, 1_250));
        latest = await api.trackWaybill(request);
        setResult(latest);
      }
    } catch (requestError) {
      setResult(null);
      onError(getErrorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }

  const summaryEntries = result
    ? Object.entries(result.summary).filter(
        ([, value]) =>
          typeof value === "string" ||
          typeof value === "number" ||
          typeof value === "boolean",
      )
    : [];

  return (
    <div className="tool-page">
      <section className="tool-hero tool-hero-tracking">
        <div className="tool-intro">
          <p className="eyebrow">PELACAKAN PENGIRIMAN</p>
          <h2>Lacak perjalanan paket</h2>
          <p>
            Satu resi disimpan satu kali, lalu statusnya dibaca dari snapshot
            lokal. Worker hanya meminta pembaruan provider ketika data perlu
            disegarkan.
          </p>
          <div className="tracking-route-visual" aria-hidden="true">
            <span className="tracking-pin">●</span>
            <i />
            <span className="tracking-box">EK</span>
          </div>
        </div>

        <form className="tool-form-card" onSubmit={submit}>
          <div className="tool-form-heading">
            <div>
              <span>Informasi paket</span>
              <strong>Masukkan resi pengiriman</strong>
            </div>
            <span className="tool-status-chip">Snapshot-first</span>
          </div>
          <label>
            Nomor resi
            <input
              value={waybill}
              onChange={(event) => setWaybill(event.target.value.toUpperCase())}
              placeholder="Contoh: JP1234567890"
              autoComplete="off"
              required
            />
          </label>
          <label>
            Ekspedisi
            <select
              value={courier}
              onChange={(event) => {
                setCourier(event.target.value);
                setResult(null);
              }}
            >
              {trackingCouriers.map((item) => (
                <option key={item.code} value={item.code}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <button
            className="button tool-submit"
            disabled={loading || !waybill.trim() || !courier}
          >
            {loading ? "Memeriksa resi…" : result ? "Cek Lagi" : "Cek Resi"}
          </button>
        </form>
      </section>

      <section className="tracking-result-panel">
        <div className="tool-results-heading">
          <div>
            <p className="eyebrow">STATUS PENGIRIMAN</p>
            <h2>{result ? trackingStatusText(result) : "Belum ada resi diperiksa"}</h2>
          </div>
          {result && (
            <span className="tracking-waybill">
              {result.courier.toUpperCase()} · {result.waybill}
            </span>
          )}
        </div>

        {result ? (
          <>
            <div className="tracking-status-card">
              <div
                className={`tracking-status-dot ${
                  result.is_final ? "tracking-status-final" : ""
                }`}
              />
              <div>
                <span>Status saat ini</span>
                <strong>{trackingStatusText(result)}</strong>
                <p>
                  {result.last_error_code
                    ? trackingErrorMessage(result.last_error_code)
                    : result.refresh_queued
                    ? "Pembaruan sedang diproses worker. Halaman ini menunggu hasilnya secara otomatis."
                    : result.provider_fetched_at
                      ? `Diperbarui ${formatDate(result.provider_fetched_at)} dari ${result.provider || "provider"}.`
                      : "Status provider belum tersedia."}
                </p>
              </div>
              <span
                className={`badge ${
                  result.last_error_code || result.refresh_queued
                    ? "badge-warning"
                    : "badge-success"
                }`}
              >
                {result.last_error_code
                  ? "perlu perhatian"
                  : result.refresh_queued
                    ? "refresh queued"
                    : "snapshot aktif"}
              </span>
            </div>

            {summaryEntries.length > 0 && (
              <div className="tracking-summary-grid">
                {summaryEntries.slice(0, 8).map(([key, value]) => (
                  <div key={key}>
                    <span>{key.replaceAll("_", " ")}</span>
                    <strong>{String(value) || "—"}</strong>
                  </div>
                ))}
              </div>
            )}

            <div className="tracking-timeline">
              <div className="tracking-timeline-heading">
                <strong>Riwayat perjalanan</strong>
                <span>{result.events.length} pembaruan</span>
              </div>
              {result.events.length ? (
                result.events.map((event, index) => (
                  <article
                    key={`${event.occurred_at}-${event.code}-${index}`}
                  >
                    <i />
                    <div>
                      <strong>{event.description}</strong>
                      <span>
                        {event.location || "Lokasi tidak dicantumkan"} ·{" "}
                        {formatDate(event.occurred_at)}
                      </span>
                    </div>
                  </article>
                ))
              ) : (
                <div className="tracking-empty-events">
                  Belum ada perjalanan pada snapshot. Pembaruan akan muncul
                  setelah worker menerima data provider.
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="tool-empty">
            <div className="tool-empty-icon" aria-hidden="true">
              ◎
            </div>
            <strong>Masukkan resi dan pilih ekspedisi</strong>
            <p>
              Nomor resi ditampilkan termasking dan plaintext-nya disimpan
              terenkripsi.
            </p>
          </div>
        )}
      </section>
    </div>
  );
}

function LocationPicker({
  api,
  label,
  placeholder,
  selected,
  onSelect,
  excludeID,
}: {
  api: AdminApi;
  label: string;
  placeholder: string;
  selected: LocationOption | null;
  onSelect: (location: LocationOption | null) => void;
  excludeID?: string;
}) {
  const [query, setQuery] = useState(selected?.label ?? "");
  const [options, setOptions] = useState<LocationOption[]>([]);
  const [searching, setSearching] = useState(false);

  useEffect(() => {
    if (selected) setQuery(selected.label);
  }, [selected]);

  useEffect(() => {
    const normalized = query.trim();
    if (normalized.length < 2 || normalized === selected?.label) {
      setOptions([]);
      setSearching(false);
      return;
    }

    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setSearching(true);
      void api
        .locations(normalized, controller.signal)
        .then((items) => {
          setOptions(
            items.filter((item) => !excludeID || item.id !== excludeID),
          );
        })
        .catch((requestError) => {
          if (
            !(requestError instanceof Error) ||
            requestError.name !== "AbortError"
          ) {
            setOptions([]);
          }
        })
        .finally(() => setSearching(false));
    }, 250);

    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [api, excludeID, query, selected?.label]);

  return (
    <label className="tool-location-picker">
      {label}
      <input
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          onSelect(null);
        }}
        placeholder={placeholder}
        autoComplete="off"
        aria-expanded={options.length > 0}
      />
      {searching && <small className="selected-id">Mencari wilayah…</small>}
      {selected && (
        <small className="selected-id">
          {selected.postal_code || "Tanpa kode pos"} · {selected.id}
        </small>
      )}
      {options.length > 0 && (
        <div className="location-options">
          {options.map((item) => (
            <button
              type="button"
              key={item.id}
              onClick={() => {
                onSelect(item);
                setQuery(item.label);
                setOptions([]);
              }}
            >
              <strong>{item.label}</strong>
              <small>
                {item.postal_code || "Tanpa kode pos"} · {item.id}
              </small>
            </button>
          ))}
        </div>
      )}
    </label>
  );
}

function APIKeyManagement({
  items,
  generated,
  loading,
  onGenerate,
  onRevoke,
  onDismissGenerated,
}: {
  items: CustomerAPIKey[];
  generated: GeneratedCustomerAPIKey | null;
  loading: boolean;
  onGenerate: (kind: "public" | "main_service") => Promise<void>;
  onRevoke: (id: string) => Promise<void>;
  onDismissGenerated: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [keyKind, setKeyKind] = useState<"public" | "main_service">(
    "main_service",
  );

  async function generate() {
    setCopied(false);
    try {
      await onGenerate(keyKind);
    } catch {
      // The parent renders the API error.
    }
  }

  async function copySecret() {
    if (!generated) return;
    try {
      await navigator.clipboard.writeText(generated.secret);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="api-key-page">
      <section className="api-key-grid">
        <div className="panel api-key-create">
          <p className="eyebrow">AKSES API</p>
          <h2>Generate API key</h2>
          <p>
            Buat key sesuai pemakainya. Main Service dapat membawa konteks
            merchant ke Emisell Gateway, sedangkan Public API hanya dapat
            mengakses ongkir, lokasi, kurir, dan tracking.
          </p>
          <label className="field">
            <span>Jenis key</span>
            <select
              value={keyKind}
              onChange={(event) =>
                setKeyKind(
                  event.target.value as "public" | "main_service",
                )
              }
            >
              <option value="main_service">Main Service (disarankan)</option>
              <option value="public">Public API</option>
            </select>
            <small>
              {keyKind === "main_service"
                ? "Khusus backend Emisell; dapat memakai X-Emisell-Merchant-ID."
                : "Tidak dapat mengakses /integrations atau membawa konteks merchant."}
            </small>
          </label>
          <div className="api-key-action">
            <button
              className="button button-primary"
              disabled={loading}
              onClick={() => void generate()}
            >
              {loading ? "Memproses…" : "Generate API key"}
            </button>
            <small>Tanpa nama dan tanpa masa berlaku.</small>
          </div>
        </div>

        <aside className="api-key-security">
          <span>Keamanan bawaan</span>
          <strong>Nilai asli tidak disimpan</strong>
          <ul>
            <li>Secret acak 256-bit dengan prefix ek_live_.</li>
            <li>Database hanya menyimpan hash SHA-256.</li>
            <li>Secret lengkap hanya ditampilkan satu kali.</li>
            <li>Key aktif tanpa kedaluwarsa sampai di-revoke.</li>
            <li>Revoke menghentikan akses key tanpa mengubah key lainnya.</li>
          </ul>
        </aside>
      </section>

      {generated && (
        <section className="generated-key" aria-live="polite">
          <div>
            <p className="eyebrow">SIMPAN SEKARANG</p>
            <h2>API key berhasil dibuat</h2>
            <p>
              Salin secret ini. Setelah panel ditutup atau halaman dimuat ulang,
              secret tidak dapat ditampilkan kembali.
            </p>
          </div>
          <code>{generated.secret}</code>
          <div className="generated-key-actions">
            <button className="button button-primary" onClick={copySecret}>
              {copied ? "Tersalin" : "Salin key"}
            </button>
            <button
              className="button button-secondary"
              onClick={onDismissGenerated}
            >
              Sudah disimpan
            </button>
          </div>
        </section>
      )}

      <section className="panel panel-table">
        <div className="panel-heading panel-padding">
          <div>
            <p className="eyebrow">CREDENTIAL AKTIF</p>
            <h2>Daftar customer API key</h2>
          </div>
          <span className="subtle">{items.length} key tercatat</span>
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>API key</th>
                <th>Akses</th>
                <th>Dibuat</th>
                <th>Terakhir dipakai</th>
                <th>Status</th>
                <th>Aksi</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => {
                return (
                  <tr key={item.id}>
                    <td>
                      <strong className="api-key-mask">{item.display_key}</strong>
                    </td>
                    <td>
                      <strong>
                        {item.kind === "main_service" ||
                        item.scopes.includes("gateway:access")
                          ? "Main Service & Gateway"
                          : "Public API"}
                      </strong>
                      <small>{item.scopes.join(" · ")}</small>
                    </td>
                    <td>
                      <strong>{formatDate(item.created_at)}</strong>
                    </td>
                    <td>{formatDate(item.last_used_at)}</td>
                    <td>
                      <span
                        className={`badge ${
                          item.active ? "badge-success" : "badge-warning"
                        }`}
                      >
                        {item.active ? "aktif" : "dicabut"}
                      </span>
                      <small>Aktif sampai di-revoke</small>
                    </td>
                    <td>
                      {item.active && (
                        <button
                          className="table-action table-action-danger"
                          disabled={loading}
                          onClick={() => {
                            if (
                              window.confirm(
                                `Cabut akses API key ${item.display_key}?`,
                              )
                            ) {
                              void onRevoke(item.id);
                            }
                          }}
                        >
                          Revoke
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
              {!items.length && (
                <tr>
                  <td colSpan={6} className="empty-state">
                    Belum ada customer API key. Gunakan tombol generate di
                    atas.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

const PARTNER_DOCUMENTATION_VIEW = {
  id: "partner" as const,
  label: "Partner API",
  classification: "Partner",
  status: "Draft",
  audience: "Vendor kurir, aggregator, dan penyedia last-mile",
  basePath: "https://<partner-host>/partner/v1",
  idFormat: "String canonical milik partner",
  authentication: "Bearer partner + HMAC-SHA256",
  description:
    "Kontrak southbound yang wajib disediakan vendor. API Kurir memanggil connector partner; partner tidak memakai endpoint customer atau admin.",
};

const PARTNER_CONNECTOR_ENDPOINTS: ApiDocumentationEndpoint[] = [
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/health",
    title: "Kesiapan connector",
    description:
      "Memastikan connector partner siap melayani request tanpa membuat transaksi atau memanggil carrier downstream.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    request: `GET https://{{partner_host}}/partner/v1/health
Authorization: Bearer {{partner_runtime_token}}
X-Partner-Key-Id: {{partner_key_id}}
X-Request-Id: req_example`,
    response: `{
  "status": "ok",
  "version": "1.0.0",
  "time": "2026-08-26T07:00:00Z"
}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/capabilities",
    title: "Capability partner",
    description:
      "Mengembalikan kemampuan connector yang telah disertifikasi, granularitas lokasi, dan jenis webhook yang didukung.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    request: `GET https://{{partner_host}}/partner/v1/capabilities
Authorization: Bearer {{partner_runtime_token}}
X-Partner-Key-Id: {{partner_key_id}}`,
    response: `{
  "provider_code": "vendor_x",
  "provider_name": "Vendor X Logistics",
  "contract_version": "1.0",
  "capabilities": {
    "rates": true,
    "shipment_create": true,
    "scheduled_pickup": true,
    "tracking": true,
    "balance": false
  },
  "location_granularities": ["district", "postal_code", "coordinate"]
}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/services",
    title: "Katalog layanan partner",
    description:
      "Menyediakan service code, kelompok layanan, fitur, batas berat, dan aturan pembulatan yang akan dipetakan ke katalog canonical API Kurir.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    request: `GET https://{{partner_host}}/partner/v1/services
Authorization: Bearer {{partner_runtime_token}}
X-Partner-Key-Id: {{partner_key_id}}`,
    response: `{
  "data": [{
    "code": "REG",
    "name": "Regular",
    "service_group": "regular",
    "service_type": "parcel",
    "active": true,
    "minimum_weight_grams": 1000,
    "maximum_weight_grams": 30000,
    "volumetric_divisor": 6000
  }]
}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/rates",
    title: "Quote tarif partner",
    description:
      "Mengambil quote yang mengikat rute, paket, service, fitur, harga, dan masa berlaku untuk proses checkout.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    parameters: [
      "origin dan destination — kode wilayah, kode pos, serta koordinat bila didukung",
      "package.weight_grams — berat aktual dalam gram",
      "service_codes — opsional; layanan yang ingin diperiksa",
    ],
    request: `POST https://{{partner_host}}/partner/v1/rates
Authorization: Bearer {{partner_runtime_token}}
X-Partner-Key-Id: {{partner_key_id}}
X-Signature-Version: v1
X-Signature-Timestamp: {{unix_timestamp}}
X-Signature-Nonce: {{nonce}}
X-Signature: {{signature}}

{
  "request_id": "req_example",
  "delivery_mode": "regular",
  "origin": {"official_code": "3273061001", "postal_code": "40174"},
  "destination": {"official_code": "3212122001", "postal_code": "45281"},
  "package": {"weight_grams": 1200, "item_value": 150000},
  "service_codes": ["REG"]
}`,
    response: `{
  "request_id": "req_example",
  "quotes": [{
    "quote_id": "qt_vendor_01J...",
    "expires_at": "2026-08-26T07:15:00Z",
    "service": {"code": "REG", "name": "Regular", "delivery_mode": "regular"},
    "chargeable_weight_grams": 2000,
    "cost": {"shipping": 18000, "total": 18000, "currency": "IDR"},
    "etd": {"min_days": 2, "max_days": 3, "text": "2-3 hari"}
  }]
}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/shipments",
    title: "Membuat shipment",
    description:
      "Membuat booking berdasarkan quote yang masih aktif. shipment_id dan Idempotency-Key yang sama tidak boleh menghasilkan booking kedua.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    parameters: [
      "Idempotency-Key — wajib dan unik untuk satu operasi",
      "quote_id — quote aktif dari endpoint rates",
      "sender, recipient, package, payment, dan pickup — detail fulfillment",
    ],
    request: `POST https://{{partner_host}}/partner/v1/shipments
Authorization: Bearer {{partner_runtime_token}}
Idempotency-Key: {{idempotency_key}}
X-Signature: {{signature}}

{
  "shipment_id": "shp_01J...",
  "merchant_reference": "ORDER-10001",
  "quote_id": "qt_vendor_01J...",
  "service_code": "REG",
  "fulfillment": "pickup",
  "sender": {"name": "Toko Emisell", "phone": "628123456789"},
  "recipient": {"name": "Budi", "phone": "628987654321"},
  "package": {"weight_grams": 1200, "item_value": 150000}
}`,
    response: `HTTP 201
{
  "shipment_id": "shp_01J...",
  "partner_shipment_id": "VENDOR-10001",
  "awb": null,
  "status": "booking_pending",
  "service_code": "REG",
  "cost": {"total": 18000, "currency": "IDR"}
}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/shipments/{partner_shipment_id}",
    title: "Rekonsiliasi shipment",
    description:
      "Membaca detail, AWB, status canonical, status mentah, biaya aktual, dan event terbaru ketika webhook terlambat atau gagal.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    parameters: ["partner_shipment_id — ID shipment yang diterbitkan partner"],
    request: `GET https://{{partner_host}}/partner/v1/shipments/VENDOR-10001
Authorization: Bearer {{partner_runtime_token}}
X-Partner-Key-Id: {{partner_key_id}}`,
    response: `{
  "shipment_id": "shp_01J...",
  "partner_shipment_id": "VENDOR-10001",
  "awb": "AWB123456789",
  "status": "in_transit",
  "partner_status": "ON_PROCESS"
}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/shipments/{partner_shipment_id}/cancel",
    title: "Membatalkan shipment",
    description:
      "Meminta pembatalan shipment. HTTP 202 berarti permintaan diterima tetapi status belum final.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    parameters: ["Idempotency-Key — wajib", "partner_shipment_id — shipment partner"],
    request: `POST https://{{partner_host}}/partner/v1/shipments/VENDOR-10001/cancel
Authorization: Bearer {{partner_runtime_token}}
Idempotency-Key: {{idempotency_key}}
X-Signature: {{signature}}

{"reason_code":"customer_request","reason":"Pembeli membatalkan pesanan"}`,
    response: `HTTP 202
{"partner_shipment_id":"VENDOR-10001","status":"cancellation_requested"}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/shipments/{partner_shipment_id}/label",
    title: "Mengambil label shipment",
    description:
      "Mengembalikan PDF atau URL HTTPS sementara dengan masa berlaku maksimal 15 menit.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    parameters: ["format — pdf atau format lain yang telah disertifikasi"],
    request: `GET https://{{partner_host}}/partner/v1/shipments/VENDOR-10001/label?format=pdf
Authorization: Bearer {{partner_runtime_token}}`,
    response: `HTTP 200 · application/pdf
atau
{"url":"https://labels.partner.example/temporary/label.pdf","expires_at":"2026-08-26T07:15:00Z"}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/pickups",
    title: "Meminta pickup",
    description:
      "Menjadwalkan pickup untuk satu atau beberapa shipment dan mengembalikan hasil per shipment ketika diproses parsial.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    parameters: ["Idempotency-Key — wajib", "partner_shipment_ids — shipment yang akan diambil"],
    request: `POST https://{{partner_host}}/partner/v1/pickups
Authorization: Bearer {{partner_runtime_token}}
Idempotency-Key: {{idempotency_key}}
X-Signature: {{signature}}

{
  "partner_shipment_ids": ["VENDOR-10001"],
  "mode": "scheduled",
  "scheduled_at": "2026-08-27T09:00:00Z"
}`,
    response: `{
  "pickup_id": "pku_01J...",
  "partner_pickup_id": "PICKUP-10001",
  "status": "accepted",
  "items": [{"partner_shipment_id":"VENDOR-10001","status":"accepted"}]
}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/pickup-windows/search",
    title: "Mencari jadwal pickup",
    description:
      "Mengambil slot pickup regular yang tersedia. Instant/on-demand tidak membutuhkan window terjadwal.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    request: `POST https://{{partner_host}}/partner/v1/pickup-windows/search
Authorization: Bearer {{partner_runtime_token}}
X-Signature: {{signature}}

{"official_code":"3273061001","postal_code":"40174","date":"2026-08-27"}`,
    response: `{"data":[{"start_at":"2026-08-27T09:00:00Z","end_at":"2026-08-27T12:00:00Z"}]}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/labels/batch",
    title: "Membuat label batch",
    description:
      "Membuat label beberapa shipment dalam satu PDF atau URL sementara menggunakan layout yang disertifikasi.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    parameters: ["Idempotency-Key — wajib", "layout — contoh a4_4 atau thermal_100x150"],
    request: `POST https://{{partner_host}}/partner/v1/labels/batch
Authorization: Bearer {{partner_runtime_token}}
Idempotency-Key: {{idempotency_key}}
X-Signature: {{signature}}

{"partner_shipment_ids":["VENDOR-10001"],"format":"pdf","layout":"a4_4"}`,
    response: `{"url":"https://labels.partner.example/temporary/batch.pdf","expires_at":"2026-08-26T07:15:00Z"}`,
  },
  {
    scope: "partner",
    method: "POST",
    path: "/partner/v1/tracking/waybills",
    title: "Tracking AWB eksternal",
    description:
      "Melacak AWB yang tidak harus dibuat melalui connector. Wajib hanya jika external_tracking telah disertifikasi.",
    authentication: "Bearer runtime token + HMAC-SHA256",
    request: `POST https://{{partner_host}}/partner/v1/tracking/waybills
Authorization: Bearer {{partner_runtime_token}}
X-Signature: {{signature}}

{"courier_code":"jne","awb":"AWB123456789","last_phone_digits":null}`,
    response: `{
  "awb": "AWB123456789",
  "status": "in_transit",
  "history": [{"status":"picked_up","occurred_at":"2026-08-25T10:00:00Z"}]
}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/account/balance",
    title: "Saldo account partner",
    description:
      "Membaca saldo account jika capability balance telah disertifikasi. Saldo partner dipisahkan dari ledger Emisell.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    request: `GET https://{{partner_host}}/partner/v1/account/balance
Authorization: Bearer {{partner_runtime_token}}`,
    response: `{"available":1250000,"pending":50000,"currency":"IDR","updated_at":"2026-08-26T07:00:00Z"}`,
  },
  {
    scope: "partner",
    method: "GET",
    path: "/partner/v1/payments/{payment_id}",
    title: "Rekonsiliasi pembayaran",
    description:
      "Membaca status pembayaran provider apabila capability payment inquiry telah disertifikasi.",
    authentication: "Bearer runtime token + X-Partner-Key-Id",
    parameters: ["payment_id — ID pembayaran dari provider"],
    request: `GET https://{{partner_host}}/partner/v1/payments/PAY-10001
Authorization: Bearer {{partner_runtime_token}}`,
    response: `{"payment_id":"PAY-10001","status":"settled","amount":18000,"currency":"IDR"}`,
  },
];

const DOCUMENTATION_VIEWS = [
  ...API_DOCUMENTATION_CONTRACTS,
  PARTNER_DOCUMENTATION_VIEW,
];

function ApiDocumentation({
  view,
  onViewChange,
}: {
  view: ApiDocumentationView;
  onViewChange: (view: ApiDocumentationView) => void;
}) {
  const activeView =
    DOCUMENTATION_VIEWS.find((item) => item.id === view) ??
    API_DOCUMENTATION_CONTRACTS[0];
  const endpoints =
    view === "partner"
      ? []
      : API_DOCUMENTATION.filter(
          (endpoint) => getApiDocumentationContract(endpoint) === view,
        );
  const isPartner = view === "partner";
  const isGateway = view === "gateway";
  const hasCheckoutWeightSop =
    view === "rajaongkir-v2" || view === "canonical" || view === "gateway";

  return (
    <div className="documentation-page">
      <section className="docs-hero">
        <div>
          <p className="eyebrow">DEVELOPER PORTAL</p>
          <h2>Kontrak API yang tidak tercampur</h2>
          <p>
            Pilih kontrak berdasarkan pemanggil dan base path. Jenis header
            hanya untuk autentikasi; bentuk ID dan respons selalu ditentukan
            oleh path endpoint.
          </p>
        </div>
        <div className="docs-downloads">
          {isPartner ? (
            <a
              className="button button-primary"
              href="/openapi/api-kurir-partner-v1.yaml"
              download
            >
              Unduh OpenAPI Partner
            </a>
          ) : (
            <>
              <a
                className="button button-primary"
                href="/api-kurir.postman_collection.json"
                download
              >
                Unduh Postman
              </a>
              <a
                className="button button-secondary"
                href="/openapi/api-kurir-public-v1.yaml"
                download
              >
                Unduh OpenAPI
              </a>
            </>
          )}
        </div>
      </section>

      <section className="docs-contract-selector" aria-label="Kontrak API">
        {DOCUMENTATION_VIEWS.map((item) => {
          const endpointCount =
            item.id === "partner"
              ? null
              : API_DOCUMENTATION.filter(
                  (endpoint) =>
                    getApiDocumentationContract(endpoint) === item.id,
                ).length;
          return (
            <button
              key={item.id}
              className={view === item.id ? "contract-active" : ""}
              onClick={() => onViewChange(item.id)}
            >
              <span>{item.classification}</span>
              <strong>{item.label}</strong>
              <small>
                {endpointCount === null
                  ? "Kontrak vendor"
                  : `${endpointCount} endpoint`}
              </small>
            </button>
          );
        })}
      </section>

      <section className="docs-quickstart">
        <article>
          <span>Base path</span>
          <strong>{activeView.basePath}</strong>
          <small>{activeView.audience}</small>
        </article>
        <article>
          <span>Klasifikasi</span>
          <strong>
            {activeView.classification} · {activeView.status}
          </strong>
          <small>{activeView.description}</small>
        </article>
        <article>
          <span>Format ID</span>
          <strong>{activeView.idFormat}</strong>
          <small>ID dari kontrak lain tidak boleh dicampur.</small>
        </article>
        <article>
          <span>Autentikasi</span>
          <strong>{activeView.authentication}</strong>
          <small>Secret asli tidak boleh ditulis pada dokumentasi atau log.</small>
        </article>
      </section>

      {isPartner ? (
        <>
          <PartnerDocumentation />
          <PartnerConnectorEndpointCatalog />
        </>
      ) : (
        <>
          {isGateway && <GatewayTenantDocumentation />}
          <section className="panel docs-guide">
            <div>
              <p className="eyebrow">ALUR INTEGRASI</p>
              <h2>Gunakan satu kontrak sampai selesai</h2>
            </div>
            <ol>
              <li>Import Collection dan isi hanya variable secret lokal.</li>
              <li>
                Gunakan base path <code>{activeView.basePath}</code> secara
                konsisten.
              </li>
              <li>Ambil ID lokasi dari endpoint dalam kontrak yang sama.</li>
              <li>Jangan meneruskan ID provider sebagai ID canonical internal.</li>
            </ol>
          </section>

          {hasCheckoutWeightSop && <ShippingWeightSopDocumentation />}

          <section className="docs-section-heading">
            <div>
              <p className="eyebrow">ENDPOINT TERSEDIA</p>
              <h2>
                {endpoints.length} endpoint · {activeView.label}
              </h2>
            </div>
            <span
              className={`docs-status docs-status-${activeView.classification.toLowerCase()}`}
            >
              {activeView.classification} · {activeView.status}
            </span>
          </section>

          <div className="endpoint-list">
            {endpoints.map((endpoint, index) => (
              <EndpointDocumentation
                endpoint={endpoint}
                classification={activeView.classification}
                key={`${endpoint.method}-${endpoint.path}`}
                open={index === 0}
              />
            ))}
          </div>
        </>
      )}

      <section className="docs-footnote">
        <strong>Aturan global dan keamanan</strong>
        <p>
          Semua berat memakai gram, uang memakai integer rupiah, timestamp
          memakai ISO 8601 UTC. Gunakan placeholder seperti
          <code> {"{{api_key}}"}</code>; jangan pernah menaruh API key aktif,
          ciphertext, atau credential provider dalam contoh.
        </p>
      </section>
    </div>
  );
}

function ShippingWeightSopDocumentation() {
  return (
    <section className="panel shipping-weight-sop">
      <div className="shipping-weight-sop-heading">
        <div>
          <p className="eyebrow">SOP CHECKOUT · ELIGIBILITY BERAT</p>
          <h2>Tampilkan hanya layanan yang sanggup menerima paket</h2>
        </div>
        <p>
          Emisell mengirim berat final dalam gram. API Kurir menyaring hasil
          provider berdasarkan minimum diterima, minimum tagihan, maksimum,
          dan service yang diaktifkan seller.
        </p>
      </div>

      <div className="shipping-weight-sop-steps">
        <article>
          <span>01</span>
          <strong>Terima berat final</strong>
          <p>Dimensi dan berat volumetrik sudah diselesaikan oleh Emisell.</p>
        </article>
        <article>
          <span>02</span>
          <strong>Ambil quote</strong>
          <p>Harga dibaca dari snapshot atau provider, bukan diketik manual.</p>
        </article>
        <article>
          <span>03</span>
          <strong>Filter service</strong>
          <p>Service di luar batas min–max tidak dikirim ke checkout.</p>
        </article>
        <article>
          <span>04</span>
          <strong>Terapkan pilihan seller</strong>
          <p>Hanya service eligible yang memang diaktifkan seller.</p>
        </article>
      </div>

      <div className="table-scroll shipping-weight-sop-table">
        <table>
          <thead>
            <tr>
              <th>Contoh</th>
              <th>Minimum diterima</th>
              <th>Minimum tagihan</th>
              <th>Maksimum</th>
              <th>Keputusan</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Paket 1 kg · cargo</td>
              <td>3 kg</td>
              <td>Mengikuti service</td>
              <td>Mengikuti service</td>
              <td><strong>Sembunyikan cargo</strong></td>
            </tr>
            <tr>
              <td>Paket 3 kg · Anteraja Cargo</td>
              <td>3 kg</td>
              <td>5 kg</td>
              <td>100 kg</td>
              <td><strong>Tampilkan; quote minimal 5 kg</strong></td>
            </tr>
            <tr>
              <td>Paket 80 kg · parcel</td>
              <td>1 gram</td>
              <td>Mengikuti quote</td>
              <td>50 kg default</td>
              <td><strong>Sembunyikan regular/economy/next day</strong></td>
            </tr>
          </tbody>
        </table>
      </div>

      <details className="shipping-weight-sop-matrix">
        <summary>Lihat matriks referensi layanan kargo</summary>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Layanan canonical</th>
                <th>Minimum diterima</th>
                <th>Minimum tagihan</th>
                <th>Maksimum</th>
                <th>Status sumber</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Anteraja BIG</td><td>3 kg</td><td>5 kg</td><td>100 kg</td>
                <td>Referensi kanal · konfirmasi kontrak</td>
              </tr>
              <tr>
                <td>JNE JTR</td><td>3 kg</td><td>5 kg</td><td>600 kg</td>
                <td>Referensi kanal · konfirmasi kontrak</td>
              </tr>
              <tr>
                <td>SiCepat GOKIL</td><td>3 kg</td><td>5 kg</td><td>50 kg</td>
                <td>Referensi kanal · konfirmasi kontrak</td>
              </tr>
              <tr>
                <td>Sentral DARAT/LAUT/UDARA</td><td>5 kg</td>
                <td>Mengikuti quote</td><td>Mengikuti quote</td>
                <td>Referensi kanal · konfirmasi kontrak</td>
              </tr>
              <tr>
                <td>Wahana KARGO</td><td>10 kg</td><td>10 kg</td><td>50 kg</td>
                <td>Publik resmi</td>
              </tr>
              <tr>
                <td>TIKI TRC</td><td>10 kg</td><td>10 kg</td>
                <td>Mengikuti quote</td><td>Publik resmi</td>
              </tr>
              <tr>
                <td>RPX HWP</td><td>20 kg</td><td>20 kg</td><td>50 kg</td>
                <td>Publik resmi</td>
              </tr>
              <tr>
                <td>SAPX CARGO</td><td>5 kg</td><td>Mengikuti quote</td>
                <td>Mengikuti quote</td><td>Perlu konfirmasi kontrak</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          J&amp;T Cargo Shopee tidak disamakan dengan J&amp;T HBO RajaOngkir.
          Service baru hanya masuk master setelah code provider terverifikasi.
        </p>
      </details>

      <div className="shipping-weight-sop-notes">
        <p>
          <strong>Minimum diterima</strong> menentukan tampil atau tidak.
          <strong> Minimum tagihan</strong> hanya menentukan dasar biaya dan
          tidak boleh dipakai untuk menyembunyikan layanan.
        </p>
        <p>
          Aturan Shopee adalah referensi kebijakan kanal, bukan kontrak
          universal provider. Exact quote provider tetap menjadi sumber harga;
          aturan kontrak seller harus menggantikan referensi bila tersedia.
        </p>
      </div>
    </section>
  );
}

function GatewayTenantDocumentation() {
  return (
    <section className="panel docs-guide">
      <div>
        <p className="eyebrow">MERCHANT CONTEXT</p>
        <h2>Satu merchant, satu konteks backend yang sederhana</h2>
        <p>
          Backend Emisell menentukan merchant dari database lalu mengirim
          dedicated service API key dan header merchant pada setiap request.
          Browser tidak memanggil endpoint gateway secara langsung.
        </p>
      </div>
      <pre>
        <code>{`GET /api/v1/integrations/shipping-services
key: <dedicated-emisell-service-key>
X-Emisell-Merchant-ID: merchant_123`}</code>
      </pre>
      <ol>
        <li>
          Semua domain milik merchant yang sama memakai
          <code>X-Emisell-Merchant-ID</code> yang sama.
        </li>
        <li>
          Domain dan gudang diselesaikan oleh Emisell sebelum request; keduanya
          bukan bagian dari kontrak autentikasi API Kurir.
        </li>
        <li>
          Emisell tidak menyimpan atau mengirim credential ID. API Kurir
          memilih key aktif berdasarkan merchant dan provider code.
        </li>
        <li>
          Satu merchant dapat memasang beberapa provider, tetapi hanya satu
          provider shipping yang efektif aktif.
        </li>
        <li>
          Request tenant tidak pernah fallback ke credential platform atau
          seller lain.
        </li>
      </ol>
    </section>
  );
}

function PartnerDocumentation() {
  return (
    <div className="partner-docs">
      <section className="panel partner-docs-intro">
        <div>
          <p className="eyebrow">SOUTHBOUND CONTRACT</p>
          <h2>Vendor menyediakan connector, API Kurir menjadi gateway</h2>
          <p>
            Partner mengimplementasikan kontrak ini pada infrastrukturnya.
            Emisell tidak memanggil API native partner dan API Kurir tidak
            menyimpan adapter khusus untuk setiap vendor baru.
          </p>
        </div>
        <div className="partner-flow" aria-label="Alur Partner API">
          <span>Emisell</span>
          <i>→</i>
          <strong>API Kurir</strong>
          <i>→</i>
          <span>Partner Connector</span>
        </div>
      </section>

      <section className="partner-docs-grid">
        <article>
          <span>01 · Discovery</span>
          <h3>Capability & layanan</h3>
          <p>
            Partner menyediakan health, capability, service catalog, coverage,
            dan informasi fitur seperti regular, cargo, instant, COD, atau
            external tracking.
          </p>
        </article>
        <article>
          <span>02 · Transaksi</span>
          <h3>Rate, shipment & pickup</h3>
          <p>
            Quote dikunci ke provider account asal. Booking, pickup, cancel,
            label, dan rekonsiliasi tidak boleh berpindah provider setelah
            shipment terbentuk.
          </p>
        </article>
        <article>
          <span>03 · Status</span>
          <h3>Tracking & webhook</h3>
          <p>
            Event AWB dan perjalanan dikirim ke callback API Kurir. Endpoint
            detail tetap menjadi sumber rekonsiliasi saat event terlambat atau
            gagal.
          </p>
        </article>
        <article>
          <span>04 · Publish</span>
          <h3>Pengujian & sertifikasi</h3>
          <p>
            Extension baru dapat dipublikasikan setelah contract test,
            idempotency, retry, signature, isolasi tenant, dan skenario kegagalan
            tervalidasi.
          </p>
        </article>
      </section>

      <section className="panel partner-security">
        <div>
          <p className="eyebrow">MINIMUM PRODUCTION</p>
          <h2>Kontrol keamanan wajib</h2>
        </div>
        <ul>
          <li>TLS 1.2+, Bearer credential terpisah, dan rotasi tanpa downtime.</li>
          <li>HMAC-SHA256 untuk request mutasi dan seluruh webhook.</li>
          <li>Timestamp, nonce, replay protection, dan idempotency key.</li>
          <li>Secret terenkripsi serta tidak pernah muncul pada log atau respons.</li>
          <li>Rate limit per partner, audit trail, dan isolasi data seller.</li>
          <li>Contract test wajib lulus sebelum status production.</li>
        </ul>
      </section>

      <section className="docs-footnote docs-draft-note">
        <strong>Status: Draft untuk implementasi dan sertifikasi</strong>
        <p>
          Spesifikasi Partner API adalah kontrak target. Endpoint tidak boleh
          dianggap aktif di production sebelum vendor tercatat lulus validasi
          pada Partner Portal.
        </p>
      </section>
    </div>
  );
}

function PartnerConnectorEndpointCatalog() {
  return (
    <section className="partner-connector-endpoint-catalog">
      <div className="docs-section-heading">
        <div>
          <p className="eyebrow">ENDPOINT CONNECTOR</p>
          <h2>{PARTNER_CONNECTOR_ENDPOINTS.length} endpoint · Partner API</h2>
          <p>
            Buka endpoint untuk melihat autentikasi, parameter, contoh request,
            dan response. Partner menyediakan endpoint ini pada host miliknya.
          </p>
        </div>
        <span className="docs-status docs-status-partner">Partner · Draft</span>
      </div>
      <div className="endpoint-list">
        {PARTNER_CONNECTOR_ENDPOINTS.map((endpoint, index) => (
          <EndpointDocumentation
            endpoint={endpoint}
            classification="Partner"
            key={`${endpoint.method}-${endpoint.path}`}
            open={index === 0}
          />
        ))}
      </div>
    </section>
  );
}

function EndpointDocumentation({
  endpoint,
  classification,
  open,
}: {
  endpoint: ApiDocumentationEndpoint;
  classification: string;
  open: boolean;
}) {
  return (
    <details className="endpoint-card" open={open}>
      <summary>
        <span className={`method-badge method-${endpoint.method.toLowerCase()}`}>
          {endpoint.method}
        </span>
        <span className="endpoint-contract">{classification}</span>
        <code>{endpoint.path}</code>
        <span className="endpoint-title">{endpoint.title}</span>
        <span className="endpoint-chevron">⌄</span>
      </summary>
      <div className="endpoint-content">
        <p>{endpoint.description}</p>
        <div className="endpoint-auth">
          <span>Autentikasi</span>
          <strong>{endpoint.authentication}</strong>
        </div>
        {endpoint.parameters && (
          <div className="endpoint-parameters">
            <h3>Parameter</h3>
            <ul>
              {endpoint.parameters.map((parameter) => (
                <li key={parameter}>{parameter}</li>
              ))}
            </ul>
          </div>
        )}
        <div
          className={`code-grid ${endpoint.request ? "" : "code-grid-single"}`}
        >
          {endpoint.request && (
            <CodeExample title="Contoh request Postman" content={endpoint.request} />
          )}
          <CodeExample title="Contoh response" content={endpoint.response} />
        </div>
      </div>
    </details>
  );
}

function CodeExample({ title, content }: { title: string; content: string }) {
  const [copied, setCopied] = useState(false);

  async function copyExample() {
    await navigator.clipboard.writeText(content);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1_500);
  }

  return (
    <div className="code-example">
      <div className="code-example-heading">
        <span>{title}</span>
        <button type="button" onClick={() => void copyExample()}>
          {copied ? "Tersalin" : "Salin"}
        </button>
      </div>
      <pre>
        <code>{content}</code>
      </pre>
    </div>
  );
}

function Metric({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint: string;
}) {
  return (
    <article className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{hint}</small>
    </article>
  );
}

function RateSnapshotTable({ items }: { items: RateSnapshot[] }) {
  return (
    <div className="table-scroll">
      <table>
        <thead>
          <tr>
            <th>Rute</th>
            <th>Layanan</th>
            <th>Berat dan harga</th>
            <th>Estimasi</th>
            <th>Sumber</th>
          </tr>
        </thead>
        <tbody>
          {items.map((snapshot) => (
            <tr key={snapshot.id}>
              <td>
                <strong>{snapshot.origin_label}</strong>
                <small>ke {snapshot.destination_label}</small>
              </td>
              <td>
                <strong>
                  {snapshot.courier_code.toUpperCase()} ·{" "}
                  {snapshot.service_code}
                </strong>
                <small>
                  {snapshot.service_name}
                  {snapshot.description ? ` · ${snapshot.description}` : ""}
                </small>
              </td>
              <td>
                <strong>{formatMoney(snapshot.returned_cost)}</strong>
                <small>
                  Permintaan {formatNumber(snapshot.requested_weight_grams)} gram
                </small>
              </td>
              <td>
                <strong>
                  {formatETD(snapshot.etd_min_days, snapshot.etd_max_days)}
                </strong>
                <small>Diambil {formatDate(snapshot.fetched_at)}</small>
              </td>
              <td>
                <span
                  className={`badge ${
                    snapshot.fresh ? "badge-success" : "badge-warning"
                  }`}
                >
                  {snapshot.fresh ? "aktif" : "kedaluwarsa"}
                </span>
                <small>
                  {snapshot.provider_code} · {snapshot.verification_status}
                </small>
                <small>
                  {snapshot.tenant_id
                    ? `Merchant ${snapshot.tenant_id}`
                    : "Platform"}
                </small>
              </td>
            </tr>
          ))}
          {!items.length && (
            <tr>
              <td colSpan={5} className="empty-state">
                Belum ada snapshot tarif. Data akan tersimpan otomatis ketika
                rute pertama kali dicek.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
