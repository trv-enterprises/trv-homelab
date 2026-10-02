package api

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

// Signer mints and checks signed media URLs. The signature covers the path
// and the expiry, keyed with the API token, so a leaked URL is only good for
// one media object for a limited time and never exposes the token itself.
type Signer struct {
	base string
	key  []byte
	ttl  time.Duration
}

func NewSigner(publicBase, token string, ttl time.Duration) *Signer {
	return &Signer{base: strings.TrimRight(publicBase, "/"), key: []byte(token), ttl: ttl}
}

func (s *Signer) sig(path string, exp int64) string {
	m := hmac.New(sha256.New, s.key)
	fmt.Fprintf(m, "%s|%d", path, exp)
	return hex.EncodeToString(m.Sum(nil))
}

// Sign returns an absolute URL for path valid for the signer's ttl.
func (s *Signer) Sign(path string) string {
	exp := time.Now().Add(s.ttl).Unix()
	return fmt.Sprintf("%s%s?exp=%d&sig=%s", s.base, path, exp, s.sig(path, exp))
}

// Verify checks exp/sig query parameters for path.
func (s *Signer) Verify(path string, q url.Values) bool {
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	want := s.sig(path, exp)
	return hmac.Equal([]byte(q.Get("sig")), []byte(want))
}

// MediaFor builds the signed links for a camera alert.
func (s *Signer) MediaFor(a model.Alert) *model.Media {
	if a.Frigate == nil {
		return nil
	}
	f := a.Frigate
	m := &model.Media{}
	if f.ReviewID != "" {
		m.Thumb = s.Sign(fmt.Sprintf("/v1/media/reviews/%s/%s/thumb.webp", a.Camera, f.ReviewID))
	}
	if len(f.EventIDs) > 0 {
		m.Snapshot = s.Sign(fmt.Sprintf("/v1/media/events/%s/snapshot.jpg", f.EventIDs[0]))
		m.Clip = s.Sign(fmt.Sprintf("/v1/media/events/%s/clip.mp4", f.EventIDs[0]))
	}
	if len(f.EventIDs) > 0 {
		m.HLS = s.Sign(fmt.Sprintf("/v1/media/hls/event/%s/index.m3u8", f.EventIDs[0]))
	}
	if f.StartTime > 0 && f.EndTime > f.StartTime {
		st, en := strconv.FormatFloat(f.StartTime, 'f', 3, 64), strconv.FormatFloat(f.EndTime, 'f', 3, 64)
		m.Recording = s.Sign(fmt.Sprintf("/v1/media/recordings/%s/%s/%s/clip.mp4", a.Camera, st, en))
		m.HLSRange = s.Sign(fmt.Sprintf("/v1/media/hls/%s/%s/%s/index.m3u8", a.Camera, st, en))
	}
	return m
}

// ThumbURL satisfies push.LinkSigner.
func (s *Signer) ThumbURL(a model.Alert) string {
	if m := s.MediaFor(a); m != nil {
		return m.Thumb
	}
	return ""
}

// newFrigateProxy reverse-proxies a fixed set of Frigate paths. The mapping
// mirrors what the dashboard's Go server does, including rebuilding the
// review thumbnail path from camera + review id (thumb_path is a container
// filesystem path, not a URL).
func newFrigateProxy(frigateURL string, signer *Signer) http.Handler {
	target, err := url.Parse(frigateURL)
	if err != nil {
		panic("bad FRIGATE_URL: " + err.Error())
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = mapMediaPath(pr.In.URL.Path)
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = ""
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Set("X-Sentinel-Public-Path", pr.In.URL.Path)
			pr.Out.Host = target.Host
			// Playlists are rewritten below, so they must arrive as text.
			// AVPlayer (and tailscale serve in front of us) advertise gzip
			// and Frigate's nginx honours it for .m3u8; signing the bytes
			// of a gzip stream produces a body nothing can decode.
			if strings.HasSuffix(pr.Out.URL.Path, ".m3u8") {
				pr.Out.Header.Set("Accept-Encoding", "identity")
			}
		},
		FlushInterval: -1, // stream video bodies as they arrive
		ModifyResponse: func(resp *http.Response) error {
			// Frigate serves review thumbnails untyped; AsyncImage copes but
			// the notification extension's attachment needs a real type.
			if strings.HasSuffix(resp.Request.URL.Path, ".webp") && !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
				resp.Header.Set("Content-Type", "image/webp")
			}
			resp.Header.Set("Cache-Control", "private, max-age=3600")
			// HLS: AVPlayer resolves segment names relative to the playlist
			// URL and drops the query string, so each segment line gets its
			// own signed query appended here.
			if strings.HasSuffix(resp.Request.URL.Path, ".m3u8") && resp.StatusCode == http.StatusOK {
				var rd io.Reader = resp.Body
				// Belt and braces: if something upstream compressed it
				// anyway, inflate before rewriting and serve it plain.
				if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
					gz, err := gzip.NewReader(resp.Body)
					if err != nil {
						resp.Body.Close()
						return err
					}
					defer gz.Close()
					rd = gz
				}
				body, err := io.ReadAll(io.LimitReader(rd, 4<<20))
				resp.Body.Close()
				if err != nil {
					return err
				}
				resp.Header.Del("Content-Encoding")
				// The public path is what the client requested; the proxy
				// request path was already rewritten, so recover it from
				// the original request header we stashed.
				pub := resp.Request.Header.Get("X-Sentinel-Public-Path")
				out := signPlaylist(body, path.Dir(pub), signer)
				resp.Body = io.NopCloser(bytes.NewReader(out))
				resp.ContentLength = int64(len(out))
				resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
				resp.Header.Set("Content-Type", "application/vnd.apple.mpegurl")
			}
			return nil
		},
	}
	return rp
}

// mapMediaPath translates /v1/media/... into Frigate's URL space. Returns ""
// for anything unrecognized, which the caller turns into a 404 before the
// proxy ever runs.
func mapMediaPath(p string) string {
	parts := strings.Split(strings.TrimPrefix(p, "/v1/media/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "events" && parts[2] == "clip.mp4":
		return "/api/events/" + parts[1] + "/clip.mp4"
	case len(parts) == 3 && parts[0] == "events" && parts[2] == "snapshot.jpg":
		return "/api/events/" + parts[1] + "/snapshot.jpg"
	case len(parts) == 4 && parts[0] == "reviews" && parts[3] == "thumb.webp":
		return "/clips/review/thumb-" + parts[1] + "-" + parts[2] + ".webp"
	case len(parts) == 5 && parts[0] == "recordings" && parts[4] == "clip.mp4":
		return "/api/" + parts[1] + "/start/" + parts[2] + "/end/" + parts[3] + "/clip.mp4"
	case len(parts) == 4 && parts[0] == "hls" && parts[1] == "event" && hlsFile(parts[3]):
		return "/vod/event/" + parts[2] + "/" + parts[3]
	case len(parts) == 5 && parts[0] == "hls" && hlsFile(parts[4]):
		return "/vod/" + parts[1] + "/start/" + parts[2] + "/end/" + parts[3] + "/" + parts[4]
	}
	return ""
}

var hlsFileRe = regexp.MustCompile(`^(index\.m3u8|seg-[A-Za-z0-9-]+\.ts)$`)

func hlsFile(name string) bool { return hlsFileRe.MatchString(name) }

// signPlaylist appends a signed query to every media line of an HLS
// playlist. dir is the public directory of the playlist (e.g.
// /v1/media/hls/event/<id>); lines are relative segment names.
func signPlaylist(body []byte, dir string, signer *Signer) []byte {
	var out bytes.Buffer
	for _, line := range strings.Split(string(body), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.Contains(t, "://") || strings.HasPrefix(t, "/") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		p := dir + "/" + t
		signed := signer.Sign(p)
		// keep the segment relative: strip base + dir, leaving name?exp=&sig=
		out.WriteString(t + signed[strings.Index(signed, "?"):])
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	if mapMediaPath(r.URL.Path) == "" {
		writeErr(w, http.StatusNotFound, "unknown media path")
		return
	}
	s.proxy.ServeHTTP(w, r)
}
