package fulfillment

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFulfillmentRepositoryTenantIsolationAndEncryptedPayloadIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cipher, err := providercredentials.NewCipher("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool, cipher)
	suffix := uint64(time.Now().UnixNano()) & 0xffffffffffff
	shipmentID := fmt.Sprintf("00000000-0000-4000-8000-%012x", suffix)
	tenantID := fmt.Sprintf("fulfillment_%d", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM fulfillment_shipments WHERE id = $1::uuid", shipmentID)
	}()

	plaintext := []byte(`{"recipient":{"phone":"081234567890"}}`)
	shipment, created, err := repository.ReserveCreate(ctx, ReserveCreateInput{
		ID: shipmentID, TenantID: tenantID, ProviderCode: "rajaongkir",
		MerchantReference: "ORDER-" + fmt.Sprint(suffix), QuoteID: "quote-1",
		CourierCode: "jne", ServiceCode: "REG", DeliveryMode: "regular",
		Fulfillment: "pickup", ShippingCost: 18000, Currency: "IDR",
		IdempotencyKey: "shipment:create:" + fmt.Sprint(suffix),
		RequestHash:    []byte("request-hash"), RequestCiphertext: plaintext,
	})
	if err != nil || !created || shipment.ID != shipmentID {
		t.Fatalf("reserve shipment=%+v created=%v err=%v", shipment, created, err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, "SELECT request_ciphertext FROM fulfillment_shipments WHERE id = $1::uuid", shipmentID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("081234567890")) || bytes.Equal(stored, plaintext) {
		t.Fatal("PII request was stored as plaintext")
	}
	completed, err := repository.CompleteCreate(ctx, tenantID, shipmentID, ProviderCreateResult{
		ProviderShipmentID: "KOM-100", Status: StatusBooked,
	})
	if err != nil || completed.ProviderShipmentID != "KOM-100" {
		t.Fatalf("complete=%+v err=%v", completed, err)
	}
	var lifecycleJobCount, webhookCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM fulfillment_lifecycle_jobs
		WHERE shipment_id = $1::uuid AND job_type = 'reconcile' AND status = 'pending'
	`, shipmentID).Scan(&lifecycleJobCount); err != nil {
		t.Fatal(err)
	}
	if lifecycleJobCount != 1 {
		t.Fatalf("expected one pending reconciliation job, got %d", lifecycleJobCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM fulfillment_webhook_outbox
		WHERE shipment_id = $1::uuid AND event_type = 'shipment.booked'
	`, shipmentID).Scan(&webhookCount); err != nil {
		t.Fatal(err)
	}
	if webhookCount != 1 {
		t.Fatalf("expected one shipment.booked webhook, got %d", webhookCount)
	}
	shipmentWithAWBID := fmt.Sprintf("10000000-0000-4000-8000-%012x", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM fulfillment_shipments WHERE id = $1::uuid", shipmentWithAWBID)
	}()
	_, created, err = repository.ReserveCreate(ctx, ReserveCreateInput{
		ID: shipmentWithAWBID, TenantID: tenantID, ProviderCode: "rajaongkir",
		MerchantReference: "ORDER-AWB-" + fmt.Sprint(suffix), QuoteID: "quote-awb",
		CourierCode: "jne", ServiceCode: "REG", DeliveryMode: "regular",
		Fulfillment: "pickup", ShippingCost: 18000, Currency: "IDR",
		IdempotencyKey: "shipment:create:awb:" + fmt.Sprint(suffix),
		RequestHash:    []byte("request-hash-awb"), RequestCiphertext: plaintext,
	})
	if err != nil || !created {
		t.Fatalf("reserve shipment with AWB created=%v err=%v", created, err)
	}
	withAWB, err := repository.CompleteCreate(ctx, tenantID, shipmentWithAWBID, ProviderCreateResult{
		ProviderShipmentID: "KOM-AWB-100", AWB: "TESTAWB123", Status: StatusBooked,
	})
	if err != nil {
		t.Fatal(err)
	}
	if withAWB.TrackingRegistrationStatus != "pending" {
		t.Fatalf("tracking registration status=%q want pending", withAWB.TrackingRegistrationStatus)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM fulfillment_lifecycle_jobs
		WHERE shipment_id = $1::uuid AND job_type = 'register_tracking' AND status = 'pending'
	`, shipmentWithAWBID).Scan(&lifecycleJobCount); err != nil {
		t.Fatal(err)
	}
	if lifecycleJobCount != 1 {
		t.Fatalf("expected immediate tracking registration job, got %d", lifecycleJobCount)
	}
	if _, err := repository.Get(ctx, tenantID+"_other", shipmentID); err != ErrShipmentNotFound {
		t.Fatalf("cross-tenant get should be hidden, got %v", err)
	}
	history, err := repository.History(ctx, tenantID, shipmentID)
	if err != nil || len(history) != 2 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	label := Label{Format: "page_5", ContentType: "application/pdf", Base64: "JVBERi0="}
	if err := repository.SaveLabel(ctx, tenantID, shipmentID, label); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.GetLabel(ctx, tenantID, shipmentID, "page_5")
	if err != nil || loaded.Base64 != label.Base64 {
		t.Fatalf("label=%+v err=%v", loaded, err)
	}
}
