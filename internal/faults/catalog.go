// Package faults is the deterministic fault catalog used by the demo.
package faults

// Spec describes one injectable fault and the telemetry it is expected to produce.
type Spec struct {
	Name               string   `json:"name"`
	Service            string   `json:"service"`
	Kind               string   `json:"kind"`
	Description        string   `json:"description"`
	ExpectedTelemetry  []string `json:"expected_telemetry"`
	ExpectedAlert      string   `json:"expected_alert"`
	ExpectedHypothesis string   `json:"expected_hypothesis"`
}

// Catalog is the supported demo fault set.
func Catalog() []Spec {
	return []Spec{
		{
			Name: "payment-latency", Service: "payment-service", Kind: "latency",
			Description:        "Adds 800ms to every payment charge.",
			ExpectedTelemetry:  []string{"latency histogram shift", "slow server spans on payment-service"},
			ExpectedAlert:      "PaymentHighLatency",
			ExpectedHypothesis: "latency",
		},
		{
			Name: "inventory-errors", Service: "inventory-service", Kind: "errors",
			Description:        "Inventory reserve returns HTTP 500.",
			ExpectedTelemetry:  []string{"5xx on inventory-service", "error spans", "error logs"},
			ExpectedAlert:      "HighErrorRate",
			ExpectedHypothesis: "dependency",
		},
		{
			Name: "database-timeout", Service: "inventory-service", Kind: "db_timeout",
			Description:        "Inventory database calls sleep and then fail.",
			ExpectedTelemetry:  []string{"long db spans", "timeout logs"},
			ExpectedAlert:      "HighErrorRate",
			ExpectedHypothesis: "database",
		},
		{
			Name: "cpu-pressure", Service: "payment-service", Kind: "cpu",
			Description:        "Burns CPU on each payment request.",
			ExpectedTelemetry:  []string{"latency increase", "process cpu time"},
			ExpectedAlert:      "PaymentHighLatency",
			ExpectedHypothesis: "saturation",
		},
		{
			Name: "memory-pressure", Service: "payment-service", Kind: "memory",
			Description:        "Retains memory on each payment request.",
			ExpectedTelemetry:  []string{"process memory growth", "latency increase"},
			ExpectedAlert:      "PaymentHighLatency",
			ExpectedHypothesis: "saturation",
		},
		{
			Name: "dependency-outage", Service: "payment-service", Kind: "outage",
			Description:        "Payment returns HTTP 503 for every charge.",
			ExpectedTelemetry:  []string{"5xx on payment-service", "failed client spans from order-service"},
			ExpectedAlert:      "HighErrorRate",
			ExpectedHypothesis: "dependency",
		},
		{
			Name: "deployment-regression", Service: "payment-service", Kind: "regression",
			Description:        "Records payment-service v1.8.2 and fails most charges with added latency.",
			ExpectedTelemetry:  []string{"deployment marker", "error rate", "latency"},
			ExpectedAlert:      "HighErrorRate",
			ExpectedHypothesis: "deployment correlation",
		},
	}
}

// Find returns a catalog entry.
func Find(name string) (Spec, bool) {
	for _, s := range Catalog() {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}
