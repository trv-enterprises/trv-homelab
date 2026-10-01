package ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

// sensorAlert is Marshal's wire format on sensors/alerts. All strings, by
// design (trv-marshal internal/alerter/alerter.go). source is always
// "alert_engine" and is not checked: the topic is the contract.
type sensorAlert struct {
	Type      string `json:"type"` // new | repeat | resolved
	Source    string `json:"source"`
	Severity  string `json:"severity"`
	Rule      string `json:"rule"`
	Message   string `json:"message"`
	Device    string `json:"device"`
	Timestamp string `json:"timestamp"`
}

// parseSensor validates a sensors/alerts payload.
func parseSensor(payload []byte, now time.Time) (sensorAlert, store.SensorMsg, error) {
	var a sensorAlert
	if err := json.Unmarshal(payload, &a); err != nil {
		return a, store.SensorMsg{}, fmt.Errorf("decode: %w", err)
	}
	if a.Rule == "" {
		return a, store.SensorMsg{}, fmt.Errorf("missing rule")
	}
	switch a.Type {
	case "new", "repeat", "resolved":
	default:
		return a, store.SensorMsg{}, fmt.Errorf("unknown type %q", a.Type)
	}
	switch a.Severity {
	case model.SeverityInfo, model.SeverityWarning, model.SeverityCritical:
	case "":
		a.Severity = model.SeverityWarning
	default:
		a.Severity = model.SeverityWarning
	}
	at := now
	if t, err := time.Parse(time.RFC3339, a.Timestamp); err == nil {
		at = t
	}
	return a, store.SensorMsg{
		Rule: a.Rule, Device: a.Device, Severity: a.Severity, Message: a.Message, At: at, Raw: string(payload),
	}, nil
}
