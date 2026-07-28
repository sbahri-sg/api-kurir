import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  AdminApi,
  API_URL,
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
} from "./api";
import {
  API_DOCUMENTATION,
  type ApiDocumentationEndpoint,
  type ApiDocumentationScope,
} from "./apiDocumentation";

type Tab =
  | "overview"
  | "check-rate"
  | "tracking"
  | "couriers"
  | "rates"
  | "mappings"
  | "quota"
  | "api-keys"
  | "documentation";

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
  const [providerKeySecret, setProviderKeySecret] = useState("");
  const [providerKeyLoading, setProviderKeyLoading] = useState(false);
  const [apiKeys, setAPIKeys] = useState<CustomerAPIKey[]>([]);
  const [generatedAPIKey, setGeneratedAPIKey] =
    useState<GeneratedCustomerAPIKey | null>(null);
  const [keyActionLoading, setKeyActionLoading] = useState(false);
  const [rateSearch, setRateSearch] = useState("");
  const [mappingSearch, setMappingSearch] = useState("");
  const [documentationScope, setDocumentationScope] =
    useState<ApiDocumentationScope>("customer");

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
        ] =
          await Promise.all([
            api.overview(signal),
            api.couriers(signal),
            api.rateSnapshots("", signal),
            api.mappings("", "", signal),
            api.quotas(signal),
            api.apiKeys(signal),
            api.providerCredentials(signal),
          ]);
        setOverview(overviewData);
        setCouriers(courierData);
        setSnapshots(snapshotData);
        setMappings(mappingsData);
        setQuotas(quotaData);
        setAPIKeys(apiKeyData);
        setProviderCredentials(providerCredentialData);
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
        "rajaongkir",
        providerKeySecret.trim(),
      );
      const [credentialData, quotaData] = await Promise.all([
        api.providerCredentials(),
        api.quotas(),
      ]);
      setProviderCredentials(credentialData);
      setQuotas(quotaData);
      setProviderKeySecret("");
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
    overview: "Ringkasan",
    "check-rate": "Cek Ongkir",
    tracking: "Cek Resi",
    couriers: "Daftar Ekspedisi",
    rates: "Tarif otomatis",
    mappings: "Mapping otomatis",
    quota: "Kuota provider",
    "api-keys": "API Keys",
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
        <nav>
          {(
            [
              ["overview", "Ringkasan"],
              ["check-rate", "Cek Ongkir"],
              ["tracking", "Cek Resi"],
              ["couriers", "Daftar Ekspedisi"],
              ["rates", "Tarif otomatis"],
              ["mappings", "Mapping otomatis"],
              ["quota", "Kuota provider"],
              ["api-keys", "API Keys"],
              ["documentation", "Dokumentasi API"],
            ] as [Tab, string][]
          ).map(([value, label]) => (
            <button
              className={tab === value ? "nav-active" : ""}
              key={value}
              onClick={() => setTab(value)}
            >
              <span className="nav-dot" />
              {label}
            </button>
          ))}
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
                label="Mapping otomatis"
                value={formatNumber(overview.location_mappings)}
                hint="Terbentuk saat rute pertama dicari"
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
                <h2>Key aktif dari database</h2>
                <p>
                  Tambahkan key RajaOngkir sekali. Sistem memvalidasi,
                  mengenkripsi, lalu otomatis memilih key dengan kuota
                  terbanyak saat tarif atau tracking membutuhkan provider.
                </p>
              </div>
              <button
                className="button button-primary"
                onClick={() => setShowProviderKeyModal(true)}
              >
                Tambah key
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
                        <td colSpan={6} className="empty-state">
                          Belum ada key database. Tambahkan key RajaOngkir untuk
                          mengaktifkan sinkronisasi otomatis.
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
                      <th>Terpakai</th>
                      <th>Sisa</th>
                      <th>Penggunaan</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {quotas.map((quota) => (
                      <tr
                        key={`${quota.provider_code}-${quota.credential_alias}-${quota.quota_date}`}
                      >
                        <td>{quota.quota_date}</td>
                        <td>
                          <strong>{quota.provider_code}</strong>
                          <small>{quota.credential_alias}</small>
                        </td>
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
                        <td colSpan={6} className="empty-state">
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
                      <p className="eyebrow">TAMBAH CREDENTIAL</p>
                      <h2>Key RajaOngkir</h2>
                    </div>
                    <button
                      onClick={() => {
                        setProviderKeySecret("");
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
                      <input value="RajaOngkir" disabled />
                    </label>
                    <label>
                      API key provider
                      <input
                        type="password"
                        value={providerKeySecret}
                        onChange={(event) =>
                          setProviderKeySecret(event.target.value)
                        }
                        placeholder="Tempel API key RajaOngkir"
                        autoComplete="new-password"
                        required
                      />
                    </label>
                    <div className="provider-key-note">
                      Key akan diuji langsung ke RajaOngkir. Jika valid, secret
                      dienkripsi AES-256-GCM dan langsung tersedia untuk sync
                      tarif secara lazy tanpa restart API.
                    </div>
                    <div className="form-actions">
                      <button
                        type="button"
                        className="button button-secondary"
                        onClick={() => {
                          setProviderKeySecret("");
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

        {tab === "documentation" && (
          <ApiDocumentation
            scope={documentationScope}
            onScopeChange={setDocumentationScope}
          />
        )}
      </section>
    </main>
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
        ...courier.services.flatMap((service) => [
          service.code,
          service.name,
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
            <span>Ekspedisi domestik</span>
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
                    {courier.provider_code}
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
                  <span className="capability-on">Cek resi</span>
                ) : (
                  <span className="capability-off">Resi belum tersedia</span>
                )}
                {courier.supports_international_cost && (
                  <span className="capability-special">Internasional</span>
                )}
              </div>

              <div className="courier-services">
                <div className="courier-section-title">
                  <span>Layanan lokal terdaftar</span>
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
                    Service tersedia akan mengikuti respons provider untuk
                    rute, berat, dan coverage yang dipilih.
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
              Berat paket
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

const TRACKING_COURIERS = [
  { code: "jne", name: "JNE" },
  { code: "sap", name: "SAP Express" },
  { code: "ninja", name: "Ninja Xpress" },
  { code: "jnt", name: "J&T Express" },
  { code: "tiki", name: "TIKI" },
  { code: "wahana", name: "Wahana" },
  { code: "pos", name: "POS Indonesia" },
  { code: "lion", name: "Lion Parcel" },
];

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

  const courierNames = useMemo(
    () => new Map(couriers.map((item) => [item.code, item.name])),
    [couriers],
  );

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
              {TRACKING_COURIERS.map((item) => (
                <option key={item.code} value={item.code}>
                  {courierNames.get(item.code) || item.name}
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

function ApiDocumentation({
  scope,
  onScopeChange,
}: {
  scope: ApiDocumentationScope;
  onScopeChange: (scope: ApiDocumentationScope) => void;
}) {
  const endpoints = API_DOCUMENTATION.filter(
    (endpoint) => endpoint.scope === scope,
  );

  return (
    <div className="documentation-page">
      <section className="docs-hero">
        <div>
          <p className="eyebrow">DEVELOPER PORTAL</p>
          <h2>Integrasi API Kurir</h2>
          <p>
            Kontrak customer mengikuti pola RajaOngkir: cari ID lokasi lokal,
            kirim berat dalam gram, lalu pilih kurir. Provider dapat berubah
            tanpa mengubah integrasi Emisell.
          </p>
        </div>
        <div className="docs-downloads">
          <a
            className="button button-primary"
            href="/api-kurir.postman_collection.json"
            download
          >
            Unduh Postman Collection
          </a>
          <a
            className="button button-secondary"
            href="/api-kurir.local.postman_environment.json"
            download
          >
            Unduh Environment
          </a>
        </div>
      </section>

      <section className="docs-quickstart">
        <article>
          <span>Base URL</span>
          <strong>{API_URL}</strong>
          <small>Gunakan variabel Postman: {"{{base_url}}"}</small>
        </article>
        <article>
          <span>Autentikasi customer</span>
          <strong>Bearer {"{{api_key}}"}</strong>
          <small>Untuk endpoint /v1 customer</small>
        </article>
        <article>
          <span>Autentikasi admin</span>
          <strong>Bearer {"{{admin_api_key}}"}</strong>
          <small>Hanya untuk endpoint /v1/admin</small>
        </article>
      </section>

      <section className="panel docs-guide">
        <div>
          <p className="eyebrow">MULAI DARI POSTMAN</p>
          <h2>Empat langkah integrasi</h2>
        </div>
        <ol>
          <li>Import Collection dan Environment dari tombol unduh.</li>
          <li>
            Isi <code>api_key</code> atau <code>admin_api_key</code> di Current
            Value Postman.
          </li>
          <li>
            Jalankan pencarian lokasi untuk memperoleh <code>origin_id</code>{" "}
            dan <code>destination_id</code>.
          </li>
          <li>
            Jalankan cek ongkir; snapshot provider akan digunakan ulang selama
            masih aktif.
          </li>
        </ol>
      </section>

      <section className="docs-section-heading">
        <div>
          <p className="eyebrow">ENDPOINT TERSEDIA</p>
          <h2>{endpoints.length} API {scope === "customer" ? "customer" : "admin"}</h2>
        </div>
        <div className="docs-scope-switch" aria-label="Kategori dokumentasi">
          <button
            className={scope === "customer" ? "scope-active" : ""}
            onClick={() => onScopeChange("customer")}
          >
            Customer & health
          </button>
          <button
            className={scope === "admin" ? "scope-active" : ""}
            onClick={() => onScopeChange("admin")}
          >
            Admin & security
          </button>
        </div>
      </section>

      <div className="endpoint-list">
        {endpoints.map((endpoint, index) => (
          <EndpointDocumentation
            endpoint={endpoint}
            key={`${endpoint.method}-${endpoint.path}`}
            open={index === 0}
          />
        ))}
      </div>

      <section className="docs-footnote">
        <strong>Aturan satuan</strong>
        <p>
          Semua berat memakai gram, uang memakai integer rupiah, timestamp
          memakai ISO 8601 UTC, dan ID provider tidak boleh dijadikan primary ID
          oleh service utama.
        </p>
      </section>
    </div>
  );
}

function EndpointDocumentation({
  endpoint,
  open,
}: {
  endpoint: ApiDocumentationEndpoint;
  open: boolean;
}) {
  return (
    <details className="endpoint-card" open={open}>
      <summary>
        <span className={`method-badge method-${endpoint.method.toLowerCase()}`}>
          {endpoint.method}
        </span>
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
  return (
    <div className="code-example">
      <div className="code-example-heading">
        <span>{title}</span>
        <small>JSON</small>
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
