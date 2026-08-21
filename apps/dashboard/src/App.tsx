import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  AdminApi,
  type Courier,
  type CustomerAPIKey,
  type GeneratedCustomerAPIKey,
  type LocationOption,
  type LocationMapping,
  type Overview,
  type ProviderCredential,
  type ProviderQuota,
  type RateResult,
  type RateSnapshot,
  type TrackingShipment,
	type TrackingOperation,
	type TrackingOperationPage,
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
  | "couriers"
  | "rates"
  | "mappings"
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
      return "Data resi tidak dapat dibaca oleh worker. Kunci enkripsi perlu diperiksa oleh admin.";
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
            api.apiKeys(signal),
            api.providerCredentials(signal),
            api.webhookSettings(signal),
          ]);
        setOverview(overviewData);
        setCouriers(courierData);
        setSnapshots(snapshotData);
        setMappings(mappingsData);
        setQuotas(quotaData);
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

  async function generateAPIKey() {
    setKeyActionLoading(true);
    setError("");
    try {
      const generated = await api.generateAPIKey();
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
    couriers: "Ekspedisi & Service",
    rates: "Snapshot Tarif",
    mappings: "Mapping Lokasi Provider",
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
                          ongkir/tracking utama atau Biteship untuk fallback
                          tracking.
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
                          Biteship · tracking fallback
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
                      terenkripsi. Biteship hanya dipakai untuk fallback
                      tracking (misalnya SiCepat), bukan cek ongkir atau
                      sinkronisasi katalog.
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
                  {courier.code.slice(0, 3).toUpperCase()}
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
  onGenerate: () => Promise<void>;
  onRevoke: (id: string) => Promise<void>;
  onDismissGenerated: () => void;
}) {
  const [copied, setCopied] = useState(false);

  async function generate() {
    setCopied(false);
    try {
      await onGenerate();
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
          <p className="eyebrow">AKSES CUSTOMER</p>
          <h2>Generate API key</h2>
          <p>
            Buat key untuk main service atau customer. Key ini hanya dapat
            mengakses API ongkir, lokasi, kurir, dan tracking—bukan endpoint
            admin. Key tetap aktif sampai Anda melakukan revoke.
          </p>
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
                      <strong>Ongkir & tracking</strong>
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
        <PartnerDocumentation />
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
          <h3>Sandbox & sertifikasi</h3>
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
          <li>Contract test sandbox wajib lulus sebelum status production.</li>
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
