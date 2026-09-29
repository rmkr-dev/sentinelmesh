package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

type memSink struct {
	mu        sync.Mutex
	resources []domain.Resource
	changes   []domain.Change
	health    []domain.HealthEvent
}

func (m *memSink) SaveResource(_ context.Context, r domain.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, r)
	return nil
}
func (m *memSink) SaveChange(_ context.Context, c domain.Change) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, c)
	return nil
}
func (m *memSink) SaveHealth(_ context.Context, h domain.HealthEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = append(m.health, h)
	return nil
}

func TestPollerPagesInventoryAndStampsEventTime(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-token") {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(r.URL.Path, "ResourceGraph"):
			pages++
			if pages == 1 {
				_, _ = w.Write([]byte(`{"data":[{"id":"/subscriptions/s/resourceGroups/g/providers/Microsoft.Storage/storageAccounts/shopsa","name":"shopsa","type":"Microsoft.Storage/storageAccounts","location":"eastus","tags":{"sentinelmesh.service":"inventory-service"}}],"$skipToken":"next"}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"/subscriptions/s/resourceGroups/g/providers/Microsoft.Web/sites/fn","name":"fn","type":"Microsoft.Web/sites","location":"eastus"}]}`))
		default:
			if !strings.Contains(string(b), "ResourceHealth") {
				_, _ = w.Write([]byte(`{"tables":[{"columns":[{"name":"TimeGenerated"},{"name":"OperationNameValue"},{"name":"ActivityStatusValue"},{"name":"ResourceId"},{"name":"Caller"},{"name":"CorrelationId"}],"rows":[["2026-09-29T11:00:00Z","Microsoft.Storage/storageAccounts/write","Success","/subscriptions/s/resourceGroups/g/providers/Microsoft.Storage/storageAccounts/shopsa","alice","corr-1"]]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"tables":[{"columns":[{"name":"TimeGenerated"},{"name":"ResourceId"},{"name":"ActivityStatusValue"},{"name":"CorrelationId"}],"rows":[["2026-09-29T11:05:00Z","/subscriptions/s/resourceGroups/g/providers/Microsoft.Storage/storageAccounts/shopsa","Unavailable","health-1"]]}]}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	if err := writeType(dir); err != nil {
		t.Fatal(err)
	}
	types, err := LoadResourceTypes(dir)
	if err != nil || len(types) != 1 {
		t.Fatal(err)
	}
	sink := &memSink{}
	err = Poller{
		Client:       Client{Credential: StaticToken{Token: "test-token"}, HTTP: srv.Client(), BaseURL: srv.URL},
		Subscription: "s",
		WorkspaceID:  "ws",
		Types:        types,
		Now:          func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
	}.Poll(context.Background(), sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.resources) != 2 || sink.resources[0].Service != "inventory-service" {
		t.Fatalf("resources %+v", sink.resources)
	}
	if len(sink.changes) != 1 || sink.changes[0].ID != "corr-1" || !sink.changes[0].OccurredAt.Equal(time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("changes %+v", sink.changes)
	}
	if len(sink.health) != 1 || sink.health[0].At.IsZero() {
		t.Fatalf("health %+v", sink.health)
	}
}

func writeType(dir string) error {
	return os.WriteFile(dir+"/storage.yaml", []byte("type: Microsoft.Storage/storageAccounts\nsignals:\n  - metric: Availability\n"), 0o644)
}
