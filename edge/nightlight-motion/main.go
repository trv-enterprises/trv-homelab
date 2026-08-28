// nightlight-motion captures nightlight occupancy events into ts-store.
//
// Subscribes to each configured nightlight's Zigbee2MQTT state topic
// (<prefix>/<device>) and writes one tall record per publish to a single
// schema store, keyed by a `device` discriminator — the same table-shape
// convention as the docker-containers and synology-snmp stores. Z2M
// publishes the full cached device state on every update, so occupancy
// messages carry illuminance for free and we capture both.
//
// A single-level MQTT wildcard cannot match a name prefix ('+' must occupy
// an entire topic level, so "zigbee2mqtt/night-light-+" is not a valid
// filter) — hence the explicit device list.
//
// Writes go over ts-store's local unix socket (AUTH line protocol), one
// connection per record: motion events are sparse, so a persistent
// connection would sit idle. Records that fail to write are logged and
// dropped — this is a telemetry tap, not a system of record.
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
		clientID:    env("MQTT_CLIENT_ID", "nightlight-motion"),
		topicPrefix: env("MQTT_TOPIC_PREFIX", "zigbee2mqtt"),
		socketPath:  env("TSSTORE_SOCKET_PATH", "/var/run/tsstore/tsstore.sock"),
		store:       env("TSSTORE_STORE", "nightlight-motion"),
		apiKey:      os.Getenv("TSSTORE_API_KEY"),
	}
	for _, d := range strings.Split(os.Getenv("NIGHTLIGHT_DEVICES"), ",") {
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
		return cfg, fmt.Errorf("NIGHTLIGHT_DEVICES is required (comma-separated Z2M device names)")
	}
	return cfg, nil
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
	slog.Info("nightlight-motion starting",
		"broker", cfg.broker, "devices", len(cfg.devices), "store", cfg.store)

	onMessage := func(_ mqtt.Client, msg mqtt.Message) {
		var payload map[string]any
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			return // availability and other non-JSON publishes
		}
		occupancy, ok := payload["occupancy"].(bool)
		if !ok {
			return // not a state message
		}
		device := msg.Topic()[strings.LastIndex(msg.Topic(), "/")+1:]
		record := map[string]any{
			"device":    device,
			"occupancy": 0,
		}
		if occupancy {
			record["occupancy"] = 1
		}
		if lux, ok := payload["illuminance"].(float64); ok {
			record["illuminance"] = int64(lux)
		}
		if err := tsstoreWrite(cfg, record); err != nil {
			slog.Error("tsstore write failed, record dropped", "device", device, "error", err)
			return
		}
		slog.Info("wrote", "device", device, "occupancy", record["occupancy"], "illuminance", record["illuminance"])
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
