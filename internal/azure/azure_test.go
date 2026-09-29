package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInventoryAndPromQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-token") {
			http.Error(w, "no token", 401)
			return
		}
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(r.URL.Path, "ResourceGraph"):
			if !strings.Contains(string(b), "resources") {
				t.Fatalf("body %s", b)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.Storage/storageAccounts/shopsa","name":"shopsa","type":"Microsoft.Storage/storageAccounts","location":"eastus","tags":{"sentinelmesh.service":"inventory-service"}}]}`))
		case strings.Contains(r.URL.Path, "/api/v1/query"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"0.2"]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := Client{Credential: StaticToken{Token: "test-token"}, HTTP: WithBase(srv.URL, srv.Client().Transport)}
	resources, err := c.Inventory(context.Background(), InventoryQuery{Subscriptions: []string{"00000000-0000-0000-0000-000000000000"}, Query: "resources"})
	if err != nil || len(resources) != 1 || resources[0].Service != "inventory-service" {
		t.Fatalf("%+v %v", resources, err)
	}
	v, ok, err := c.PromQL(context.Background(), srv.URL, "up")
	if err != nil || !ok || v != 0.2 {
		t.Fatalf("v=%v ok=%v err=%v", v, ok, err)
	}
}

func TestParseCommonAlert(t *testing.T) {
	body := []byte(`{"schemaId":"azureMonitorCommonAlertSchema","data":{"essentials":{"alertId":"a1","alertRule":"storage-throttle","severity":"Sev2","signalType":"Metric","monitorCondition":"Fired","alertTargetIDs":["/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.Storage/storageAccounts/shopsa"],"firedDateTime":"2026-09-29T12:00:00Z","description":"throttled"}}}`)
	alerts, err := ParseAlerts(body)
	if err != nil || len(alerts) != 1 || alerts[0].Status != "firing" || alerts[0].Service != "shopsa" {
		t.Fatalf("%+v %v", alerts, err)
	}
	rows := ChangesFromActivity([]map[string]any{{
		"OperationNameValue":  "Microsoft.Storage/storageAccounts/write",
		"ActivityStatusValue": "Success",
		"ResourceId":          "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.Storage/storageAccounts/shopsa",
		"Caller":              "00000000-0000-0000-0000-000000000000",
		"CorrelationId":       "c1",
	}}, time.Now().UTC())
	if len(rows) != 1 || rows[0].Actor != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("%+v", rows)
	}
}
