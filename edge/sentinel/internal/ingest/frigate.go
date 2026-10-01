package ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/store"
)

// reviewMsg is Frigate's frigate/reviews envelope (Frigate 0.14+). before is
// the prior state, after the current one; we only ever read after.
type reviewMsg struct {
	Type  string        `json:"type"` // new | update | end
	After reviewSegment `json:"after"`
}

type reviewSegment struct {
	ID        string  `json:"id"`
	Camera    string  `json:"camera"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"` // 0/absent while in progress
	Severity  string  `json:"severity"` // alert | detection
	ThumbPath string  `json:"thumb_path"`
	Data      struct {
		Detections []string `json:"detections"`
		Objects    []string `json:"objects"`
		Zones      []string `json:"zones"`
	} `json:"data"`
}

func parseReview(payload []byte, now time.Time) (reviewMsg, store.ReviewMsg, error) {
	var m reviewMsg
	if err := json.Unmarshal(payload, &m); err != nil {
		return m, store.ReviewMsg{}, fmt.Errorf("decode: %w", err)
	}
	if m.After.ID == "" || m.After.Camera == "" {
		return m, store.ReviewMsg{}, fmt.Errorf("missing after.id/camera")
	}
	switch m.Type {
	case "new", "update", "end":
	default:
		return m, store.ReviewMsg{}, fmt.Errorf("unknown type %q", m.Type)
	}
	a := m.After
	return m, store.ReviewMsg{
		ReviewID:  a.ID,
		Camera:    a.Camera,
		Severity:  a.Severity,
		ThumbPath: a.ThumbPath,
		EventIDs:  a.Data.Detections,
		Objects:   a.Data.Objects,
		Zones:     a.Data.Zones,
		StartTime: a.StartTime,
		EndTime:   a.EndTime,
		Ended:     m.Type == "end",
		At:        now,
		Raw:       string(payload),
	}, nil
}
