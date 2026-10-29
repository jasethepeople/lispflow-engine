package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/jasethepeople/lispflow-engine/billing"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var engine = billing.NewBillingEngine()

type EvaluateRequest struct {
	CustomerID string             `json:"customer_id"`
	PlanSexp   string             `json:"plan_sexp"`
	Usage      map[string]float64 `json:"usage"`
}

func main() {
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req EvaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// The real engine API is two calls, not one: load the plan (or reuse an
		// already-loaded one for this customer), then evaluate usage against it.
		if req.PlanSexp != "" {
			if err := engine.LoadPlan(req.CustomerID, req.PlanSexp); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}

		cost, currency, err := engine.EvaluatePricing(r.Context(), req.CustomerID, req.Usage)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"customer_id": req.CustomerID,
			"cost":        cost,
			"currency":    currency,
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy"}`))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.ListenAndServe(":"+port, mux)
}
