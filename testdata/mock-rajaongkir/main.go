package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/calculate/domestic-cost", calculate)
	mux.HandleFunc("/api/v1/destination/domestic-destination", destinations)
	log.Print("mock RajaOngkir listening on :18081")
	log.Fatal(http.ListenAndServe(":18081", mux))
}

func calculate(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || strings.TrimSpace(request.Header.Get("key")) == "" {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := request.ParseForm(); err != nil {
		http.Error(response, "invalid form", http.StatusBadRequest)
		return
	}
	if request.Form.Get("origin") == "" ||
		request.Form.Get("destination") == "" ||
		request.Form.Get("weight") == "" ||
		request.Form.Get("courier") == "" {
		http.Error(response, "missing form", http.StatusBadRequest)
		return
	}
	writeJSON(response, map[string]any{
		"meta": map[string]any{
			"message": "Success Calculate Domestic Shipping cost",
			"code":    200,
			"status":  "success",
		},
		"data": []map[string]any{{
			"name":        "TIKI",
			"code":        "tiki",
			"service":     "REG",
			"description": "Regular Service",
			"cost":        22000,
			"etd":         "2-4 day",
		}},
	})
}

func destinations(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || strings.TrimSpace(request.Header.Get("key")) == "" {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(response, map[string]any{
		"meta": map[string]any{
			"message": "Success Get Domestic Destinations",
			"code":    200,
			"status":  "success",
		},
		"data": []map[string]any{{
			"id":               300,
			"label":            "Lengkong, Bandung, Jawa Barat",
			"province_name":    "Jawa Barat",
			"city_name":        "Bandung",
			"district_name":    "Lengkong",
			"subdistrict_name": "",
			"zip_code":         "40261",
		}},
	})
}

func writeJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		http.Error(response, "encode error", http.StatusInternalServerError)
	}
}
