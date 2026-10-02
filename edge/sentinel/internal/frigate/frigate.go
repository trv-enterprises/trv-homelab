// Package frigate reads the camera list from Frigate's config API so the
// notification sources can include cameras that have never alerted yet.
package frigate

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Cameras struct {
	base   string
	client *http.Client
	ttl    time.Duration

	mu     sync.Mutex
	names  []string
	loaded time.Time
}

func NewCameras(baseURL string, ttl time.Duration) *Cameras {
	return &Cameras{base: baseURL, client: &http.Client{Timeout: 8 * time.Second}, ttl: ttl}
}

// Names returns the configured camera names, cached for ttl. On error the
// last good list (possibly empty) is returned with the error.
func (c *Cameras) Names(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.loaded) < c.ttl && c.names != nil {
		return c.names, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/config", nil)
	if err != nil {
		return c.names, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.loaded = time.Now()
		return c.names, err
	}
	defer resp.Body.Close()
	var cfg struct {
		Cameras map[string]json.RawMessage `json:"cameras"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		c.loaded = time.Now()
		return c.names, err
	}
	names := make([]string, 0, len(cfg.Cameras))
	for n := range cfg.Cameras {
		names = append(names, n)
	}
	sort.Strings(names)
	c.names, c.loaded = names, time.Now()
	return names, nil
}
