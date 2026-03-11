package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	APIKey        string
	Location      string
	MQTTBroker    string
	PollInterval  time.Duration
	AlertInterval time.Duration
}

type CurrentConditions struct {
	Datetime       string   `json:"datetime"`
	Temp           float64  `json:"temp"`
	FeelsLike      float64  `json:"feelslike"`
	Humidity       float64  `json:"humidity"`
	Dew            float64  `json:"dew"`
	Precip         float64  `json:"precip"`
	PrecipProb     float64  `json:"precipprob"`
	WindGust       float64  `json:"windgust"`
	WindSpeed      float64  `json:"windspeed"`
	WindDir        float64  `json:"winddir"`
	Pressure       float64  `json:"pressure"`
	Visibility     float64  `json:"visibility"`
	CloudCover     float64  `json:"cloudcover"`
	UVIndex        float64  `json:"uvindex"`
	Conditions     string   `json:"conditions"`
	Icon           string   `json:"icon"`
	Sunrise        string   `json:"sunrise"`
	Sunset         string   `json:"sunset"`
	MoonPhase      float64  `json:"moonphase"`
	SolarRadiation float64  `json:"solarradiation"`
	Stations       []string `json:"stations"`
	Source         string   `json:"source"`
}

type Alert struct {
	Event       string `json:"event"`
	Headline    string `json:"headline"`
	Ends        string `json:"ends"`
	Onset       string `json:"onset"`
	ID          string `json:"id"`
	Language    string `json:"language"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

type HourConditions struct {
	Datetime   string  `json:"datetime"`
	Temp       float64 `json:"temp"`
	FeelsLike  float64 `json:"feelslike"`
	Humidity   float64 `json:"humidity"`
	Precip     float64 `json:"precip"`
	PrecipProb float64 `json:"precipprob"`
	WindSpeed  float64 `json:"windspeed"`
	WindGust   float64 `json:"windgust"`
	Conditions string  `json:"conditions"`
	Icon       string  `json:"icon"`
}

type Day struct {
	Datetime    string           `json:"datetime"`
	TempMax     float64          `json:"tempmax"`
	TempMin     float64          `json:"tempmin"`
	Temp        float64          `json:"temp"`
	Humidity    float64          `json:"humidity"`
	Precip      float64          `json:"precip"`
	PrecipProb  float64          `json:"precipprob"`
	WindSpeed   float64          `json:"windspeed"`
	WindGust    float64          `json:"windgust"`
	Conditions  string           `json:"conditions"`
	Description string           `json:"description"`
	Icon        string           `json:"icon"`
	SevereRisk  float64          `json:"severerisk"`
	Hours       []HourConditions `json:"hours"`
}

type WeatherResponse struct {
	QueryCost         int                `json:"queryCost"`
	Latitude          float64            `json:"latitude"`
	Longitude         float64            `json:"longitude"`
	ResolvedAddress   string             `json:"resolvedAddress"`
	Timezone          string             `json:"timezone"`
	Days              []Day              `json:"days"`
	Alerts            []Alert            `json:"alerts"`
	CurrentConditions *CurrentConditions `json:"currentConditions"`
}

func loadConfig() Config {
	cfg := Config{
		APIKey:        os.Getenv("VISUAL_CROSSING_KEY"),
		Location:      os.Getenv("WEATHER_LOCATION"),
		MQTTBroker:    os.Getenv("MQTT_BROKER"),
		PollInterval:  15 * time.Minute,
		AlertInterval: 5 * time.Minute,
	}
	if cfg.APIKey == "" {
		log.Fatal("VISUAL_CROSSING_KEY is required")
	}
	if cfg.Location == "" {
		cfg.Location = "Spring,TX"
	}
	if cfg.MQTTBroker == "" {
		cfg.MQTTBroker = "tcp://localhost:1883"
	}
	// Free tier: 1000 records/day. Min safe interval ~2min (720/day).
	// Default: current every 15min (96/day) + alerts every 5min (288/day) = 384/day.
	if cfg.PollInterval < 2*time.Minute {
		log.Printf("WARNING: poll interval %s is below 2min minimum, clamping to 2m", cfg.PollInterval)
		cfg.PollInterval = 2 * time.Minute
	}
	if cfg.AlertInterval < 2*time.Minute {
		log.Printf("WARNING: alert interval %s is below 2min minimum, clamping to 2m", cfg.AlertInterval)
		cfg.AlertInterval = 2 * time.Minute
	}
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PollInterval = d
		}
	}
	if v := os.Getenv("ALERT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.AlertInterval = d
		}
	}
	return cfg
}

func fetchWeather(apiKey, location, include string) (*WeatherResponse, error) {
	url := fmt.Sprintf(
		"https://weather.visualcrossing.com/VisualCrossingWebServices/rest/services/timeline/%s?unitGroup=us&include=%s&key=%s&contentType=json",
		location, include, apiKey,
	)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var weather WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		return nil, fmt.Errorf("JSON decode failed: %w", err)
	}
	return &weather, nil
}

func publish(client mqtt.Client, topic string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR marshaling %s: %v", topic, err)
		return
	}
	token := client.Publish(topic, 1, true, data)
	if token.Wait() && token.Error() != nil {
		log.Printf("ERROR publishing %s: %v", topic, token.Error())
	} else {
		log.Printf("Published %s (%d bytes)", topic, len(data))
	}
}

func pollCurrent(cfg Config, client mqtt.Client) {
	weather, err := fetchWeather(cfg.APIKey, cfg.Location, "current,hours,days")
	if err != nil {
		log.Printf("ERROR fetching current weather: %v", err)
		return
	}

	if weather.CurrentConditions != nil {
		publish(client, "weather/current", weather.CurrentConditions)
	}

	// Publish today's hourly forecast
	if len(weather.Days) > 0 {
		publish(client, "weather/forecast/hourly", weather.Days[0].Hours)

		// Publish daily forecast (up to 7 days)
		maxDays := 7
		if len(weather.Days) < maxDays {
			maxDays = len(weather.Days)
		}
		publish(client, "weather/forecast/daily", weather.Days[:maxDays])
	}

	log.Printf("Current: %.1f°F, %s (cost: %d)", weather.CurrentConditions.Temp, weather.CurrentConditions.Conditions, weather.QueryCost)
}

func pollAlerts(cfg Config, client mqtt.Client) {
	weather, err := fetchWeather(cfg.APIKey, cfg.Location, "alerts")
	if err != nil {
		log.Printf("ERROR fetching alerts: %v", err)
		return
	}

	publish(client, "weather/alerts", weather.Alerts)

	if len(weather.Alerts) > 0 {
		for _, a := range weather.Alerts {
			log.Printf("ALERT: %s — %s", a.Event, a.Headline)
		}
	}
}

func main() {
	cfg := loadConfig()
	log.Printf("Starting weather-poller: location=%s broker=%s poll=%s alerts=%s",
		cfg.Location, cfg.MQTTBroker, cfg.PollInterval, cfg.AlertInterval)

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID("weather-poller").
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10 * time.Second)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT connect failed: %v", token.Error())
	}
	log.Printf("Connected to MQTT broker")

	// Initial fetch
	pollCurrent(cfg, client)
	pollAlerts(cfg, client)

	currentTicker := time.NewTicker(cfg.PollInterval)
	alertTicker := time.NewTicker(cfg.AlertInterval)
	defer currentTicker.Stop()
	defer alertTicker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-currentTicker.C:
			pollCurrent(cfg, client)
		case <-alertTicker.C:
			pollAlerts(cfg, client)
		case <-sig:
			log.Println("Shutting down...")
			client.Disconnect(1000)
			return
		}
	}
}
