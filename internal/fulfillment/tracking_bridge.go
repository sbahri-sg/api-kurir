package fulfillment

import (
	"context"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/emisell/api-kurir/internal/tracking"
)

type TrackingServiceRegistrar struct {
	service *tracking.Service
}

func NewTrackingServiceRegistrar(service *tracking.Service) *TrackingServiceRegistrar {
	return &TrackingServiceRegistrar{service: service}
}

func (r *TrackingServiceRegistrar) RegisterFulfillmentTracking(
	ctx context.Context,
	input TrackingRegistration,
) (string, error) {
	if r == nil || r.service == nil {
		return "", tracking.ErrAdapterUnavailable
	}
	ctx = tenancy.WithIdentity(ctx, tenancy.Identity{TenantID: input.TenantID})
	subscription, err := r.service.Subscribe(ctx, tracking.SubscriptionRequest{
		OrderReference:       input.OrderReference,
		FulfillmentReference: input.FulfillmentReference,
		CourierCode:          input.CourierCode,
		Waybill:              input.Waybill,
	})
	if err != nil {
		return "", err
	}
	return subscription.Shipment.ID, nil
}
