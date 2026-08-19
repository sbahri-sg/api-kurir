package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/labstack/echo/v5"
)

const legacyRegionProvider = "rajaongkir"

var legacyDefaultCouriers = []string{
	"jne",
	"sicepat",
	"ide",
	"sap",
	"jnt",
	"ninja",
	"tiki",
	"lion",
	"anteraja",
	"pos",
	"ncs",
	"rex",
	"rpx",
	"sentral",
	"star",
	"wahana",
	"dse",
}

var legacyServiceNames = map[string]struct{}{
	"REG":      {},
	"EXPRESS":  {},
	"SAMEDAY":  {},
	"ECONOMY":  {},
	"TRUCKING": {},
	"CARGO":    {},
	"MOTOR":    {},
	"OTHER":    {},
}

type legacyRegionStore interface {
	locations.Repository
	locations.LegacyRepository
}

func registerLegacyRegionRoutes(
	e *echo.Echo,
	rateService *rates.Service,
	repository legacyRegionStore,
	customerAPIKeyService customerKeyAuthenticator,
	apiKeys []string,
) {
	group := e.Group("")
	group.Use(rajaOngkirV2CompatibilityMiddleware())
	group.Use(customerAPIKeyMiddleware(apiKeys, customerAPIKeyService))

	group.GET(
		"/regions/provinces",
		legacyHierarchyListHandler(repository, "province", ""),
	)
	group.GET(
		"/regions/cities",
		legacyHierarchyListHandler(repository, "city", "provinceId"),
	)
	group.GET(
		"/regions/districts",
		legacyHierarchyListHandler(repository, "district", "cityId"),
	)
	group.GET(
		"/regions/subdistricts",
		legacyHierarchyListHandler(repository, "subdistrict", "districtId"),
	)
	group.GET(
		"/regions/provinces/:id",
		legacyRegionByIDHandler(repository, "province"),
	)
	group.GET(
		"/regions/cities/:id",
		legacyRegionByIDHandler(repository, "city"),
	)
	group.GET(
		"/regions/districts/:id",
		legacyRegionByIDHandler(repository, "district"),
	)
	group.GET(
		"/shipping/domestic-cost",
		legacyDomesticCostHandler(rateService, repository),
	)
}

func legacyHierarchyListHandler(
	repository locations.LegacyRepository,
	level string,
	parentQuery string,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		parentID := ""
		if parentQuery != "" {
			parentID = strings.TrimSpace(c.QueryParam(parentQuery))
			if parentID == "" {
				return legacyRegionError(
					c,
					http.StatusBadRequest,
					parentQuery+" must be a valid ID",
				)
			}
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 2*time.Second)
		defer cancel()
		regions, err := repository.ListLegacyHierarchy(
			ctx,
			legacyRegionProvider,
			level,
			parentID,
		)
		if errors.Is(err, locations.ErrLocationNotFound) {
			return legacyRegionError(c, http.StatusNotFound, "Region not found")
		}
		if err != nil {
			return err
		}

		data := make([]map[string]any, 0, len(regions))
		for _, region := range regions {
			data = append(data, legacyRegionResponse(region, level, false))
		}
		return c.JSON(http.StatusOK, map[string]any{"data": data})
	}
}

func legacyRegionByIDHandler(
	repository locations.LegacyRepository,
	level string,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identifier := strings.TrimSpace(c.Param("id"))
		if identifier == "" {
			return legacyRegionError(c, http.StatusBadRequest, "ID must be valid")
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 2*time.Second)
		defer cancel()
		region, err := repository.FindLegacyRegion(
			ctx,
			legacyRegionProvider,
			level,
			identifier,
		)
		if errors.Is(err, locations.ErrLocationNotFound) {
			return legacyRegionError(c, http.StatusNotFound, "Region not found")
		}
		if err != nil {
			return err
		}
		return c.JSON(
			http.StatusOK,
			map[string]any{"data": legacyRegionResponse(region, level, true)},
		)
	}
}

func legacyRegionResponse(
	region locations.LegacyRegion,
	level string,
	detail bool,
) map[string]any {
	result := map[string]any{
		"id":   legacyExternalID(region.ID),
		"name": region.Name,
	}
	switch level {
	case "district":
		result["zipCode"] = region.PostalCode
		if detail {
			result["cityId"] = legacyExternalID(region.CityID)
			result["cityName"] = region.CityName
			result["provinceId"] = legacyExternalID(region.ProvinceID)
			result["provinceName"] = region.ProvinceName
		}
	case "subdistrict":
		result["zipCode"] = region.PostalCode
		result["districtId"] = legacyExternalID(region.DistrictID)
		result["cityId"] = legacyExternalID(region.CityID)
		result["provinceId"] = legacyExternalID(region.ProvinceID)
	}
	return result
}

func legacyExternalID(identifier string) any {
	identifier = strings.TrimSpace(identifier)
	if value, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return value
	}
	return identifier
}

func legacyDomesticCostHandler(
	service *rates.Service,
	repository legacyRegionStore,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		originID := strings.TrimSpace(c.QueryParam("origin"))
		destinationID := strings.TrimSpace(c.QueryParam("destination"))
		weight, err := strconv.ParseInt(strings.TrimSpace(c.QueryParam("weight")), 10, 64)
		if originID == "" || destinationID == "" || err != nil {
			return legacyRegionError(
				c,
				http.StatusBadRequest,
				"origin, destination, and weight are required",
			)
		}

		courierValue := strings.TrimSpace(c.QueryParam("courier"))
		if courierValue == "" {
			courierValue = strings.Join(legacyDefaultCouriers, ":")
		}
		couriers, err := normalizeCourierCodes(courierValue)
		if err != nil {
			return legacyRegionError(c, http.StatusBadRequest, err.Error())
		}
		priceFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("price")))
		if priceFilter != "" && priceFilter != "lowest" && priceFilter != "highest" {
			return legacyRegionError(c, http.StatusBadRequest, "price must be lowest or highest")
		}
		serviceFilter, err := normalizeLegacyServiceFilter(c.QueryParam("serviceName"))
		if err != nil {
			return legacyRegionError(c, http.StatusBadRequest, err.Error())
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 6*time.Second)
		defer cancel()
		origin, err := resolveLegacyDistrict(ctx, repository, originID)
		if err != nil {
			return legacyRegionError(c, http.StatusBadRequest, "origin is not valid")
		}
		destination, err := resolveLegacyDistrict(ctx, repository, destinationID)
		if err != nil {
			return legacyRegionError(c, http.StatusBadRequest, "destination is not valid")
		}

		request := rates.Request{
			Origin:            origin,
			Destination:       destination,
			Granularity:       "district",
			PriceFilter:       priceFilter,
			ActualWeightGrams: weight,
			Couriers:          couriers,
		}
		if _, err := normalizeCalculateRequest(calculateRequest{
			Origin:      origin,
			Destination: destination,
			Weight:      weight,
			Courier:     courierValue,
			Price:       priceFilter,
		}); err != nil {
			return legacyRegionError(c, http.StatusBadRequest, err.Error())
		}

		results, err := service.Calculate(ctx, request)
		if err != nil {
			return writeRajaOngkirRateError(c, request, err)
		}
		data := make([]map[string]any, 0, len(results))
		for _, result := range results {
			serviceName := legacyServiceName(result)
			if len(serviceFilter) > 0 {
				if _, included := serviceFilter[serviceName]; !included {
					continue
				}
			}
			data = append(data, map[string]any{
				"name":        result.Card.CourierName,
				"code":        result.Card.CourierCode,
				"service":     result.Card.ServiceCode,
				"description": result.Card.ServiceName,
				"cost":        result.Cost.Total,
				"etd":         rajaOngkirETD(result),
				"serviceName": serviceName,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message": "Success Calculate Domestic Shipping cost",
				"code":    http.StatusOK,
				"status":  "success",
			},
			"data": data,
		})
	}
}

func resolveLegacyDistrict(
	ctx context.Context,
	repository legacyRegionStore,
	identifier string,
) (string, error) {
	if strings.HasPrefix(identifier, "loc_") {
		return repository.ResolvePublicID(ctx, identifier, "district")
	}
	return repository.ResolveLegacyPublicID(
		ctx,
		legacyRegionProvider,
		"district",
		identifier,
	)
}

func normalizeLegacyServiceFilter(value string) (map[string]struct{}, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	result := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		name := strings.ToUpper(strings.TrimSpace(item))
		if _, valid := legacyServiceNames[name]; !valid {
			return nil, errors.New("serviceName is not valid")
		}
		result[name] = struct{}{}
	}
	return result, nil
}

func legacyServiceName(result rates.Result) string {
	courier := strings.ToLower(strings.TrimSpace(result.Card.CourierCode))
	service := strings.ToUpper(strings.TrimSpace(result.Card.ServiceCode))
	canonical := strings.ToUpper(strings.TrimSpace(result.Card.CanonicalServiceCode))
	if canonical == "" {
		canonical = service
	}

	if isLegacyMotorService(courier, service) {
		return "MOTOR"
	}
	if isLegacyTruckingService(courier, service, canonical) {
		return "TRUCKING"
	}
	switch strings.ToLower(strings.TrimSpace(result.Card.ServiceGroup)) {
	case "regular":
		return "REG"
	case "economy":
		return "ECONOMY"
	case "next_day", "express", "instant":
		return "EXPRESS"
	case "same_day":
		return "SAMEDAY"
	case "cargo":
		return "CARGO"
	default:
		return "OTHER"
	}
}

func isLegacyMotorService(courier string, service string) bool {
	switch courier + ":" + service {
	case "dse:MOTOR",
		"dse:MOTOR SPORT",
		"pos:POS KARGO MOTOR",
		"rex:M-100",
		"rex:M-150",
		"tiki:T15",
		"tiki:T25",
		"tiki:T60":
		return true
	default:
		return false
	}
}

func isLegacyTruckingService(courier string, service string, canonical string) bool {
	switch courier {
	case "jne":
		return strings.HasPrefix(service, "JTR") || canonical == "JTR"
	case "ide":
		return strings.EqualFold(service, "IDTRUCK")
	case "tiki":
		return service == "TRC" || canonical == "TRC"
	case "wahana":
		return service == "KARGO"
	case "pos":
		return service == "POS KARGO"
	case "ncs":
		return service == "DARAT"
	case "dse":
		return strings.HasPrefix(service, "DARAT") || service == "LAUT"
	case "sentral":
		return strings.HasPrefix(service, "DARAT")
	default:
		return strings.Contains(service, "TRUCKING")
	}
}

func legacyRegionError(c *echo.Context, status int, message string) error {
	return c.JSON(status, map[string]any{"message": message})
}
