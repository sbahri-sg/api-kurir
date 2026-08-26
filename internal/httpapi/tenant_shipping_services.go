package httpapi

import (
	"errors"
	"net/http"

	"github.com/emisell/api-kurir/internal/merchantshipping"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

type updateTenantShippingServicesRequest struct {
	Mode          string                       `json:"mode"`
	EnabledGroups []string                     `json:"enabled_groups"`
	Services      []merchantshipping.Selection `json:"services"`
}

func tenantShippingServiceCatalogHandler(
	service *merchantshipping.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		result, err := service.Catalog(c.Request().Context(), identity.TenantID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func tenantShippingServiceUpdateHandler(
	service *merchantshipping.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		var request updateTenantShippingServicesRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.Update(c.Request().Context(), identity.TenantID, merchantshipping.UpdateInput{
			Mode:          request.Mode,
			EnabledGroups: request.EnabledGroups,
			Services:      request.Services,
			UpdatedBy:     "tenant:" + identity.TenantID,
		})
		var limitError *merchantshipping.SelectionLimitError
		switch {
		case errors.Is(err, merchantshipping.ErrShippingDisabled):
			return writeError(
				c,
				http.StatusConflict,
				"SHIPPING_DISABLED",
				"Merchant belum mengaktifkan provider pengiriman.",
				nil,
			)
		case errors.As(err, &limitError):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"SHIPPING_SERVICE_LIMIT_EXCEEDED",
				"Pilihan kurir atau layanan melebihi limit merchant.",
				map[string]any{
					"max_couriers":       limitError.MaxCouriers,
					"max_services":       limitError.MaxServices,
					"requested_couriers": limitError.RequestedCouriers,
					"requested_services": limitError.RequestedServices,
				},
			)
		case errors.Is(err, merchantshipping.ErrInvalidPreference):
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_SHIPPING_SERVICE_PREFERENCE",
				"Hanya mode custom yang didukung dan enabled_groups harus kosong.",
				nil,
			)
		case errors.Is(err, merchantshipping.ErrUnknownService):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"SHIPPING_SERVICE_NOT_FOUND",
				"Salah satu layanan tidak tersedia pada katalog aktif.",
				nil,
			)
		case err != nil:
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}
