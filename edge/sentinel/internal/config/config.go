// Package config reads sentinel's configuration from the environment. There
// is no config file: the Ansible role templates an env file and compose
// injects it, the same way weather-poller is configured.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

type Config struct {
	MQTTBroker   string
	MQTTClientID string

	HTTPAddr      string
	PublicBaseURL string // scheme+host the app reaches us at; used in signed media links
	APIToken      string // the owner's token; also the key media links are signed with
	// People is everyone who may use the API, the owner first. The owner is
	// whoever API_TOKEN belongs to, and adopts the data of a server that had
	// no people yet.
	People []model.Person

	DBPath    string
	Retention time.Duration

	FrigateURL string
	RulesPath  string

	APNSKey      []byte // PEM contents of the .p8
	APNSKeyID    string
	APNSTeamID   string
	APNSBundleID string

	// Dashboard alert feed (optional). URL+key enable it.
	DashboardURL        string // API base, e.g. http://192.168.1.44:3001
	DashboardAPIKey     string
	DashboardPublicURL  string // what Safari opens, e.g. http://dashboard.<tailnet>.ts.net
	DashboardLinkUserID string // appended as ?user_id= so the browser is signed in
	DashboardMarkSeen   bool   // resolving in the app marks the alert seen in the dashboard
}

func Load() (Config, error) {
	c := Config{
		MQTTBroker:          env("MQTT_BROKER", "tcp://mosquitto:1883"),
		MQTTClientID:        env("MQTT_CLIENT_ID", "sentinel"),
		HTTPAddr:            env("HTTP_ADDR", ":8070"),
		PublicBaseURL:       strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:8070"), "/"),
		APIToken:            os.Getenv("API_TOKEN"),
		DBPath:              env("DB_PATH", "/data/sentinel.db"),
		FrigateURL:          strings.TrimRight(env("FRIGATE_URL", "http://192.168.1.156:5000"), "/"),
		RulesPath:           env("RULES_PATH", "/etc/marshal/rules.yaml"),
		APNSKeyID:           os.Getenv("APNS_KEY_ID"),
		DashboardURL:        strings.TrimRight(os.Getenv("DASHBOARD_URL"), "/"),
		DashboardAPIKey:     os.Getenv("DASHBOARD_API_KEY"),
		DashboardPublicURL:  strings.TrimRight(os.Getenv("DASHBOARD_PUBLIC_URL"), "/"),
		DashboardLinkUserID: os.Getenv("DASHBOARD_LINK_USER_ID"),
		DashboardMarkSeen:   env("DASHBOARD_MARK_SEEN", "true") == "true",
		APNSTeamID:          os.Getenv("APNS_TEAM_ID"),
		APNSBundleID:        os.Getenv("APNS_BUNDLE_ID"),
	}

	if c.APIToken == "" {
		return c, fmt.Errorf("API_TOKEN is required")
	}
	if len(c.APIToken) < 16 {
		return c, fmt.Errorf("API_TOKEN is too short (min 16 chars)")
	}

	people, err := parsePeople(env("API_PERSON", "owner"), c.APIToken, os.Getenv("API_PEOPLE"))
	if err != nil {
		return c, err
	}
	c.People = people

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

var personID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// parsePeople builds the people list: the owner (API_PERSON, holding
// API_TOKEN) followed by API_PEOPLE, a comma-separated list of id:token.
// Ids are lowercase names; tokens must be distinct, because the token is the
// only thing that says who is asking.
func parsePeople(owner, ownerToken, extra string) ([]model.Person, error) {
	people := []model.Person{{ID: strings.TrimSpace(owner), Token: ownerToken}}
	for _, pair := range strings.Split(extra, ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		id, tok, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, fmt.Errorf("API_PEOPLE: want id:token pairs separated by commas")
		}
		people = append(people, model.Person{ID: strings.TrimSpace(id), Token: strings.TrimSpace(tok)})
	}
	ids, tokens := map[string]bool{}, map[string]bool{}
	for _, p := range people {
		switch {
		case !personID.MatchString(p.ID):
			return nil, fmt.Errorf("person id %q: use lowercase letters, digits, - and _", p.ID)
		case len(p.Token) < 16:
			return nil, fmt.Errorf("token for %s is too short (min 16 chars)", p.ID)
		case ids[p.ID]:
			return nil, fmt.Errorf("person id %s is used twice", p.ID)
		case tokens[p.Token]:
			return nil, fmt.Errorf("%s has the same token as someone else", p.ID)
		}
		ids[p.ID], tokens[p.Token] = true, true
	}
	return people, nil
}

// DashboardEnabled reports whether the dashboard feed is configured.
func (c Config) DashboardEnabled() bool { return c.DashboardURL != "" && c.DashboardAPIKey != "" }

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
