// Package model holds the wire and storage types shared by every sentinel
// package. Keep it dependency-free: store, ingest, push and api all import it.
package model

import "time"

// Alert kinds.
const (
	KindSensor = "sensor" // Marshal rule alert (sensors/alerts)
	KindCamera = "camera" // Frigate review (frigate/reviews)
)

// Alert statuses.
const (
	StatusActive   = "active"
	StatusResolved = "resolved" // sensor: Marshal sent "resolved", or a human resolved it
	StatusEnded    = "ended"    // camera: Frigate sent type "end"
	StatusStale    = "stale"    // sensor: nothing heard past the rule's repeat window
)

// Severities as stored. Marshal's info/warning/critical are kept verbatim;
// Frigate's "alert" is mapped to warning so the app has one scale.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Alert is one notification-worthy thing that happened. Sensor alerts are
// keyed by Marshal rule name while active; camera alerts by Frigate review ID.
type Alert struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Severity    string     `json:"severity"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Rule        string     `json:"rule"`
	Device      string     `json:"device,omitempty"`
	Camera      string     `json:"camera,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	RepeatCount int        `json:"repeat_count"`
	Frigate     *Frigate   `json:"frigate,omitempty"`
	Media       *Media     `json:"media,omitempty"`
	Raw         string     `json:"-"`
}

// Frigate carries the review fields the app needs to find media.
type Frigate struct {
	ReviewID  string   `json:"review_id"`
	ThumbPath string   `json:"thumb_path,omitempty"`
	EventIDs  []string `json:"event_ids"`
	Objects   []string `json:"objects"`
	StartTime float64  `json:"start_time"`
	EndTime   float64  `json:"end_time,omitempty"`
}

// Media is the set of pre-signed URLs the api layer attaches to a camera
// alert on the way out. Empty strings mean "not available yet".
type Media struct {
	Thumb     string `json:"thumb,omitempty"`
	Snapshot  string `json:"snapshot,omitempty"`
	Clip      string `json:"clip,omitempty"`      // mp4 of the first detection event (download/fallback)
	Recording string `json:"recording,omitempty"` // mp4 of the whole review span
	HLS       string `json:"hls,omitempty"`       // HLS playlist for the first event; what AVPlayer should use
	HLSRange  string `json:"hls_range,omitempty"` // HLS playlist for the whole review span
}

// Device is a registered APNs device token.
type Device struct {
	Token      string    `json:"token"`
	Env        string    `json:"env"` // "sandbox" | "production"
	Name       string    `json:"name,omitempty"`
	AppVersion string    `json:"app_version,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// APNs environments.
const (
	EnvSandbox    = "sandbox"
	EnvProduction = "production"
)

// Event is what the SSE stream and the push layer receive after a store
// change. Created is true for the first sighting of an alert.
type Event struct {
	Type    string `json:"type"` // "created" | "updated"
	Alert   Alert  `json:"alert"`
	Created bool   `json:"-"`
}
