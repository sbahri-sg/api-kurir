package tracking

import "github.com/emisell/api-kurir/internal/couriers"

func normalizeCourierCode(value string) string {
	return couriers.NormalizeCode(value)
}

func normalizeResultCourier(result Result, requestedCourier string) Result {
	courierCode := ""
	if result.Summary != nil {
		if providerValue, ok := result.Summary["courier_code"].(string); ok {
			courierCode = normalizeCourierCode(providerValue)
		}
	}
	if courierCode == "" {
		courierCode = normalizeCourierCode(requestedCourier)
	}
	if courierCode == "" {
		return result
	}
	if result.Summary == nil {
		result.Summary = make(map[string]any)
	}
	result.Summary["courier_code"] = courierCode
	return result
}
