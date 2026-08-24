const configuredAPIURL = import.meta.env.VITE_API_URL?.trim();

// An empty base URL keeps every request relative to the dashboard origin.
// In production Nginx owns /v1/* and forwards it to the API container, so the
// browser never needs to know the backend hostname, port, or Docker service.
export const API_URL =
  configuredAPIURL && configuredAPIURL !== "/"
    ? configuredAPIURL.replace(/\/+$/, "")
    : "";

type ApiEnvelope<T> = {
  data: T;
  meta?: Record<string, unknown>;
};

type ApiErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

export type Overview = {
  active_rate_cards: number;
  official_locations: number;
  location_mappings: number;
  fresh_quote_snapshots: number;
  quota_used_today: number;
  quota_limit_today: number;
  tracking_pending_jobs: number;
  tracking_shipments: number;
};

export type LocationOption = {
  id: string;
  label: string;
  province_id: string;
  city_id: string;
  district_id: string;
  subdistrict_id: string;
  province_name: string;
  city_name: string;
  district_name: string;
  subdistrict_name: string;
  zip_code: string;
  province: string;
  city: string;
  district: string;
  subdistrict: string;
  postal_code: string;
  postal_codes: string[];
};

export type CourierService = {
  code: string;
  name: string;
  group: string;
  service_type: string;
  calculation_mode: string;
};

export type Courier = {
  code: string;
  name: string;
  provider_code: string;
  rate_provider_code: string;
  tracking_provider_code: string;
  supports_domestic_cost: boolean;
  supports_international_cost: boolean;
  supports_tracking: boolean;
  catalog_source: string;
  tracking_catalog_source: string;
  catalog_verified_at: string;
  services: CourierService[];
};

export type RateResult = {
  courier: {
    code: string;
    name: string;
  };
  service: {
    code: string;
    name: string;
    canonical_code: string;
    group: string;
    type: string;
    variant_code: string;
  };
  cost: number;
  etd: {
    min_days: number | null;
    max_days: number | null;
    text: string;
  };
  weight: {
    actual_grams: number;
    volumetric_grams: number;
    chargeable_grams: number;
    rounded_grams: number;
    billing_grams: number;
    minimum_grams: number;
    minimum_accepted_grams: number;
    minimum_billable_grams: number;
    maximum_accepted_grams: number | null;
    rounding_profile: string;
  };
  breakdown: {
    shipping: number;
    surcharge: number;
    insurance: number;
    tax: number;
    total: number;
  };
  source: {
    type: string;
    provider: string;
    verification_status: string;
    effective_from: string;
    fetched_at: string;
    is_stale: boolean;
  };
  eligibility: {
    eligible: boolean;
    weight_basis: "provided";
    evaluated_weight_grams: number;
    minimum_accepted_weight_grams: number;
    minimum_billable_weight_grams: number | null;
    maximum_accepted_weight_grams: number | null;
    source_type: string;
    source_reference: string;
    verification_status: string;
    verified_at: string;
  } | null;
};

export type TrackingEvent = {
  code: string;
  description: string;
  location?: string;
  occurred_at: string;
};

export type TrackingShipment = {
  courier: string;
  waybill: string;
  status: string;
  status_label: string;
  summary: Record<string, unknown>;
  events: TrackingEvent[];
  provider: string;
  provider_fetched_at: string | null;
  next_refresh_at: string | null;
  is_final: boolean;
  refresh_queued: boolean;
  last_error_code?: string;
	validation_status: string;
	validation_checked_at?: string | null;
	provider_hit_count: number;
	provider_hit_limit: number;
	polling_stopped: boolean;
};

export type TrackingOperationSummary = {
  total: number;
  pending: number;
  running: number;
  failed: number;
  invalid: number;
  final: number;
};

export type TrackingOperation = {
  id: string;
  tenant_id?: string;
  order_id?: string;
  fulfillment_id?: string;
  subscription_revision?: number;
  revision_history_count: number;
  courier: string;
  waybill: string;
  validation_status: string;
  status: string;
  status_label: string;
  provider: string;
  provider_fetched_at: string | null;
  next_refresh_at: string | null;
  is_final: boolean;
  last_error_code?: string;
  provider_hit_count: number;
  provider_hit_limit: number;
  queue_status: string;
  job_attempt_count: number;
  job_max_attempts: number;
  job_available_at: string | null;
  job_locked_at: string | null;
  job_locked_by?: string;
  created_at: string;
  updated_at: string;
};

export type TrackingOperationPage = {
  items: TrackingOperation[];
  total: number;
  summary: TrackingOperationSummary;
};

export type RateSnapshot = {
  id: string;
  tenant_id?: string;
  provider_credential_id?: string;
  origin_public_id: string;
  origin_label: string;
  destination_public_id: string;
  destination_label: string;
  courier_code: string;
  courier_name: string;
  service_code: string;
  service_name: string;
  description: string;
  requested_weight_grams: number;
  returned_cost: number;
  etd_min_days: number | null;
  etd_max_days: number | null;
  provider_code: string;
  verification_status: string;
  source_type: string;
  fetched_at: string;
  expires_at: string | null;
  fresh: boolean;
};

export type LocationMapping = {
  id: string;
  location_public_id: string;
  location_label: string;
  provider_code: string;
  provider_location_id: string;
  provider_location_name: string;
  granularity: string;
  verified_at: string | null;
  active: boolean;
};

export type ProviderQuota = {
  tenant_id?: string;
  provider_code: string;
  credential_alias: string;
  quota_date: string;
  daily_limit: number;
  used_count: number;
  reserved_count: number;
  remaining_count: number;
  usage_percentage: number;
  health_status: string;
  reset_at: string;
  updated_at: string;
};

export type ShippingProvider = {
  code: string;
  name: string;
  logo: string;
  description: string;
  built_in: boolean;
  requires_credential: boolean;
  available: boolean;
  display_order: number;
  installed_merchant_count: number;
  active_merchant_count: number;
  credential_count: number;
  created_at: string;
  updated_at: string;
};

export type ShippingProviderCreateInput = {
  code: string;
  name: string;
  logo: string;
  description: string;
  display_order: number;
};

export type ShippingProviderUpdateInput = Omit<
  ShippingProviderCreateInput,
  "code"
> & {
  available: boolean;
};

export type ProviderCredential = {
  id: string;
  tenant_id?: string;
  provider_code: string;
  credential_alias: string;
  display_key: string;
  daily_limit: number;
  active: boolean;
  validation_status: "valid" | "invalid";
  last_validated_at: string;
  last_selected_at: string | null;
  created_at: string;
  disabled_at: string | null;
};

export type CustomerAPIKey = {
  id: string;
  key_prefix: string;
  key_last_four: string;
  display_key: string;
  scopes: string[];
  kind: "public" | "main_service";
  active: boolean;
  last_used_at: string | null;
  created_by: string;
  created_at: string;
  revoked_by: string | null;
  revoked_at: string | null;
};

export type GeneratedCustomerAPIKey = {
  api_key: CustomerAPIKey;
  secret: string;
};

export type WebhookSettings = {
  configured: boolean;
  callback_url: string;
  enabled: boolean;
  secret_configured: boolean;
  secret_hint: string;
  source: "database" | "environment";
  last_test_at: string | null;
  last_test_success: boolean | null;
  last_test_http_status: number | null;
  last_test_error: string;
  updated_by: string;
  updated_at: string | null;
};

export type GeneratedWebhookSecret = {
  settings: WebhookSettings;
  secret: string;
};

export type WebhookTestResult = {
  success: boolean;
  http_status: number;
  event_id: string;
  tested_at: string;
  message: string;
};

export class AdminApi {
  constructor(
    private readonly key: string,
    private readonly actor: string,
  ) {}

  overview(signal?: AbortSignal) {
    return this.request<Overview>("/v1/admin/overview", { signal });
  }

  locations(search: string, signal?: AbortSignal) {
    const query = new URLSearchParams({
      search: search.trim(),
      limit: "12",
    });
    return this.request<LocationOption[]>(`/v1/admin/locations?${query}`, {
      signal,
    });
  }

  couriers(signal?: AbortSignal) {
    return this.request<Courier[]>("/v1/admin/couriers", { signal });
  }

  calculateDomesticCost(input: {
    origin: string;
    destination: string;
    weight: number;
    courier: string;
  }) {
    return this.request<RateResult[]>("/v1/admin/calculate/domestic-cost", {
      method: "POST",
      body: JSON.stringify(input),
    });
  }

  trackWaybill(input: {
    waybill: string;
    courier: string;
    last_phone_number?: string;
  }) {
    return this.request<TrackingShipment>("/v1/admin/track/waybill", {
      method: "POST",
      body: JSON.stringify({
        ...input,
        refresh: "if_stale",
      }),
    });
  }

  trackingOperations(filters: {
    search?: string;
    courier?: string;
    validation_status?: string;
    queue_status?: string;
    limit?: number;
    offset?: number;
  } = {}, signal?: AbortSignal) {
    const query = new URLSearchParams({
      limit: String(filters.limit ?? 100),
      offset: String(filters.offset ?? 0),
    });
    if (filters.search?.trim()) query.set("search", filters.search.trim());
    if (filters.courier?.trim()) query.set("courier", filters.courier.trim());
    if (filters.validation_status?.trim()) query.set("validation_status", filters.validation_status.trim());
    if (filters.queue_status?.trim()) query.set("queue_status", filters.queue_status.trim());
    return this.request<TrackingOperationPage>(
      `/v1/admin/tracking-operations?${query}`,
      { signal },
    );
  }

  deleteTrackingOperation(id: string) {
    return this.request<void>(`/v1/admin/tracking-operations/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  }

  rateSnapshots(search = "", signal?: AbortSignal) {
    const query = new URLSearchParams({ limit: "100" });
    if (search.trim()) query.set("search", search.trim());
    return this.request<RateSnapshot[]>(`/v1/admin/rate-snapshots?${query}`, {
      signal,
    });
  }

  mappings(search = "", provider = "", signal?: AbortSignal) {
    const query = new URLSearchParams({ limit: "100" });
    if (search.trim()) query.set("search", search.trim());
    if (provider.trim()) query.set("provider", provider.trim());
    return this.request<LocationMapping[]>(
      `/v1/admin/location-mappings?${query}`,
      { signal },
    );
  }

  quotas(signal?: AbortSignal) {
    return this.request<ProviderQuota[]>(
      "/v1/admin/provider-quotas?limit=90",
      { signal },
    );
  }

  shippingProviders(signal?: AbortSignal) {
    return this.request<ShippingProvider[]>("/v1/admin/shipping-providers", {
      signal,
    });
  }

  createShippingProvider(input: ShippingProviderCreateInput) {
    return this.request<ShippingProvider>("/v1/admin/shipping-providers", {
      method: "POST",
      body: JSON.stringify(input),
    });
  }

  updateShippingProvider(code: string, input: ShippingProviderUpdateInput) {
    return this.request<ShippingProvider>(
      `/v1/admin/shipping-providers/${encodeURIComponent(code)}`,
      { method: "PUT", body: JSON.stringify(input) },
    );
  }

  providerCredentials(signal?: AbortSignal) {
    return this.request<ProviderCredential[]>(
      "/v1/admin/provider-credentials",
      { signal },
    );
  }

  addProviderCredential(providerCode: string, apiKey: string) {
    return this.request<ProviderCredential>("/v1/admin/provider-credentials", {
      method: "POST",
      body: JSON.stringify({
        provider_code: providerCode,
        api_key: apiKey,
      }),
    });
  }

  disableProviderCredential(id: string) {
    return this.request<void>(
      `/v1/admin/provider-credentials/${id}/disable`,
      { method: "POST" },
    );
  }

  apiKeys(signal?: AbortSignal) {
    return this.request<CustomerAPIKey[]>("/v1/admin/api-keys?limit=100", {
      signal,
    });
  }

  generateAPIKey(kind: "public" | "main_service") {
    return this.request<GeneratedCustomerAPIKey>("/v1/admin/api-keys", {
      method: "POST",
      body: JSON.stringify({ kind }),
    });
  }

  revokeAPIKey(id: string) {
    return this.request<void>(`/v1/admin/api-keys/${id}/revoke`, {
      method: "POST",
    });
  }

  webhookSettings(signal?: AbortSignal) {
    return this.request<WebhookSettings>("/v1/admin/tracking-webhook", {
      signal,
    });
  }

  updateWebhookSettings(callbackURL: string, enabled: boolean) {
    return this.request<WebhookSettings>("/v1/admin/tracking-webhook", {
      method: "PUT",
      body: JSON.stringify({ callback_url: callbackURL, enabled }),
    });
  }

  generateWebhookSecret() {
    return this.request<GeneratedWebhookSecret>(
      "/v1/admin/tracking-webhook/secret",
      { method: "POST" },
    );
  }

  testWebhook() {
    return this.request<WebhookTestResult>(
      "/v1/admin/tracking-webhook/test",
      { method: "POST" },
    );
  }

  private async request<T>(
    path: string,
    init: RequestInit = {},
  ): Promise<T> {
    const response = await fetch(`${API_URL}${path}`, {
      ...init,
      headers: {
        Authorization: `Bearer ${this.key}`,
        "Content-Type": "application/json",
        "X-Admin-Actor": this.actor,
        ...init.headers,
      },
    });

    if (!response.ok) {
      let payload: ApiErrorEnvelope = {};
      try {
        payload = (await response.json()) as ApiErrorEnvelope;
      } catch {
        // The status code remains the authoritative fallback.
      }
      const error = new Error(
        payload.error?.message ?? `API merespons HTTP ${response.status}.`,
      );
      Object.assign(error, {
        status: response.status,
        code: payload.error?.code,
      });
      throw error;
    }
    if (response.status === 204) {
      return undefined as T;
    }
    const payload = (await response.json()) as ApiEnvelope<T>;
    return payload.data;
  }
}
