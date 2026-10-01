// Package config reads sentinel's configuration from the environment. There
// is no config file: the Ansible role templates an env file and compose
// injects it, the same way weather-poller is configured.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	MQTTBroker   string
	MQTTClientID string

	HTTPAddr      string
	PublicBaseURL string // scheme+host the app reaches us at; used in signed media links
	APIToken      string

	DBPath    string
	Retention time.Duration

	FrigateURL string
	RulesPath  string

	APNSKey      []byte // PEM contents of the .p8
	APNSKeyID    string
	APNSTeamID   string
	APNSBundleID string
}

func Load() (Config, error) {
	c := Config{
		MQTTBroker:    env("MQTT_BROKER", "tcp://mosquitto:1883"),
		MQTTClientID:  env("MQTT_CLIENT_ID", "sentinel"),
		HTTPAddr:      env("HTTP_ADDR", ":8070"),
		PublicBaseURL: strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:8070"), "/"),
		APIToken:      os.Getenv("API_TOKEN"),
		DBPath:        env("DB_PATH", "/data/sentinel.db"),
		FrigateURL:    strings.TrimRight(env("FRIGATE_URL", "http://192.168.1.156:5000"), "/"),
		RulesPath:     env("RULES_PATH", "/etc/marshal/rules.yaml"),
		APNSKeyID:     os.Getenv("APNS_KEY_ID"),
		APNSTeamID:    os.Getenv("APNS_TEAM_ID"),
		APNSBundleID:  os.Getenv("APNS_BUNDLE_ID"),
	}

	if c.APIToken == "" {
		return c, fmt.Errorf("API_TOKEN is required")
	}
	if len(c.APIToken) < 16 {
		return c, fmt.Errorf("API_TOKEN is too short (min 16 chars)")
	}

	ret, err := time.ParseDuration(env("RETENTION", "720h"))
	if err != nil {
		return c, fmt.Errorf("RETENTION: %w", err)
	}
	c.Retention = ret

	// APNs is optional so the server can run (and be tested) before the
	// Apple side exists. All four must be present to enable push.
	if b64 := os.Getenv("APNS_KEY_B64"); b64 != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			return c, fmt.Errorf("APNS_KEY_B64: %w", err)
		}
		c.APNSKey = key
	}
	return c, nil
}

// PushEnabled reports whether every APNs setting is present.
func (c Config) PushEnabled() bool {
	return len(c.APNSKey) > 0 && c.APNSKeyID != "" && c.APNSTeamID != "" && c.APNSBundleID != ""
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
