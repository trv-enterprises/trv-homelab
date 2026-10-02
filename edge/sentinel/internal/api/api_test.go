package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

// Frigate's nginx gzips .m3u8 when the client asks for it, and AVPlayer
// always asks. The proxy must get (or make) plain text before it appends
// segment signatures, or it ships a corrupt gzip body.
func TestHLSPlaylistThroughProxyIsNeverGzipped(t *testing.T) {
	const playlist = "#EXTM3U\n#EXTINF:2.392,\nseg-1-v1-a1.ts\n#EXT-X-ENDLIST\n"
	var sawEncoding string
	frigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vod/event/e1/index.m3u8" {
			http.NotFound(w, r)
			return
		}
		sawEncoding = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Vary", "Accept-Encoding")
		if strings.Contains(sawEncoding, "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			io.WriteString(gz, playlist)
			gz.Close()
			return
		}
		io.WriteString(w, playlist)
	}))
	defer frigate.Close()

	s := NewSigner("http://h", "0123456789abcdef0123456789abcdef", time.Minute)
	proxy := newFrigateProxy(frigate.URL, s)

	for _, accept := range []string{"", "gzip, deflate", "identity"} {
		req := httptest.NewRequest("GET", "/v1/media/hls/event/e1/index.m3u8", nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("accept=%q: status %d", accept, rec.Code)
		}
		if ce := rec.Header().Get("Content-Encoding"); ce != "" {
			t.Fatalf("accept=%q: response still encoded %q", accept, ce)
		}
		body := rec.Body.String()
		if !strings.HasPrefix(body, "#EXTM3U") {
			t.Fatalf("accept=%q: body is not a plaintext playlist (upstream saw Accept-Encoding=%q):\n%q", accept, sawEncoding, body)
		}
		lines := strings.Split(strings.TrimSpace(body), "\n")
		if len(lines) != 4 || !strings.HasPrefix(lines[2], "seg-1-v1-a1.ts?exp=") || !strings.Contains(lines[2], "&sig=") {
			t.Fatalf("accept=%q: segment not signed:\n%s", accept, body)
		}
		if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
			t.Fatalf("accept=%q: Content-Length %q for %d bytes", accept, got, len(body))
		}
	}
}
