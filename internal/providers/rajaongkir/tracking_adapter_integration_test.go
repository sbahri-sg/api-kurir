package rajaongkir

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRajaOngkirTrackingWorkerLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	waybill := fmt.Sprintf("INT%d", time.Now().UTC().UnixNano())
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		providerCalls.Add(1)
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Error(readErr)
			response.WriteHeader(http.StatusInternalServerError)
			return
		}
		form, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			t.Error(parseErr)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		if form.Get("awb") != waybill ||
			form.Get("courier") != "jne" ||
			form.Has("last_phone_number") {
			t.Errorf("unexpected provider form: %v", form)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"meta":{"message":"success","code":200},
			"data":{
				"delivered":true,
				"summary":{
					"courier_code":"jne",
					"courier_name":"JNE",
					"service_code":"REG",
					"origin":"Jakarta",
					"destination":"Bandung",
					"status":"DELIVERED"
				},
				"details":{"weight":"1"},
				"delivery_status":{
					"status":"DELIVERED",
					"pod_date":"2026-07-28",
					"pod_time":"10:15"
				},
				"manifest":[{
					"manifest_code":"DELIVERED",
					"manifest_description":"Package delivered",
					"manifest_date":"2026-07-28",
					"manifest_time":"10:15",
					"city_name":"Bandung"
				}]
			}
		}`))
	}))
	defer provider.Close()

	client, err := NewClient(
		provider.URL+"/api/v1/",
		"integration-dummy-key",
		provider.Client(),
	)
	if err != nil {
		t.Fatal(err)
	}
	alias := "tracking-integration-" + time.Now().UTC().Format("20060102150405.000000000")
	rateRepository := rates.NewPostgresRepository(pool)
	trackingRepository := tracking.NewPostgresRepository(pool)
	adapter := NewTrackingAdapter(
		client,
		rateRepository,
		rateRepository,
		alias,
		50_000,
		[]string{"jne"},
	)
	cipherKey := bytes.Repeat([]byte{23}, 32)
	cipher, err := tracking.NewCipher(base64.StdEncoding.EncodeToString(cipherKey))
	if err != nil {
		t.Fatal(err)
	}
	service := tracking.NewService(trackingRepository, cipher, "jne")
	shipment, err := service.Register(
		ctx,
		"jne",
		waybill,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM tracking_shipments WHERE id = $1::uuid",
			shipment.ID,
		)
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM provider_api_calls WHERE credential_alias = $1",
			alias,
		)
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM provider_quota_ledger WHERE credential_alias = $1",
			alias,
		)
	})

	runnerContext, stopRunner := context.WithCancel(ctx)
	runner := tracking.NewRunner(
		trackingRepository,
		cipher,
		[]tracking.Adapter{adapter},
		"integration-worker",
		2,
		time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	runnerDone := make(chan error, 1)
	go func() {
		runnerDone <- runner.Run(runnerContext)
	}()

	var status string
	var final bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err = pool.QueryRow(ctx, `
			SELECT normalized_status, is_final
			FROM tracking_shipments
			WHERE id = $1::uuid
		`, shipment.ID).Scan(&status, &final)
		if err == nil && status == "delivered" && final {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopRunner()
	if runnerErr := <-runnerDone; runnerErr != nil {
		t.Fatal(runnerErr)
	}
	if status != "delivered" || !final {
		t.Fatalf("unexpected shipment state: status=%s final=%t", status, final)
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("provider calls: got %d want 1", providerCalls.Load())
	}

	var storedWaybill, storedContext, storedSummary []byte
	if err := pool.QueryRow(ctx, `
		SELECT waybill_ciphertext, provider_context_ciphertext, summary_json::text
		FROM tracking_shipments
		WHERE id = $1::uuid
	`, shipment.ID).Scan(&storedWaybill, &storedContext, &storedSummary); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(storedWaybill, []byte(waybill)) ||
		bytes.Contains(storedContext, []byte("54321")) ||
		bytes.Contains(storedSummary, []byte("54321")) {
		t.Fatal("tracking plaintext leaked into persisted shipment data")
	}

	var usedCount, callCount int
	if err := pool.QueryRow(ctx, `
		SELECT used_count
		FROM provider_quota_ledger
		WHERE provider_code = 'rajaongkir'
		  AND credential_alias = $1
	`, alias).Scan(&usedCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM provider_api_calls
		WHERE provider_code = 'rajaongkir'
		  AND credential_alias = $1
		  AND endpoint = 'track/waybill'
		  AND outcome = 'success'
	`, alias).Scan(&callCount); err != nil {
		t.Fatal(err)
	}
	if usedCount != 1 || callCount != 1 {
		t.Fatalf("unexpected accounting: quota=%d calls=%d", usedCount, callCount)
	}
}
