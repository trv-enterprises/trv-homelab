package ingest

import (
	"testing"
	"time"
)

func TestParseSensor(t *testing.T) {
	now := time.Now()
	raw := []byte(`{"type":"new","source":"alert_engine","severity":"warning","rule":"large_garage_door_left_open","message":"Large garage door has been open for 30 minutes","device":"Large Garage Door","timestamp":"2026-10-01T12:00:00Z"}`)
	a, m, err := parseSensor(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Type != "new" || m.Rule != "large_garage_door_left_open" || m.Device != "Large Garage Door" {
		t.Fatalf("%+v %+v", a, m)
	}
	if !m.At.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp not parsed: %v", m.At)
	}
	// bad severity normalizes, bad type rejects
	if _, m, err := parseSensor([]byte(`{"type":"repeat","rule":"x","severity":"nope"}`), now); err != nil || m.Severity != "warning" {
		t.Fatalf("severity normalize: %v %+v", err, m)
	}
	if _, _, err := parseSensor([]byte(`{"type":"bogus","rule":"x"}`), now); err == nil {
		t.Fatal("expected error for bogus type")
	}
	if _, _, err := parseSensor([]byte(`not json`), now); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestParseReview(t *testing.T) {
	now := time.Now()
	raw := []byte(`{"type":"update","before":{"id":"1790881483.871611-wbo712","severity":"detection"},"after":{"id":"1790881483.871611-wbo712","camera":"driveway","start_time":1790881483.871611,"end_time":null,"severity":"alert","thumb_path":"/media/frigate/clips/review/thumb-driveway-1790881483.871611-wbo712.webp","data":{"detections":["1790881485.460721-mtjxk9","1790881483.244017-aqj4f3"],"objects":["car","person"],"sub_labels":[],"zones":["Street"],"audio":[]}}}`)
	m, r, err := parseReview(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != "update" || r.ReviewID != "1790881483.871611-wbo712" || r.Camera != "driveway" || r.Severity != "alert" {
		t.Fatalf("%+v", r)
	}
	if len(r.EventIDs) != 2 || len(r.Objects) != 2 || r.Zones[0] != "Street" || r.Ended {
		t.Fatalf("%+v", r)
	}
	if r.EndTime != 0 {
		t.Fatalf("null end_time should be 0, got %v", r.EndTime)
	}
	_, r2, err := parseReview([]byte(`{"type":"end","after":{"id":"a","camera":"c","severity":"alert","end_time":5.5}}`), now)
	if err != nil || !r2.Ended || r2.EndTime != 5.5 {
		t.Fatalf("end: %+v %v", r2, err)
	}
	if _, _, err := parseReview([]byte(`{"type":"new","after":{}}`), now); err == nil {
		t.Fatal("expected error for missing id")
	}
}
