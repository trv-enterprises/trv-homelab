package api

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

func TestSignerRoundTrip(t *testing.T) {
	s := NewSigner("https://host.example", "0123456789abcdef0123456789abcdef", time.Minute)
	u := s.Sign("/v1/media/events/e1/clip.mp4")
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host != "host.example" {
		t.Fatalf("bad url %q: %v", u, err)
	}
	if !s.Verify(parsed.Path, parsed.Query()) {
		t.Fatal("fresh signature should verify")
	}
	q := parsed.Query()
	q.Set("sig", strings.Repeat("0", 64))
	if s.Verify(parsed.Path, q) {
		t.Fatal("tampered sig verified")
	}
	if s.Verify("/v1/media/events/OTHER/clip.mp4", parsed.Query()) {
		t.Fatal("signature bound to wrong path")
	}
	expired := NewSigner("https://host.example", "0123456789abcdef0123456789abcdef", -time.Minute)
	eu, _ := url.Parse(expired.Sign("/x"))
	if expired.Verify(eu.Path, eu.Query()) {
		t.Fatal("expired signature verified")
	}
}

func TestMediaFor(t *testing.T) {
	s := NewSigner("http://h", "0123456789abcdef0123456789abcdef", time.Minute)
	a := model.Alert{Kind: model.KindCamera, Camera: "driveway", Frigate: &model.Frigate{ReviewID: "r1", EventIDs: []string{"e1"}, StartTime: 10, EndTime: 20}}
	m := s.MediaFor(a)
	if m == nil || m.Thumb == "" || m.Clip == "" || m.Snapshot == "" || m.Recording == "" {
		t.Fatalf("media %+v", m)
	}
	if !strings.Contains(m.Recording, "/recordings/driveway/10.000/20.000/clip.mp4") {
		t.Fatalf("recording path %q", m.Recording)
	}
	a.Frigate.EventIDs = nil
	a.Frigate.EndTime = 0
	m = s.MediaFor(a)
	if m.Clip != "" || m.Recording != "" || m.Thumb == "" {
		t.Fatalf("partial media %+v", m)
	}
}

func TestMapMediaPath(t *testing.T) {
	cases := map[string]string{
		"/v1/media/events/e1/clip.mp4":                   "/api/events/e1/clip.mp4",
		"/v1/media/events/e1/snapshot.jpg":               "/api/events/e1/snapshot.jpg",
		"/v1/media/reviews/driveway/r1/thumb.webp":       "/clips/review/thumb-driveway-r1.webp",
		"/v1/media/recordings/driveway/1.0/2.0/clip.mp4": "/api/driveway/start/1.0/end/2.0/clip.mp4",
		"/v1/media/events/e1/../../config":               "",
		"/v1/media/anything":                             "",
		"/v1/media/hls/event/e1/index.m3u8":              "/vod/event/e1/index.m3u8",
		"/v1/media/hls/event/e1/seg-1-v1.ts":             "/vod/event/e1/seg-1-v1.ts",
		"/v1/media/hls/event/e1/evil.txt":                "",
		"/v1/media/hls/driveway/1.0/2.0/index.m3u8":      "/vod/driveway/start/1.0/end/2.0/index.m3u8",
	}
	for in, want := range cases {
		if got := mapMediaPath(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestSignPlaylist(t *testing.T) {
	s := NewSigner("http://h", "0123456789abcdef0123456789abcdef", time.Minute)
	in := []byte("#EXTM3U\n#EXTINF:2.392,\nseg-1-v1.ts\n#EXT-X-ENDLIST\n")
	out := string(signPlaylist(in, "/v1/media/hls/event/e1", s))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[2], "seg-1-v1.ts?exp=") || !strings.Contains(lines[2], "&sig=") {
		t.Fatalf("playlist not signed:\n%s", out)
	}
	u, _ := url.Parse("http://h/v1/media/hls/event/e1/" + lines[2])
	if !s.Verify(u.Path, u.Query()) {
		t.Fatal("segment signature does not verify against its public path")
	}
}
