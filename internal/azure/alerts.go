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
	service := serviceFromResourceID(target)
	id := ess.AlertID
	if id == "" {
		id = ess.AlertRule + "|" + target
	}
	return []domain.Alert{{
		ID: id, Fingerprint: id, Name: ess.AlertRule, Service: service, Severity: ess.Severity,
		Status: status, Summary: ess.Description, StartsAt: ess.FiredTime,
		Labels: map[string]string{
			"signal_type": ess.SignalType,
			"resource_id": target,
			"alert_rule":  ess.AlertRule,
		},
	}}, nil
}

func serviceFromResourceID(id string) string {
	id = strings.Trim(id, "/")
	if id == "" {
		return ""
	}
	parts := strings.Split(id, "/")
	return parts[len(parts)-1]
}
