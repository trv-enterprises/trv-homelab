// presence-sensor captures presence-sensor state into ts-store.
//
// Subscribes to each configured sensor's Zigbee2MQTT state topic
// (<prefix>/<device>) and writes one tall record per publish to a single
// schema store, keyed by a `device` discriminator — the same table-shape
// convention as nightlight-motion, docker-containers and synology-snmp.
//
// It is the sibling of nightlight-motion, and exists because a dedicated
// presence sensor is a different shape of device: it reports `presence`
// rather than `occupancy`, and carries environment and battery readings a
// mains-powered nightlight does not. Written against the Aqara FP300
// (PS-S04D); any sensor publishing a boolean `presence` fits, and simply
// omits the fields it does not have.
//
// Strictly passive. It never publishes — no /get, no /set — so it cannot
// wake a battery device or cost it a transmission. A sleepy end device
// reports when something changes (or on its own heartbeat interval), and
// Z2M republishes its full cached state each time, so every record carries
// every field and one record is one device-side change.
//
// Writes go over ts-store's local unix socket (AUTH line protocol), one
// connection per record: events are sparse, so a persistent connection
// would sit idle. Records that fail to write are logged and dropped — this
// is a telemetry tap, not a system of record.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Fields copied from the Z2M payload when present, under Z2M's own names.
// `presence` is handled separately: it is what makes a publish a state
// message at all. Keep these in step with the schema the Ansible role
// applies (roles/tsstore/tasks/presence-sensor.yml).
var (
	boolFields  = []string{"pir_detection"}
	intFields   = []string{"illuminance", "battery", "voltage", "linkquality"}
	floatFields = []string{"temperature", "humidity"}
)

type config struct {
	broker      string
	clientID    string
	topicPrefix string
	devices     []string
	socketPath  string
	store       string
	apiKey      string
}

func loadConfig() (config, error) {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	cfg := config{
		broker:      os.Getenv("MQTT_BROKER"),
		clientID:    env("MQTT_CLIENT_ID", "presence-sensor"),
		topicPrefix: env("MQTT_TOPIC_PREFIX", "zigbee2mqtt"),
		socketPath:  env("TSSTORE_SOCKET_PATH", "/var/run/tsstore/tsstore.sock"),
		store:       env("TSSTORE_STORE", "presence-sensors"),
		apiKey:      os.Getenv("TSSTORE_API_KEY"),
	}
	for _, d := range strings.Split(os.Getenv("PRESENCE_DEVICES"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			cfg.devices = append(cfg.devices, d)
		}
	}
	switch {
	case cfg.broker == "":
		return cfg, fmt.Errorf("MQTT_BROKER is required (e.g. tcp://<broker>:1883)")
	case cfg.apiKey == "":
		return cfg, fmt.Errorf("TSSTORE_API_KEY is required")
	case len(cfg.devices) == 0:
		return cfg, fmt.Errorf("PRESENCE_DEVICES is required (comma-separated Z2M device names)")
	}
	return cfg, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// buildRecord maps one Z2M publish to a store record. ok is false for
// anything that is not a device state message (availability strings,
// payloads without a boolean `presence`).
func buildRecord(device string, raw []byte) (record map[string]any, ok bool) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	presence, isBool := payload["presence"].(bool)
	if !isBool {
		return nil, false
	}
	record = map[string]any{
		"device":   device,
		"presence": boolToInt(presence),
	}
	for _, f := range boolFields {
		if v, ok := payload[f].(bool); ok {
			record[f] = boolToInt(v)
		}
	}
	// encoding/json decodes every JSON number as float64.
	for _, f := range intFields {
		if v, ok := payload[f].(float64); ok {
			record[f] = int64(v)
		}
	}
	for _, f := range floatFields {
		if v, ok := payload[f].(float64); ok {
			record[f] = v
		}
	}
	return record, true
}

// tsstoreWrite opens the socket, authenticates, writes one record, quits.
func tsstoreWrite(cfg config, record map[string]any) error {
	conn, err := net.DialTimeout("unix", cfg.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}

	expectOK := func(stage string) error {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("%s read: %w", stage, err)
		}
		if resp := strings.TrimSpace(string(buf[:n])); !strings.HasPrefix(resp, "OK") {
			return fmt.Errorf("%s failed: %s", stage, resp)
		}
		return nil
	}

	if _, err := fmt.Fprintf(conn, "AUTH %s %s\n", cfg.store, cfg.apiKey); err != nil {
		return fmt.Errorf("auth write: %w", err)
	}
	if err := expectOK("auth"); err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("record write: %w", err)
	}
	if err := expectOK("write"); err != nil {
		return err
	}
	_, _ = conn.Write([]byte("QUIT\n"))
	return nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	slog.Info("presence-sensor starting",
		"broker", cfg.broker, "devices", len(cfg.devices), "store", cfg.store)

	onMessage := func(_ mqtt.Client, msg mqtt.Message) {
		device := msg.Topic()[strings.LastIndex(msg.Topic(), "/")+1:]
		record, ok := buildRecord(device, msg.Payload())
		if !ok {
			return
		}
		if err := tsstoreWrite(cfg, record); err != nil {
			slog.Error("tsstore write failed, record dropped", "device", device, "error", err)
			return
		}
		slog.Info("wrote", "device", device, "presence", record["presence"],
			"pir_detection", record["pir_detection"], "illuminance", record["illuminance"])
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.broker).
		SetClientID(cfg.clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10 * time.Second).
		SetMaxReconnectInterval(time.Minute)

	// Subscribe from OnConnect, never once-at-startup: under CleanSession the
	// broker discards subscriptions on every disconnect, and paho's ResumeSubs
	// resumes nothing (see trv-marshal's reconnect notes). This handler runs
	// on the initial connect and every reconnect.
	//
	// Z2M retains device state, so each (re)subscribe is answered with the
	// last known state and writes one record that is not a fresh device
	// report. That is the broker replaying its cache — nothing is asked of
	// the sensor.
	opts.OnConnect = func(client mqtt.Client) {
		for _, d := range cfg.devices {
			topic := cfg.topicPrefix + "/" + d
			if t := client.Subscribe(topic, 1, onMessage); t.Wait() && t.Error() != nil {
				slog.Error("subscribe failed", "topic", topic, "error", t.Error())
				continue
			}
			slog.Info("subscribed", "topic", topic)
		}
	}
	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		slog.Warn("mqtt connection lost", "error", err)
	}

	client := mqtt.NewClient(opts)
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		// With SetConnectRetry the initial connect retries internally, so an
		// error here is a config-shaped problem (bad URL), not a flaky broker.
		slog.Error("mqtt connect", "error", t.Error())
		os.Exit(1)
	}

	select {} // run until systemd stops us
}
