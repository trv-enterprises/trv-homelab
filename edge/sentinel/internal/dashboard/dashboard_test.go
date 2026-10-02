package dashboard

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseSSE(t *testing.T) {
	stream := "event: connected\ndata: {\"timestamp\":1}\n\n: heartbeat 2\n\nevent: alert\ndata: {\"id\":\"a1\",\"title\":\"x\"}\n\ndata: {\"no\":\"event\"}\n\n"
	var got []string
	err := ParseSSE(context.Background(), strings.NewReader(stream), func(ev, data string) { got = append(got, ev+"|"+data) })
	if err == nil || err.Error() != "EOF" {
		t.Fatalf("expected EOF at end, got %v", err)
	}
	if len(got) != 3 || got[1] != `alert|{"id":"a1","title":"x"}` || got[2] != `|{"no":"event"}` {
		t.Fatalf("frames: %q", got)
	}
}

func TestLinkAndSource(t *testing.T) {
	c := NewClient(Config{PublicURL: "http://dash.example", LinkUserID: "u-1"})
	a := Alert{DashboardID: "d1", DashboardVars: map[string]string{"host": "trv-srv-001", "empty": ""}}
	u := c.Link(a)
	if !strings.HasPrefix(u, "http://dash.example/view/dashboards/d1?") || !strings.Contains(u, "var_host=trv-srv-001") || !strings.Contains(u, "user_id=u-1") || strings.Contains(u, "var_empty") {
		t.Fatalf("link %q", u)
	}
	if c.Link(Alert{}) != "" {
		t.Fatal("no dashboard id must mean no link")
	}
	if SourceID("High Temp (CPU) ") != "dashboard_high-temp-cpu" || SourceID("") != "dashboard_unnamed" {
		t.Fatalf("source ids: %q %q", SourceID("High Temp (CPU) "), SourceID(""))
	}
	if Severity("error") != "critical" || Severity("info") != "info" || Severity("warning") != "warning" || Severity("") != "warning" {
		t.Fatal("severity map")
	}
	m := c.ToMsg(Alert{ID: "x", RuleName: "r", Source: "s", FiredAt: time.Unix(1, 0)}, "{}")
	if m.SourceID != "dashboard_r" || m.ExternalID != "x" || m.Store != "s" {
		t.Fatalf("msg %+v", m)
	}
}
