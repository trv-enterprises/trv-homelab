package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A trimmed real publish from an Aqara FP300 (PS-S04D) on Zigbee2MQTT.
const fp300Payload = `{"absence_delay_timer":10,"battery":100,"humidity":57.17,
"illuminance":1,"linkquality":112,"motion_sensitivity":"medium",
"pir_detection":false,"presence":true,"presence_detection_options":"both",
"target_distance":0,"temperature":23.72,"voltage":3121}`

func TestBuildRecordFP300(t *testing.T) {
	record, ok := buildRecord("aqara", []byte(fp300Payload))
	if !ok {
		t.Fatal("state message rejected")
	}
	want := map[string]any{
		"device":        "aqara",
		"presence":      1,
		"pir_detection": 0,
		"illuminance":   int64(1),
		"battery":       int64(100),
		"voltage":       int64(3121),
		"linkquality":   int64(112),
		"temperature":   23.72,
		"humidity":      57.17,
	}
	if len(record) != len(want) {
		t.Errorf("got %d fields, want %d: %v", len(record), len(want), record)
	}
	for k, v := range want {
		if record[k] != v {
			t.Errorf("%s = %#v, want %#v", k, record[k], v)
		}
	}
}

// Integer fields must reach the wire as JSON integers: a schema store's
// int column does not take 3121.0.
func TestRecordIntegersMarshalWithoutFraction(t *testing.T) {
	record, _ := buildRecord("aqara", []byte(fp300Payload))
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{`"voltage":3121`, `"battery":100`, `"presence":1`, `"pir_detection":0`} {
		if !strings.Contains(string(line), f) {
			t.Errorf("record %s does not contain %s", line, f)
		}
	}
}

// A sensor without the FP300's extras writes only what it has.
func TestBuildRecordOmitsAbsentFields(t *testing.T) {
	record, ok := buildRecord("other", []byte(`{"presence":false,"illuminance":40}`))
	if !ok {
		t.Fatal("state message rejected")
	}
	if len(record) != 3 || record["presence"] != 0 || record["illuminance"] != int64(40) {
		t.Errorf("unexpected record: %v", record)
	}
}

func TestBuildRecordRejectsNonState(t *testing.T) {
	for name, payload := range map[string]string{
		"availability string": `online`,
		"no presence field":   `{"occupancy":true,"illuminance":3}`,
		"presence not a bool": `{"presence":"true"}`,
		"empty":               ``,
	} {
		if _, ok := buildRecord("d", []byte(payload)); ok {
			t.Errorf("%s: accepted, want rejected", name)
		}
	}
}
