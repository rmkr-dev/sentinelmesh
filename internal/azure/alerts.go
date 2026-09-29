package azure

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// CommonAlert is the Azure Monitor common alert schema payload.
type CommonAlert struct {
	SchemaID string `json:"schemaId"`
	Data     struct {
		Essentials struct {
			AlertID          string    `json:"alertId"`
			AlertRule        string    `json:"alertRule"`
			Severity         string    `json:"severity"`
			SignalType       string    `json:"signalType"`
			MonitorCondition string    `json:"monitorCondition"`
			TargetIDs        []string  `json:"alertTargetIDs"`
			FiredTime        time.Time `json:"firedDateTime"`
			Description      string    `json:"description"`
		} `json:"essentials"`
	} `json:"data"`
}

// ParseAlerts normalizes a common alert schema document into domain alerts.
func ParseAlerts(body []byte) ([]domain.Alert, error) {
	var doc CommonAlert
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	ess := doc.Data.Essentials
	status := "firing"
	if strings.EqualFold(ess.MonitorCondition, "Resolved") {
		status = "resolved"
	}
	target := ""
	if len(ess.TargetIDs) > 0 {
		target = ess.TargetIDs[0]
	}
	id := ess.AlertID
	if id == "" {
		id = ess.AlertRule + "|" + target
	}
	return []domain.Alert{{
		ID: id, Fingerprint: id, Name: ess.AlertRule, Service: "", Severity: platformSeverity(ess.Severity),
		Status: status, Summary: ess.Description, StartsAt: ess.FiredTime,
		Labels: map[string]string{
			"signal_type":    ess.SignalType,
			"resource_id":    strings.ToLower(target),
			"alert_rule":     ess.AlertRule,
			"azure_severity": ess.Severity,
		},
	}}, nil
}

func platformSeverity(sev string) string {
	switch strings.ToLower(sev) {
	case "sev0", "sev1":
		return "critical"
	case "sev2", "sev3":
		return "warning"
	case "sev4":
		return "info"
	default:
		return "warning"
	}
}

// BindService sets the catalog service when azure.resource_ids contains the alert target.
// An unbound alert keeps an empty service so it cannot match every Kubernetes object.
func BindService(alert domain.Alert, services []domain.Service) domain.Alert {
	target := strings.ToLower(alert.Labels["resource_id"])
	if target == "" {
		return alert
	}
	for _, svc := range services {
		raw := svc.Attributes["azure.resource_ids"]
		for _, part := range strings.Split(raw, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part != "" && part == target {
				alert.Service = svc.Name
				return alert
			}
		}
	}
	alert.Service = ""
	return alert
}
